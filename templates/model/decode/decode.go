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

	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"

	"github.com/abcxyz/abc/internal/version"
	"github.com/abcxyz/abc/templates/model"
	goldentestv1alpha1 "github.com/abcxyz/abc/templates/model/goldentest/v1alpha1"
	goldentestv1beta3 "github.com/abcxyz/abc/templates/model/goldentest/v1beta3"
	"github.com/abcxyz/abc/templates/model/header"
	manifestv1alpha1 "github.com/abcxyz/abc/templates/model/manifest/v1alpha1"
	specv1alpha1 "github.com/abcxyz/abc/templates/model/spec/v1alpha1"
	specv1beta1 "github.com/abcxyz/abc/templates/model/spec/v1beta1"
	specv1beta2 "github.com/abcxyz/abc/templates/model/spec/v1beta2"
	specv1beta3 "github.com/abcxyz/abc/templates/model/spec/v1beta3"
	specv1beta4 "github.com/abcxyz/abc/templates/model/spec/v1beta4"
)

var (
	KindTemplate   = "Template"   // the value of the "kind" field in a spec.yaml file
	KindGoldenTest = "GoldenTest" // ... a test.yaml file
	KindManifest   = "Manifest"   // ... a manifest.yaml file
)

type apiVersionDef struct {
	apiVersion string

	// Set this to true for api_versions that are still under construction and
	// should not be supported in official release builds. We don't want real
	// users using unreleased api_versions which are still under construction
	// and may receive breaking changes.
	unreleased bool

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
	{
		apiVersion: "cli.abcxyz.dev/v1beta2",
		kinds: map[string]model.ValidatorUpgrader{
			KindTemplate:   &specv1beta2.Spec{},
			KindGoldenTest: &goldentestv1alpha1.Test{},
			KindManifest:   &manifestv1alpha1.Manifest{},
		},
	},
	{
		apiVersion: "cli.abcxyz.dev/v1beta3",
		kinds: map[string]model.ValidatorUpgrader{
			KindTemplate:   &specv1beta3.Spec{},
			KindGoldenTest: &goldentestv1beta3.Test{},
			KindManifest:   &manifestv1alpha1.Manifest{},
		},
	},
	{
		apiVersion: "cli.abcxyz.dev/v1beta5",
		unreleased: true,
		kinds: map[string]model.ValidatorUpgrader{
			KindTemplate:   &specv1beta4.Spec{},
			KindGoldenTest: &goldentestv1beta3.Test{},
			KindManifest:   &manifestv1alpha1.Manifest{},
		},
	},
}

// Decode parses the given YAML contents of r into a struct and returns it. The
// given filename is used only for error messages. The type of struct to return
// is determined by the "kind" field in the YAML. If the given requireKind is
// non-empty, then we'll also validate that the "kind" of the YAML file matches
// requireKind, and return error if not. This also calls Validate() on
// the returned struct and returns error if invalid.
func Decode(r io.Reader, filename, requireKind string) (model.ValidatorUpgrader, string, error) {
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, "", fmt.Errorf("error reading file %s: %w", filename, err)
	}

	cf := &header.Fields{}
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

	if cf.Kind.Val == "" {
		return nil, "", fmt.Errorf(`file %s must set the field "kind"`, filename)
	}
	if requireKind != "" && cf.Kind.Val != requireKind {
		return nil, "", fmt.Errorf("file %s has kind %q, but %q is required", filename, cf.Kind.Val, requireKind)
	}

	vu, err := decodeFromVersionKind(filename, apiVersion, cf.Kind.Val, buf)
	if err == nil {
		return vu, apiVersion, nil
	}

	// Parsing or validation failed. We'll try to detect a common user error
	// that might have caused this problem and print a helpful message. The user
	// might be trying to use a new feature, but the api_version declared in
	// their YAML file doesn't support that new feature. To detect this, we'll
	// speculatively try to parse the YAML file with a newer api_version and see
	// if that version would have been valid. If so, we inform the user that
	// they should change the api_version field in their YAML file.
	attemptAPIVersion := apiVersions[len(apiVersions)-1].apiVersion
	if attemptAPIVersion == apiVersion {
		return nil, "", err // api_version upgrade isn't possible, they're already on the latest.
	}
	if _, attemptErr := decodeFromVersionKind(filename, attemptAPIVersion, cf.Kind.Val, buf); attemptErr == nil {
		return nil, "", fmt.Errorf("file %s sets api_version %q but does not parse and validate successfully under that version. However, it will be valid if you change the api_version to %q. The error was: %w",
			filename, apiVersion, attemptAPIVersion, err)
	}

	return nil, "", err
}

// DecodeValidateUpgrade parses the given YAML contents of r into a struct,
// then repeatedly calls Upgrade() and Validate() on it until it's the newest version, then
// returns it. requireKind has the same meaning as in Decode().
func DecodeValidateUpgrade(ctx context.Context, r io.Reader, filename, requireKind string) (model.ValidatorUpgrader, error) {
	vu, apiVersion, err := Decode(r, filename, requireKind)
	if err != nil {
		return nil, err
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

// decodeFromVersionKind returns an instance of the YAML struct for the given API version and kind.
// It also validates the resulting struct.
func decodeFromVersionKind(filename, apiVersion, kind string, buf []byte) (model.ValidatorUpgrader, error) {
	idx := slices.IndexFunc(apiVersions, func(v apiVersionDef) bool {
		return v.apiVersion == apiVersion
	})
	if idx == -1 {
		return nil, fmt.Errorf("file %s has unknown api_version %q; you might need to upgrade your abc CLI. See https://github.com/abcxyz/abc/#installation", filename, apiVersion)
	}

	versionDef := apiVersions[idx]
	if versionDef.unreleased && version.IsReleaseBuild() {
		return nil, fmt.Errorf("api_version %q is not supported in this version of abc; you might need to upgrade. See https://github.com/abcxyz/abc/#installation", apiVersion)
	}

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

	if err := yaml.Unmarshal(buf, vu); err != nil {
		return nil, fmt.Errorf("error parsing YAML file %s: %w", filename, err)
	}

	if err := vu.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed in %s: %w", filename, err)
	}

	return vu, nil
}

// LatestSupportedAPIVersion is the most up-to-date API version. It's
// in the format "cli.abcxyz.dev/v1beta4".
//
// isReleaseBuild is the value of version.IsReleaseBuild(), but for testing
// purposes we make it an argument rather than hardcoding.
func LatestSupportedAPIVersion(isReleaseBuild bool) string {
	// Release builds (like "I am the official release of version 1.2.3") will
	// read and write only officially released, finalized api_versions. Other
	// builds (e.g. CI builds, local dev builds, devs running "go test" on
	// workstations) are more permissive and will read and write the most recent
	// unreleased work-in-progress api_version.
	if !isReleaseBuild {
		return apiVersions[len(apiVersions)-1].apiVersion
	}
	for i := len(apiVersions) - 1; i > 0; i-- {
		if !apiVersions[i].unreleased {
			return apiVersions[i].apiVersion
		}
	}
	// Justification for why it's OK to panic here: if this passes unit tests,
	// it will never fail on a user's machine.
	panic("internal error: there are no apiVersions that are marked as released")
}
