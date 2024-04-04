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

package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abcxyz/abc/templates/model/decode"
	manifest "github.com/abcxyz/abc/templates/model/manifest/v1alpha1"
)

// MustFindManifest is a wrapper around FindManifest that calls Fatal if there's
// an error or if there's no manifest.
func MustFindManifest(tb testing.TB, dir string) string {
	tb.Helper()

	basename, err := FindManifest(dir)
	if err != nil {
		tb.Fatal(err)
	}
	if basename == "" {
		tb.Fatalf("no manifest found in %q", dir)
	}
	return basename
}

// FindManifest finds a manifest file in the given directory. If there's no
// manifest file in that directory, then empty string is returned. If there are
// multiple manifests, an error is returned.
func FindManifest(dir string) (string, error) {
	joined := filepath.Join(dir, "manifest*.yaml")
	matches, err := filepath.Glob(joined)
	if err != nil {
		return "", err //nolint:wrapcheck
	}

	if len(matches) == 0 {
		return "", nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("multiple manifests were found in %q: %s", dir, strings.Join(matches, ", "))
	}

	return filepath.Base(matches[0]), nil
}

// MustLoadManifest parses the given manifest file.
func MustLoadManifest(ctx context.Context, tb testing.TB, path string) *manifest.Manifest {
	tb.Helper()

	f, err := os.Open(path)
	if err != nil {
		tb.Fatalf("failed to open manifest file at %q: %v", path, err)
	}
	defer f.Close()

	manifestI, err := decode.DecodeValidateUpgrade(ctx, f, path, decode.KindManifest)
	if err != nil {
		tb.Fatalf("error reading manifest file: %v", err)
	}

	out, ok := manifestI.(*manifest.Manifest)
	if !ok {
		tb.Fatalf("internal error: manifest file did not decode to *manifest.Manifest")
	}

	return out
}
