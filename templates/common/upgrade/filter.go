// Copyright 2024 The Authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package upgrade

import (
	"context"
	"fmt"
	"maps"
	"reflect"

	"github.com/google/cel-go/cel"
	"gopkg.in/yaml.v3"

	manifest "github.com/abcxyz/abc/templates/model/manifest/v1alpha1"
	"github.com/abcxyz/pkg/logging"
)

// The set of keys must be the same between manifestsUnfiltered and
// manifestBufs.
func filterManifests(ctx context.Context, filterCELExpr string, manifestsUnfiltered map[string]*manifest.Manifest, manifestBufs map[string][]byte) (map[string]*manifest.Manifest, error) {
	logger := logging.FromContext(ctx).With("logger", "filterOneManifest")

	out := maps.Clone(manifestsUnfiltered)
	if len(filterCELExpr) == 0 {
		return out, nil
	}

	for path, buf := range manifestBufs {
		ok, err := filterOneManifest(filterCELExpr, buf)
		if err != nil {
			return nil, err
		}
		logger.InfoContext(ctx, "The CEL filter was successfully evaluated for a manifest",
			"manifest_filename", path,
			"result", ok)

		if !ok {
			delete(out, path)
		}
	}

	return out, nil
}

// Returns true if the given CEL expression returns true when evaluated against
// the given manifest.
func filterOneManifest(filterCELExpr string, buf []byte) (bool, error) {
	// Subtle point: we use the YAML from the file rather than using the
	// manifest struct that's already in memory. This means that the CEL
	// expression will see exactly the values in the file, and not values that
	// might be inferred when upgrading to the latest api_version. This avoids
	// confusing the user with magically changing field values.
	var asMap map[string]any
	if err := yaml.Unmarshal(buf, &asMap); err != nil {
		return false, fmt.Errorf("internal error: failed unmarshaling YAML back to map: %w", err)
	}

	celOpts := make([]cel.EnvOption, 0, len(asMap))
	for name := range asMap {
		celOpts = append(celOpts, cel.Variable(name, cel.DynType))
	}
	celEnv, err := cel.NewEnv(celOpts...)
	if err != nil {
		return false, fmt.Errorf("internal error: cel.NewEnv(): %w", err)
	}

	ast, issues := celEnv.Compile(filterCELExpr)
	if err := issues.Err(); err != nil {
		return false, fmt.Errorf("failed compiling CEL expression: %w", err)
	}

	prog, err := celEnv.Program(ast)
	if err != nil {
		return false, fmt.Errorf("failed constructing CEL program: %w", err)
	}

	celOut, _, err := prog.Eval(asMap)
	if err != nil {
		return false, fmt.Errorf("failed executing CEL expression: %w", err)
	}

	boolI, err := celOut.ConvertToNative(reflect.TypeOf(true))
	if err != nil {
		return false, fmt.Errorf("CEL filter evaluation did not return bool: %w", err)
	}

	result, ok := boolI.(bool)
	if !ok {
		return false, fmt.Errorf("internal error: CEL filter evaluation should return bool, got %T: %w", boolI, err)
	}

	return result, nil
}
