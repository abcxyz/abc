// Copyright 2023 The Authors (see AUTHORS file)
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

package decode

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/abcxyz/abc/templates/model"
	goldentestv1alpha1 "github.com/abcxyz/abc/templates/model/goldentest/v1alpha1"
	manifestv1alpha1 "github.com/abcxyz/abc/templates/model/manifest/v1alpha1"
	specv1alpha1 "github.com/abcxyz/abc/templates/model/spec/v1alpha1"
	specv1beta1 "github.com/abcxyz/abc/templates/model/spec/v1beta1"
	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"
)

var (
	KindTemplate   = "Template"   // the value of the "kind" field in a spec.yaml file
	KindGoldenTest = "GoldenTest" // ... a test.yaml file
	KindManifest   = "Manifest"   // ... a manifest.yaml file
)

type apiVersionDef struct {
	apiVersion string

	// Map keys are the "kind" values found in the YAML files.
	kinds map[string]model.ValidatorUpgrader
}

// This list is in chronological order of API release (oldest to newest). To
// remove support for an api_version, delete from the head of the list. To add a
// new api_version, append to the end of the list. See
// templates/model/README.md for detailed instructions on creating a new
// api_version.

// Typically, you'll only be changing one of the model types
// (spec/manifest/goldentest) when introducing a new api_version. In that case
// just copy-paste the old model types from the previous api_version.
var apiVersions = []apiVersionDef{
	{
		apiVersion: "cli.abcxyz.dev/v1alpha1",
		kinds: map[string]model.ValidatorUpgrader{
			KindTemplate:   &specv1alpha1.Spec{},
			KindGoldenTest: &goldentestv1alpha1.Test{},
			KindManifest:   &manifestv1alpha1.Manifest{},
		},
	},
	{
		apiVersion: "cli.abcxyz.dev/v1beta1",
		kinds: map[string]model.ValidatorUpgrader{
			KindTemplate:   &specv1beta1.Spec{},
			KindGoldenTest: &goldentestv1alpha1.Test{},
			KindManifest:   &manifestv1alpha1.Manifest{},
		},
	},
}

// Decode parses the given YAML contents of r into a struct and returns it. The given filename
// is used only for error messages. The type of struct to return is determined by the "kind" field
// in the YAML. If the given requireKind is non-empty, then we'll also validate that the "kind"
// of the YAML file matches requireKind, and return error if not.
func Decode(r io.Reader, filename, requireKind string) (model.ValidatorUpgrader, string, error) {
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, "", fmt.Errorf("error reading file %s: %w", filename, err)
	}

	cf := &commonFields{}
	if err := yaml.Unmarshal(buf, cf); err != nil {
		return nil, "", fmt.Errorf("error parsing file %s: %w", filename, err)
	}

	var apiVersion string
	if cf.NewStyleAPIVersion.Val != "" && cf.OldStyleAPIVersion.Val != "" {
		return nil, "", cf.OldStyleAPIVersion.Pos.Errorf("must not set both apiVersion and api_version, please use api_version only")
	}
	if cf.NewStyleAPIVersion.Val == "" && cf.OldStyleAPIVersion.Val == "" {
		return nil, "", fmt.Errorf(`file %s must set the field "api_version"`, filename)
	}
	if cf.NewStyleAPIVersion.Val != "" {
		apiVersion = cf.NewStyleAPIVersion.Val
	}
	if cf.OldStyleAPIVersion.Val != "" {
		apiVersion = cf.OldStyleAPIVersion.Val
	}

	if cf.Kind == "" {
		return nil, "", fmt.Errorf(`file %s must set the field "kind"`, filename)
	}
	if requireKind != "" && cf.Kind != requireKind {
		return nil, "", fmt.Errorf("file %s has kind %q, but %q is required", filename, cf.Kind, requireKind)
	}

	vu, err := modelForKind(filename, apiVersion, cf.Kind)
	if err != nil {
		return nil, "", err
	}

	if err := yaml.Unmarshal(buf, vu); err != nil {
		return nil, "", fmt.Errorf("error parsing YAML file %s: %w", filename, err)
	}

	return vu, apiVersion, nil
}

// DecodeValidateUpgrade parses the given YAML contents of r into a struct,
// then repeatedly calls Upgrade() and Validate() on it until it's the newest version, then
// returns it. requireKind has the same meaning as in Decode().
func DecodeValidateUpgrade(ctx context.Context, r io.Reader, filename, requireKind string) (model.ValidatorUpgrader, error) {
	vu, apiVersion, err := Decode(r, filename, requireKind)
	if err != nil {
		return nil, err
	}

	if err := vu.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed in %s: %w", filename, err)
	}

	for {
		upgraded, err := vu.Upgrade(ctx)
		if err != nil {
			if errors.Is(err, model.ErrLatestVersion) {
				return vu, nil
			}
			return nil, fmt.Errorf("internal error: YAML model couldn't be upgraded from api_version %s: %w", apiVersion, err)
		}
		vu = upgraded
		if err := vu.Validate(); err != nil {
			// If there's a validation error after an upgrade after a successful
			// validation, that means there's a broken Upgrade() function that
			// created an invalid struct. This isn't the user's fault, there's a
			// bug in Upgrade().
			return nil, fmt.Errorf("internal error: validation failed after automatic schema upgrade from %s in %s: %w", apiVersion, filename, err)
		}
	}
}

// commonFields is the set fields that are present in every "kind" of YAML file
// used in this program.
type commonFields struct {
	OldStyleAPIVersion model.String `yaml:"api_version"`
	NewStyleAPIVersion model.String `yaml:"apiVersion"`
	Kind               string       `yaml:"kind"`
}

// modelForKind returns an instance of the YAML struct for the given API version and kind.
func modelForKind(filename, apiVersion, kind string) (model.ValidatorUpgrader, error) {
	idx := slices.IndexFunc(apiVersions, func(v apiVersionDef) bool {
		return v.apiVersion == apiVersion
	})
	if idx == -1 {
		return nil, fmt.Errorf("file %s has unknown api_version %q; you might need to upgrade your abc CLI. See https://github.com/abcxyz/abc/#installation", filename, apiVersion)
	}

	versionDef := apiVersions[idx]

	archetype, ok := versionDef.kinds[kind]
	if !ok {
		return nil, fmt.Errorf("file %s has kind %q that is not known in API version %q; you might need to use a more recent API version or fix the kind field", filename, kind, apiVersion)
	}

	// Make a copy; we don't want to modify the global archetype struct that's
	// stored in apiVersions.
	vu, ok := reflect.New(reflect.TypeOf(archetype).Elem()).Interface().(model.ValidatorUpgrader)
	if !ok {
		return nil, fmt.Errorf("internal error: type-assertion to ValidatorUpgrader failed")
	}
	return vu, nil
}
