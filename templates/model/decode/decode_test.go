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
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/exp/maps"

	"github.com/abcxyz/abc/templates/model"
	goldentestfeatures "github.com/abcxyz/abc/templates/model/goldentest/features"
	goldentestv1alpha1 "github.com/abcxyz/abc/templates/model/goldentest/v1alpha1"
	goldentestv1beta4 "github.com/abcxyz/abc/templates/model/goldentest/v1beta4"
	manifestv1alpha1 "github.com/abcxyz/abc/templates/model/manifest/v1alpha1"
	specfeatures "github.com/abcxyz/abc/templates/model/spec/features"
	specv1alpha1 "github.com/abcxyz/abc/templates/model/spec/v1alpha1"
	specv1beta6 "github.com/abcxyz/abc/templates/model/spec/v1beta6"
	mdl "github.com/abcxyz/abc/templates/testutil/model"
	"github.com/abcxyz/pkg/sets"
	"github.com/abcxyz/pkg/testutil"
)

func TestDecode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		requireKind    string
		fileContents   string
		isReleaseBuild bool
		want           model.ValidatorUpgrader
		wantVersion    string
		wantErr        string
	}{
		{
			name:        "oldest_template",
			requireKind: KindTemplate,
			fileContents: `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'mydesc'
steps:
  - action: 'include'
    desc: 'include all files'
    params:
      paths: ['.']`,
			want: &specv1alpha1.Spec{
				Desc: mdl.S("mydesc"),
				Steps: []*specv1alpha1.Step{
					{
						Action: mdl.S("include"),
						Desc:   mdl.S("include all files"),
						Include: &specv1alpha1.Include{
							Paths: []*specv1alpha1.IncludePath{
								{
									Paths: mdl.Strings("."),
								},
							},
						},
					},
				},
			},
			wantVersion: "cli.abcxyz.dev/v1alpha1",
		},
		{
			name:        "oldest_golden_test",
			requireKind: KindGoldenTest,
			fileContents: `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'GoldenTest'
inputs:
  - name: 'foo'
    value: 'bar'`,
			want: &goldentestv1alpha1.Test{
				Inputs: []*goldentestv1alpha1.VarValue{
					{
						Name:  mdl.S("foo"),
						Value: mdl.S("bar"),
					},
				},
			},
			wantVersion: "cli.abcxyz.dev/v1alpha1",
		},
		{
			name:        "oldest_manifest",
			requireKind: KindManifest,
			fileContents: `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Manifest'
template_location: 'foo'
template_dirhash: 'bar'`,
			want: &manifestv1alpha1.Manifest{
				TemplateLocation: mdl.S("foo"),
				TemplateDirhash:  mdl.S("bar"),
			},
			wantVersion: "cli.abcxyz.dev/v1alpha1",
		},
		{
			name:        "newest_template",
			requireKind: KindTemplate,
			fileContents: `api_version: 'cli.abcxyz.dev/v1beta6'
kind: 'Template'
desc: 'mydesc'
steps:
  - action: 'include'
    desc: 'include all files'
    if: 'true'
    params:
      paths: ['.']`,
			want: &specv1beta6.Spec{
				Desc: mdl.S("mydesc"),
				Steps: []*specv1beta6.Step{
					{
						Action: mdl.S("include"),
						If:     mdl.S("true"),
						Desc:   mdl.S("include all files"),
						Include: &specv1beta6.Include{
							Paths: []*specv1beta6.IncludePath{
								{
									Paths: mdl.Strings("."),
								},
							},
						},
					},
				},
			},
			wantVersion: "cli.abcxyz.dev/v1beta6",
		},
		{
			name:        "newest_golden_test",
			requireKind: KindGoldenTest,
			fileContents: `api_version: 'cli.abcxyz.dev/v1beta5'
kind: 'GoldenTest'
inputs:
  - name: 'foo'
    value: 'bar'
builtin_vars:
  - name: '_git_tag'
    value: 'my-cool-tag'`,
			want: &goldentestv1beta4.Test{
				Inputs: []*goldentestv1beta4.VarValue{
					{
						Name:  mdl.S("foo"),
						Value: mdl.S("bar"),
					},
				},
				BuiltinVars: []*goldentestv1beta4.VarValue{
					{
						Name:  mdl.S("_git_tag"),
						Value: mdl.S("my-cool-tag"),
					},
				},
			},
			wantVersion: "cli.abcxyz.dev/v1beta5",
		},
		{
			name:        "newest_manifest",
			requireKind: KindManifest,
			fileContents: `api_version: 'cli.abcxyz.dev/v1beta1'
kind: 'Manifest'
template_location: 'foo'
template_dirhash: 'bar'`,
			want: &manifestv1alpha1.Manifest{
				TemplateLocation: mdl.S("foo"),
				TemplateDirhash:  mdl.S("bar"),
			},
			wantVersion: "cli.abcxyz.dev/v1beta1",
		},
		{
			name:        "requireKind_is_empty",
			requireKind: "",
			fileContents: `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'mydesc'
steps:
  - action: 'include'
    desc: 'include all files'
    params:
      paths: ['.']`,
			want: &specv1alpha1.Spec{
				Desc: mdl.S("mydesc"),
				Steps: []*specv1alpha1.Step{
					{
						Action: mdl.S("include"),
						Desc:   mdl.S("include all files"),
						Include: &specv1alpha1.Include{
							Paths: []*specv1alpha1.IncludePath{
								{
									Paths: mdl.Strings("."),
								},
							},
						},
					},
				},
			},
			wantVersion: "cli.abcxyz.dev/v1alpha1",
		},
		{
			name:        "incorrect_requireKind",
			requireKind: KindTemplate,
			fileContents: `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Manifest'
desc: 'mydesc'
steps:
  - action: 'include'
    params:
      paths: ['.']`,
			wantVersion: "cli.abcxyz.dev/v1alpha1",
			wantErr:     `has kind "Manifest", but "Template" is required`,
		},
		{
			name:         "malformed_yaml",
			fileContents: `*&^*&^*&^`,
			wantErr:      "error parsing file file.yaml: yaml: ",
		},
		{
			name:         "missing_api_version",
			fileContents: `kind: 'Template'`,
			wantErr:      `file file.yaml must set the field "api_version"`,
		},
		{
			name:         "missing_kind",
			fileContents: `api_version: 'cli.abcxyz.dev/v1alpha1'`,
			wantErr:      `file file.yaml must set the field "kind"`,
		},
		{
			name: "api_version_snake_case",
			fileContents: `api_version: 'cli.abcxyz.dev/v1beta1'
kind: 'GoldenTest'`,
			want:        &goldentestv1alpha1.Test{},
			wantVersion: "cli.abcxyz.dev/v1beta1",
		},
		{
			name: "api_version_camel_case",
			fileContents: `apiVersion: 'cli.abcxyz.dev/v1beta1'
kind: 'GoldenTest'`,
			want:        &goldentestv1alpha1.Test{},
			wantVersion: "cli.abcxyz.dev/v1beta1",
		},
		{
			name: "api_version_snake_and_camel_case",
			fileContents: `apiVersion: 'cli.abcxyz.dev/v1beta1'
api_version: 'cli.abcxyz.dev/v1beta1'
kind: 'GoldenTest'`,
			wantErr: `must not set both apiVersion and api_version, please use api_version only`,
		},
		{
			name:        "speculative_upgrade_template",
			requireKind: KindTemplate,
			fileContents: `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'mydesc'
steps:
  - action: 'include'
    desc: 'include all files'
    if: 'true'
    params:
      paths: ['.']`,
			want: &specv1beta6.Spec{
				Desc: mdl.S("mydesc"),
				Steps: []*specv1beta6.Step{
					{
						Action: mdl.S("include"),
						If:     mdl.S("true"),
						Desc:   mdl.S("include all files"),
						Include: &specv1beta6.Include{
							Paths: []*specv1beta6.IncludePath{
								{
									Paths: mdl.Strings("."),
								},
							},
						},
					},
				},
			},
			wantErr: `file file.yaml sets api_version "cli.abcxyz.dev/v1alpha1" but does not parse and validate successfully under that version. However, it will be valid if you change the api_version`,
		},
		{
			name:        "speculative_upgrade_goldentest",
			requireKind: KindGoldenTest,
			fileContents: `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'GoldenTest'
builtin_vars:
- name: '_git_tag'
  value: 'foo'`,
			want: &goldentestv1beta4.Test{
				BuiltinVars: []*goldentestv1beta4.VarValue{
					{
						Name:  mdl.S("_git_tag"),
						Value: mdl.S("foo"),
					},
				},
			},
			wantErr: `file file.yaml sets api_version "cli.abcxyz.dev/v1alpha1" but does not parse and validate successfully under that version. However, it will be valid if you change the api_version`,
		},
		{
			name:        "template_exceeds_latest_supported_api_version",
			requireKind: KindTemplate,
			fileContents: `api_version: 'cli.abcxyz.dev/v1beta7'
kind: 'Template'
desc: 'mydesc'
steps:
  - action: 'include'
    desc: 'include all files'
    if: 'true'
    params:
      paths: ['.']`,
			isReleaseBuild: true,
			wantErr:        `api_version "cli.abcxyz.dev/v1beta7" is not supported in this version of abc; you might need to upgrade. See https://github.com/abcxyz/abc/#installation`,
		},
		{
			name:        "golden_test_exceeds_latest_supported_api_version",
			requireKind: KindGoldenTest,
			fileContents: `api_version: 'cli.abcxyz.dev/v1beta7'
kind: 'GoldenTest'
inputs:
    - name: 'foo'
      value: 'bar'
builtin_vars:
    - name: '_git_tag'
      value: 'my-cool-tag'`,
			isReleaseBuild: true,
			wantErr:        `api_version "cli.abcxyz.dev/v1beta7" is not supported in this version of abc; you might need to upgrade. See https://github.com/abcxyz/abc/#installation`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, gotVersion, _, err := Decode(strings.NewReader(tc.fileContents), "file.yaml", tc.requireKind, tc.isReleaseBuild)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}
			if err != nil {
				return
			}

			opt := cmpopts.IgnoreTypes(&model.ConfigPos{}, model.ConfigPos{}) // don't force test authors to assert the line and column numbers
			if diff := cmp.Diff(tc.want, got, opt); diff != "" {
				t.Fatalf("model struct wasn't as expected (-got,+want): %s", diff)
			}
			if gotVersion != tc.wantVersion {
				t.Fatalf("got api_version %q, want %q", gotVersion, tc.wantVersion)
			}
		})
	}
}

func TestDecodeValidateUpgrade(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		fileContents string
		want         model.ValidatorUpgrader
		wantErr      string
	}{
		{
			name: "oldest_template_upgrades_to_newest",
			fileContents: `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'mydesc'
steps:
  - action: 'include'
    desc: 'step desc'
    params:
      paths: ['.']`,
			want: &specv1beta6.Spec{
				Desc: mdl.S("mydesc"),
				Features: specfeatures.Features{
					SkipGlobs:   true,
					SkipGitVars: true,
					SkipTime:    true,
				},
				Steps: []*specv1beta6.Step{
					{
						Action: mdl.S("include"),
						Desc:   mdl.S("step desc"),
						Include: &specv1beta6.Include{
							Paths: []*specv1beta6.IncludePath{
								{
									Paths: mdl.Strings("."),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "oldest_goldentest_upgrades_to_newest",
			fileContents: `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'GoldenTest'
inputs:
  - name: 'foo'
    value: 'bar'`,
			want: &goldentestv1beta4.Test{
				Inputs: []*goldentestv1beta4.VarValue{
					{
						Name:  mdl.S("foo"),
						Value: mdl.S("bar"),
					},
				},
				Features: goldentestfeatures.Features{
					SkipStdout:     true,
					SkipABCRenamed: true,
				},
			},
		},
		{
			name: "oldest_manifest_upgrades_to_newest",
			fileContents: `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Manifest'
template_location: 'foo'
template_dirhash: 'bar'`,
			want: &manifestv1alpha1.Manifest{
				TemplateLocation: mdl.S("foo"),
				TemplateDirhash:  mdl.S("bar"),
			},
		},
		{
			name: "validation_failure_oldest",
			fileContents: `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'`,
			wantErr: `validation failed in file.yaml: at line 1 column 1: field "desc" is required`,
		},
		{
			name: "validation_failure_newest",
			fileContents: `api_version: 'cli.abcxyz.dev/v1beta1'
kind: 'Template'`,
			wantErr: `validation failed in file.yaml: at line 1 column 1: field "desc" is required`,
		},
		{
			name: "oldest_template_rules_survive_upgrade",
			fileContents: `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'mydesc'

inputs:
  - name: 'foo'
    desc: 'The name parameter'
    rules:
      - rule: 'size(foo) < 10'
        message: 'name length must be less than 10'

steps:
  - action: 'include'
    desc: 'step desc'
    params:
      paths: ['.']`,
			want: &specv1beta6.Spec{
				Desc: mdl.S("mydesc"),
				Features: specfeatures.Features{
					SkipGlobs:   true,
					SkipGitVars: true,
					SkipTime:    true,
				},
				Inputs: []*specv1beta6.Input{
					{
						Name: mdl.S("foo"),
						Desc: mdl.S("The name parameter"),
						Rules: []*specv1beta6.Rule{
							{
								Rule:    mdl.S("size(foo) < 10"),
								Message: mdl.S("name length must be less than 10"),
							},
						},
					},
				},
				Steps: []*specv1beta6.Step{
					{
						Action: mdl.S("include"),
						Desc:   mdl.S("step desc"),
						Include: &specv1beta6.Include{
							Paths: []*specv1beta6.IncludePath{
								{
									Paths: mdl.Strings("."),
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			rd := strings.NewReader(tc.fileContents)
			vu, _, err := DecodeValidateUpgrade(ctx, rd, "file.yaml", "")
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}
			if err != nil {
				return
			}

			opts := []cmp.Option{
				cmpopts.IgnoreTypes(&model.ConfigPos{}, model.ConfigPos{}), // don't force test authors to assert the line and column numbers
				cmpopts.EquateEmpty(), // the "copier" library turns nil slices into empty slices :/
			}
			if diff := cmp.Diff(vu, tc.want, opts...); diff != "" {
				t.Errorf("output wasn't as expected (-got,+want): %s", diff)
			}
		})
	}
}

// The list of API versions should not have any entries with the same version
// string.
func TestAPIVersions_NoDupes(t *testing.T) {
	t.Parallel()

	seen := map[string]struct{}{}
	for _, entry := range apiVersions {
		if _, ok := seen[entry.apiVersion]; ok {
			t.Errorf("API version %q appears twice in apiVersions", entry.apiVersion)
		}
		seen[entry.apiVersion] = struct{}{}
	}
}

// The set of kinds should either grow or stay the same with a new api version.
// This test guards against somebody accidentally removing support for a
// particular kind when adding a new API version.
func TestAPIVersions_NoDroppedKinds(t *testing.T) {
	t.Parallel()

	var wantKinds []string
	for _, entry := range apiVersions {
		kindsThisVersion := maps.Keys(entry.kinds)
		if missing := sets.Subtract(wantKinds, kindsThisVersion); len(missing) > 0 {
			t.Fatalf("apiVersion %q should have an entry the kind(s) %q that existed in a previous version",
				entry.apiVersion, missing)
		}
		wantKinds = kindsThisVersion
	}
}

func TestAPIVersions_ArchetypesArePointers(t *testing.T) {
	t.Parallel()

	for _, entry := range apiVersions {
		for _, archetype := range entry.kinds {
			if reflect.TypeOf(archetype).Kind() != reflect.Pointer {
				t.Errorf("apiVersion for %q had an archetype %T that should have been a pointer",
					entry.apiVersion, archetype)
			}
		}
	}
}

func TestLatestSupportedAPIVersion(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		isReleaseBuild bool
		want           string
	}{
		{
			name:           "is_release_build",
			isReleaseBuild: true,
			want:           "cli.abcxyz.dev/v1beta6", // update for each api_version release
		},
		{
			name:           "not_release_build",
			isReleaseBuild: false,
			want:           "cli.abcxyz.dev/v1beta6", // update for creation of a new api_version
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := LatestSupportedAPIVersion(tc.isReleaseBuild)
			if got != tc.want {
				t.Errorf("LatestSupportedAPIVersion(%t)=%q, want %q",
					tc.isReleaseBuild, got, tc.want)
			}
		})
	}
}
