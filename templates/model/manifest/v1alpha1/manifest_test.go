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

package manifest

import (
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gopkg.in/yaml.v3"

	"github.com/abcxyz/abc/templates/model"
	mdl "github.com/abcxyz/abc/templates/testutil/model"
	"github.com/abcxyz/pkg/testutil"
)

func TestDecode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		in               string
		want             *Manifest
		wantUnmarshalErr string
		wantValidateErr  []string
	}{
		{
			name: "simple_success",
			in: `
api_version: 'cli.abcxyz.dev/v1alpha1'
template_location: 'github.com/abcxyz/abc/t/rest_server@latest'
template_dirhash: 'h1:5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03'
template_version: 'v1.2.3'
inputs:
  - name: 'my_input_1'
    value: 'my_value_1'
  - name: 'my_input_2'
    value: 'my_value_2'
output_files:
  - file: 'a/b/c.txt'
    hash: 'h1:b5bb9d8014a0f9b1d61e21e796d78dccdf1352f23cd32812f4850b878ae4944c'
    patch: |
        --- a/myfile.txt
        +++ b/myfile.txt
        @@ -1 +1 @@
        -red is my favorite color
        +purple is my favorite color
  - file: 'd/e/f.txt'
    hash: 'h1:7d865e959b2466918c9863afca942d0fb89d7c9ac0c99bafc3749504ded97730'`,
			want: &Manifest{
				TemplateLocation: mdl.S("github.com/abcxyz/abc/t/rest_server@latest"),
				TemplateDirhash:  mdl.S("h1:5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03"),
				TemplateVersion:  mdl.S("v1.2.3"),
				Inputs: []*Input{
					{
						Name:  mdl.S("my_input_1"),
						Value: mdl.S("my_value_1"),
					},
					{
						Name:  mdl.S("my_input_2"),
						Value: mdl.S("my_value_2"),
					},
				},
				OutputFiles: []*OutputFile{
					{
						File: mdl.S("a/b/c.txt"),
						Hash: mdl.S("h1:b5bb9d8014a0f9b1d61e21e796d78dccdf1352f23cd32812f4850b878ae4944c"),
						Patch: mdl.SP(`--- a/myfile.txt
+++ b/myfile.txt
@@ -1 +1 @@
-red is my favorite color
+purple is my favorite color
`),
					},
					{
						File: mdl.S("d/e/f.txt"),
						Hash: mdl.S("h1:7d865e959b2466918c9863afca942d0fb89d7c9ac0c99bafc3749504ded97730"),
					},
				},
			},
		},
		{
			name: "fields_missing",
			in:   `api_version: "foo"`,
			wantValidateErr: []string{
				`at line 1 column 1: field "template_dirhash" is required`,
			},
		},
		{
			name: "input_missing_name",
			in: `
api_version: 'cli.abcxyz.dev/v1alpha1'
template_location: 'github.com/abcxyz/abc/t/rest_server@latest'
template_dirhash: 'h1:5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03'
inputs:
  - value: 'my_value_1'
output_files:
  - file: 'a/b/c.txt'
    hash: 'h1:b5bb9d8014a0f9b1d61e21e796d78dccdf1352f23cd32812f4850b878ae4944c'`,
			wantValidateErr: []string{`at line 6 column 5: field "name" is required`},
		},
		{
			name: "missing_file",
			in: `
api_version: 'cli.abcxyz.dev/v1alpha1'
template_location: 'github.com/abcxyz/abc/t/rest_server@latest'
template_dirhash: 'h1:5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03'
inputs:
  - name: 'my_input_1'
    value: 'my_value_1'
output_files:
  - hash: 'h1:b5bb9d8014a0f9b1d61e21e796d78dccdf1352f23cd32812f4850b878ae4944c'`,
			wantValidateErr: []string{`at line 9 column 5: field "file" is required`},
		},
		{
			name: "missing_hash",
			in: `
api_version: 'cli.abcxyz.dev/v1alpha1'
template_location: 'github.com/abcxyz/abc/t/rest_server@latest'
template_dirhash: 'h1:5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03'
inputs:
  - name: 'my_input_1'
    value: 'my_value_1'
output_files:
  - file: 'a/b/c.txt'`,
			wantValidateErr: []string{`at line 9 column 5: field "hash" is required`},
		},
		{
			name: "no_files", // It's rare but legal for a template to have no output files
			in: `
api_version: 'cli.abcxyz.dev/v1alpha1'
template_location: 'github.com/abcxyz/abc/t/rest_server@latest'
template_dirhash: 'h1:5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03'
inputs:
  - name: 'my_input_1'
    value: 'my_value_1'
`,
			want: &Manifest{
				TemplateLocation: mdl.S("github.com/abcxyz/abc/t/rest_server@latest"),
				TemplateDirhash:  mdl.S("h1:5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03"),
				Inputs: []*Input{
					{
						Name:  mdl.S("my_input_1"),
						Value: mdl.S("my_value_1"),
					},
				},
			},
		},
		{
			name: "no_inputs", // It's legal for a template to have no inputs
			in: `
api_version: 'cli.abcxyz.dev/v1alpha1'
template_location: 'github.com/abcxyz/abc/t/rest_server@latest'
template_dirhash: 'h1:5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03'
output_files:
  - file: 'a/b/c.txt'
    hash: 'h1:b5bb9d8014a0f9b1d61e21e796d78dccdf1352f23cd32812f4850b878ae4944c'`,
			want: &Manifest{
				TemplateLocation: mdl.S("github.com/abcxyz/abc/t/rest_server@latest"),
				TemplateDirhash:  mdl.S("h1:5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03"),
				OutputFiles: []*OutputFile{
					{
						File: mdl.S("a/b/c.txt"),
						Hash: mdl.S("h1:b5bb9d8014a0f9b1d61e21e796d78dccdf1352f23cd32812f4850b878ae4944c"),
					},
				},
			},
		},
		{
			name:             "bad_yaml_syntax",
			in:               `[[[[[[[`,
			wantUnmarshalErr: "did not find expected node content",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := &Manifest{}
			dec := yaml.NewDecoder(strings.NewReader(tc.in))
			err := dec.Decode(got)

			if diff := testutil.DiffErrString(err, tc.wantUnmarshalErr); diff != "" {
				t.Fatal(diff)
			}
			if err != nil {
				return
			}

			err = got.Validate()
			for _, wantValidateErr := range tc.wantValidateErr {
				if diff := testutil.DiffErrString(err, wantValidateErr); diff != "" {
					t.Fatal(diff)
				}
			}
			if err != nil {
				return
			}

			opt := cmpopts.IgnoreTypes(&model.ConfigPos{}, model.ConfigPos{}) // don't force test authors to assert the line and column numbers
			if diff := cmp.Diff(got, tc.want, opt); diff != "" {
				t.Errorf("unmarshaling didn't yield expected struct. Diff (-got +want): %s", diff)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string

		// For ease of writing test cases, each test case receives a valid
		// manifest and can alter it to make it invalid in a specific way to be
		// tested.
		mutate func(valid *Manifest)

		in      *Manifest
		wantErr string
	}{
		{
			name:   "valid_manifest_accepted",
			mutate: func(in *Manifest) {},
		},
		{
			name: "empty_dirhash",
			mutate: func(in *Manifest) {
				in.TemplateDirhash.Val = ""
			},
			wantErr: `"template_dirhash" is required`,
		},
		{
			name: "missing_input_name",
			mutate: func(in *Manifest) {
				in.Inputs[0].Name.Val = ""
			},
			wantErr: `"name" is required`,
		},
		{
			name: "missing_file_hash",
			mutate: func(in *Manifest) {
				in.OutputFiles[0].Hash.Val = ""
			},
			wantErr: `"hash" is required`,
		},
		{
			name: "missing_file_name",
			mutate: func(in *Manifest) {
				in.OutputFiles[0].File.Val = ""
			},
			wantErr: `"file" is required`,
		},
		{
			name: "dot_dot_traversal",
			mutate: func(in *Manifest) {
				in.OutputFiles[0].File.Val = "../" + in.OutputFiles[0].File.Val
			},
			wantErr: `disallowed ".."`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			manifest := &Manifest{
				CreationTime:     time.Date(2024, time.June, 25, 11, 0, 0, 0, time.UTC),
				ModificationTime: time.Date(2024, time.June, 25, 12, 0, 0, 0, time.UTC),
				TemplateLocation: mdl.S("some_template_location"),
				LocationType:     mdl.S("some_location_type"),
				TemplateVersion:  mdl.S("some_version"),
				UpgradeChannel:   mdl.S("some_upgrade_channel"),
				TemplateDirhash:  mdl.S("some_dirhash"),
				Inputs: []*Input{
					{
						Name:  mdl.S("some_input_name"),
						Value: mdl.S("some_input_value"),
					},
				},
				OutputFiles: []*OutputFile{
					{
						File: mdl.S("some_output_file"),
						Hash: mdl.S("some_hash"),
					},
				},
			}

			tc.mutate(manifest)
			got := manifest.Validate()
			if diff := testutil.DiffErrString(got, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
