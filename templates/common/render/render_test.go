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

package render

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/builtinvar"
	"github.com/abcxyz/abc/templates/common/input"
	"github.com/abcxyz/abc/templates/common/paths"
	"github.com/abcxyz/abc/templates/model"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta3"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
)

func TestRender(t *testing.T) {
	t.Parallel()

	clk := clock.NewMock()
	// We don't use UTC time here because we want to make sure local time
	// gets converted to UTC time before saving.
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("time.LoadLocation(): %v", err)
	}
	clk.Set(time.Date(2023, 12, 8, 15, 59, 2, 13, loc))

	// Many (but not all) of the subtests use this spec.yaml.
	specContents := `
api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'A template for the ages'
inputs:
- name: 'name_to_greet'
  desc: 'A name to include in the message'
- name: 'emoji_suffix'
  desc: 'An emoji suffix to include in message'
- name: 'ending_punctuation'
  desc: 'The punctuation mark with which to end the message'
  default:  '.'
steps:
- desc: 'Print a message'
  action: 'print'
  params:
    message: 'Hello, {{.name_to_greet}}{{.emoji_suffix}}{{.ending_punctuation}}'
- desc: 'Include some files and directories'
  action: 'include'
  params:
    paths:
      - paths: ['file1.txt', 'dir1', 'dir2/file2.txt']
- desc: 'Replace "blue" with "red"'
  action: 'string_replace'
  params:
    paths: ['.']
    replacements:
    - to_replace: 'blue'
      with: 'red'
`

	cases := []struct {
		name                    string
		templateContents        map[string]string
		existingDestContents    map[string]string
		flagInputs              map[string]string
		inputFileNames          []string
		inputFileContents       map[string]string
		flagKeepTempDirs        bool
		flagForceOverwrite      bool
		flagSkipInputValidation bool
		flagManifest            bool
		flagDebugStepDiffs      bool
		overrideBuiltinVars     map[string]string
		removeAllErr            error
		wantScratchContents     map[string]string
		wantTemplateContents    map[string]string
		wantDestContents        map[string]string
		wantBackupContents      map[string]string
		wantStdout              string
		wantErr                 string
	}{
		{
			name: "simple_success_with_inputs_flag",
			flagInputs: map[string]string{
				"name_to_greet":      "Bob",
				"emoji_suffix":       "🐈",
				"ending_punctuation": "!",
			},
			templateContents: map[string]string{
				"myfile.txt":           "Some random stuff",
				"spec.yaml":            specContents,
				"file1.txt":            "my favorite color is blue",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantStdout: "Hello, Bob🐈!\n",
			wantDestContents: map[string]string{
				"file1.txt":            "my favorite color is red",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
		},
		{
			name: "simple_success_with_debug_flag",
			flagInputs: map[string]string{
				"name_to_greet":      "Bob",
				"emoji_suffix":       "🐈",
				"ending_punctuation": "!",
			},
			flagDebugStepDiffs: true,
			templateContents: map[string]string{
				"myfile.txt":           "Some random stuff",
				"spec.yaml":            specContents,
				"file1.txt":            "my favorite color is blue",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantStdout: "Hello, Bob🐈!\n",
			wantDestContents: map[string]string{
				"file1.txt":            "my favorite color is red",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
		},
		{
			name: "simple_success_with_manifest",
			flagInputs: map[string]string{
				"name_to_greet":      "Bob",
				"emoji_suffix":       "🐈",
				"ending_punctuation": "!",
			},
			templateContents: map[string]string{
				"myfile.txt":           "Some random stuff",
				"spec.yaml":            specContents,
				"file1.txt":            "my favorite color is blue",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			flagManifest: true,
			wantStdout:   "Hello, Bob🐈!\n",
			wantDestContents: map[string]string{
				"file1.txt":            "my favorite color is red",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
				".abc/manifest_nolocation_2023-12-08T23_59_02.000000013Z.lock.yaml": `# Generated by the "abc templates" command. Do not modify.
api_version: cli.abcxyz.dev/v1beta3
kind: Manifest
creation_time: 2023-12-08T23:59:02.000000013Z
modification_time: 2023-12-08T23:59:02.000000013Z
template_location: ""
location_type: ""
template_version: ""
template_dirhash: h1:Gym1rh37Q4e6h72ELjloc4lfVPR6B6tuRaLnFmakAYo=
inputs:
    - name: emoji_suffix
      value: "\U0001F408"
    - name: ending_punctuation
      value: '!'
    - name: name_to_greet
      value: Bob
output_hashes:
    - file: dir1/file_in_dir.txt
      hash: h1:IeeGbHh8lPKI7ISJDiQTcNzKT/kATZ6IBgL4PbzOE4M=
    - file: dir2/file2.txt
      hash: h1:AUDAxmpkSrLdJ6xVNvIMw3PW/RiW+YOOy0WVZ13aAfo=
    - file: file1.txt
      hash: h1:UQ18krF3vW1ggpVvzlSWqmU0l4Fsuskdq7PaT9KHZ/4=
`,
			},
		},
		{
			name: "simple_success_with_input_file_flag",

			inputFileNames: []string{"inputs.yaml"},
			inputFileContents: map[string]string{
				"inputs.yaml": `
name_to_greet: 'Bob'
emoji_suffix: '🐈'`,
			},
			templateContents: map[string]string{
				"myfile.txt":           "Some random stuff",
				"spec.yaml":            specContents,
				"file1.txt":            "my favorite color is blue",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantStdout: "Hello, Bob🐈.\n",
			wantDestContents: map[string]string{
				"file1.txt":            "my favorite color is red",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
		},
		{
			name:           "simple_success_with_both_inputs_and_input_file_flags",
			flagInputs:     map[string]string{"name_to_greet": "Robert"},
			inputFileNames: []string{"inputs.yaml"},
			inputFileContents: map[string]string{
				"inputs.yaml": `
name_to_greet: 'Bob'
emoji_suffix: '🐈'`,
			},
			templateContents: map[string]string{
				"myfile.txt":           "Some random stuff",
				"spec.yaml":            specContents,
				"file1.txt":            "my favorite color is blue",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantStdout: "Hello, Robert🐈.\n",
			wantDestContents: map[string]string{
				"file1.txt":            "my favorite color is red",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
		},
		{
			name:           "simple_success_with_two_input_file_flags",
			inputFileNames: []string{"inputs.yaml", "other-inputs.yaml"},
			inputFileContents: map[string]string{
				"inputs.yaml": `
name_to_greet: 'Bob'`,
				"other-inputs.yaml": `
emoji_suffix: '🐈'`,
			},
			templateContents: map[string]string{
				"myfile.txt":           "Some random stuff",
				"spec.yaml":            specContents,
				"file1.txt":            "my favorite color is blue",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantStdout: "Hello, Bob🐈.\n",
			wantDestContents: map[string]string{
				"file1.txt":            "my favorite color is red",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
		},
		{
			name:           "conflicting_input_files",
			inputFileNames: []string{"inputs.yaml", "other-inputs.yaml"},
			inputFileContents: map[string]string{
				"inputs.yaml":       `name_to_greet: 'Alice'`,
				"other-inputs.yaml": `name_to_greet: 'Bob'`,
			},
			templateContents: map[string]string{
				"spec.yaml": specContents,
			},
			wantErr: "input key \"name_to_greet\" appears in multiple input files",
		},
		{
			name: "keep_temp_dirs_on_success_if_flag",
			flagInputs: map[string]string{
				"name_to_greet": "Bob",
				"emoji_suffix":  "🐈",
			},
			flagKeepTempDirs: true,
			templateContents: map[string]string{
				"spec.yaml":            specContents,
				"file1.txt":            "my favorite color is blue",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantStdout: "Hello, Bob🐈.\n",
			wantScratchContents: map[string]string{
				"file1.txt":            "my favorite color is red",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantTemplateContents: map[string]string{
				"spec.yaml":            specContents,
				"file1.txt":            "my favorite color is blue",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantDestContents: map[string]string{
				"file1.txt":            "my favorite color is red",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
		},
		{
			name: "keep_temp_dirs_on_failure_if_flag",
			flagInputs: map[string]string{
				"name_to_greet": "Bob",
				"emoji_suffix":  "🐈",
			},
			flagKeepTempDirs: true,
			templateContents: map[string]string{
				"spec.yaml": "this is an unparseable YAML file *&^#%$",
			},
			wantTemplateContents: map[string]string{
				"spec.yaml": "this is an unparseable YAML file *&^#%$",
			},
			wantErr: "error parsing file spec.yaml",
		},
		{
			name: "existing_dest_file_with_overwrite_flag_should_succeed",
			flagInputs: map[string]string{
				"name_to_greet": "Bob",
				"emoji_suffix":  "🐈",
			},
			flagForceOverwrite: true,
			existingDestContents: map[string]string{
				"file1.txt": "old contents",
			},
			templateContents: map[string]string{
				"myfile.txt":           "Some random stuff",
				"spec.yaml":            specContents,
				"file1.txt":            "new contents",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantStdout: "Hello, Bob🐈.\n",
			wantDestContents: map[string]string{
				"file1.txt":            "new contents",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantBackupContents: map[string]string{
				"file1.txt": "old contents",
			},
		},
		{
			name: "existing_dest_file_without_overwrite_flag_should_fail",
			flagInputs: map[string]string{
				"name_to_greet": "Bob",
				"emoji_suffix":  "🐈",
			},
			flagForceOverwrite: false,
			existingDestContents: map[string]string{
				"file1.txt": "old contents",
			},
			templateContents: map[string]string{
				"myfile.txt":           "Some random stuff",
				"spec.yaml":            specContents,
				"file1.txt":            "file1 contents",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantStdout: "Hello, Bob🐈.\n",
			wantDestContents: map[string]string{
				"file1.txt": "old contents",
			},
			wantErr: "overwriting was not enabled",
		},
		{
			name:                 "fs_error",
			removeAllErr:         fmt.Errorf("fake removeAll error for testing"),
			wantTemplateContents: map[string]string{},
			wantErr:              "fake removeAll error for testing",
		},
		{
			name: "defaults_inputs",
			flagInputs: map[string]string{
				"name_to_greet": "Bob",
				"emoji_suffix":  "🐈",
			},
			templateContents: map[string]string{
				"myfile.txt":           "Some random stuff",
				"spec.yaml":            specContents,
				"file1.txt":            "file1 contents",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantStdout: "Hello, Bob🐈.\n",
			wantDestContents: map[string]string{
				"file1.txt":            "file1 contents",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
		},
		{
			name: "handles_unknown_inputs",
			flagInputs: map[string]string{
				"name_to_greet": "Bob",
				"emoji_suffix":  "🐈",
				"pets_name":     "Fido",
				"pets_age":      "15",
			},
			templateContents: map[string]string{
				"myfile.txt":           "Some random stuff",
				"spec.yaml":            specContents,
				"file1.txt":            "file1 contents",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantErr: `unknown input(s): pets_age, pets_name`,
		},
		{
			name:       "handles_missing_required_inputs",
			flagInputs: map[string]string{},
			templateContents: map[string]string{
				"myfile.txt":           "Some random stuff",
				"spec.yaml":            specContents,
				"file1.txt":            "file1 contents",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantErr: `missing input(s): emoji_suffix, name_to_greet`,
		},
		{
			name: "plain_destination_include",
			templateContents: map[string]string{
				"spec.yaml": `
api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'my template'
steps:
  - desc: 'Include from destination'
    action: 'include'
    params:
        paths:
            - paths: ['myfile.txt', 'subdir_a', 'subdir_b/file_b.txt']
              from: 'destination'
  - desc: 'Replace "purple" with "red"'
    action: 'string_replace'
    params:
        paths: ['.']
        replacements:
          - to_replace: 'purple'
            with: 'red'`,
			},
			existingDestContents: map[string]string{
				"myfile.txt":          "purple is my favorite color",
				"subdir_a/file_a.txt": "purple is my favorite color",
				"subdir_b/file_b.txt": "purple is my favorite color",
			},
			wantDestContents: map[string]string{
				"myfile.txt":          "red is my favorite color",
				"subdir_a/file_a.txt": "red is my favorite color",
				"subdir_b/file_b.txt": "red is my favorite color",
			},
			wantBackupContents: map[string]string{
				"myfile.txt":          "purple is my favorite color",
				"subdir_a/file_a.txt": "purple is my favorite color",
				"subdir_b/file_b.txt": "purple is my favorite color",
			},
		},
		{
			name: "mix_of_destination_include_and_normal_include",
			templateContents: map[string]string{
				"file_b.txt": "red is my favorite color",
				"spec.yaml": `
api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'my template'
steps:
  - desc: 'Include from destination'
    action: 'include'
    params:
        paths:
            - paths: ['file_a.txt']
              from: 'destination'
  - desc: 'Include from template'
    action: 'include'
    params:
        paths:
            - paths: ['file_b.txt']
  - desc: 'Replace "purple" with "red"'
    action: 'string_replace'
    params:
        paths: ['.']
        replacements:
          - to_replace: 'purple'
            with: 'red'`,
			},
			existingDestContents: map[string]string{
				"file_a.txt": "purple is my favorite color",
			},
			wantDestContents: map[string]string{
				"file_a.txt": "red is my favorite color",
				"file_b.txt": "red is my favorite color",
			},
			wantBackupContents: map[string]string{
				"file_a.txt": "purple is my favorite color",
			},
		},
		{
			name: "with_default_ignore",
			templateContents: map[string]string{
				"dir/file_b.txt":          "red is my favorite color",
				".bin/file_to_ignore.txt": "src: file to ignore",
				"spec.yaml": `
api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'my template'
steps:
  - desc: 'Include from destination'
    action: 'include'
    params:
        paths:
            - paths: ['.']
              from: 'destination'
  - desc: 'Include from template'
    action: 'include'
    params:
        paths:
            - paths: ['.']
  - desc: 'Replace "purple" with "red"'
    action: 'string_replace'
    params:
        paths: ['.']
        replacements:
          - to_replace: 'purple'
            with: 'red'`,
			},
			existingDestContents: map[string]string{
				"file_a.txt":              "purple is my favorite color",
				".bin/file_to_ignore.txt": "dest: purple is my favorite color",
			},
			wantDestContents: map[string]string{
				"file_a.txt":              "red is my favorite color",
				"dir/file_b.txt":          "red is my favorite color",
				".bin/file_to_ignore.txt": "dest: purple is my favorite color",
			},
			wantBackupContents: map[string]string{
				"file_a.txt": "purple is my favorite color",
			},
		},
		{
			name: "with_custom_ignore",
			templateContents: map[string]string{
				"sub_dir/file_b.txt": "src: file to ignore",
				"spec.yaml": `
api_version: 'cli.abcxyz.dev/v1beta2'
kind: 'Template'
desc: 'my template'
ignore:
  - 'sub_dir/file_b.txt'
steps:
  - desc: 'Include from destination'
    action: 'include'
    params:
        paths:
            - paths: ['.']
              from: 'destination'
  - desc: 'Include from template'
    action: 'include'
    params:
        paths:
            - paths: ['sub_dir']
  - desc: 'Replace "purple" with "red"'
    action: 'string_replace'
    params:
        paths: ['.']
        replacements:
          - to_replace: 'purple'
            with: 'red'`,
			},
			existingDestContents: map[string]string{
				"file_a.txt":         "purple is my favorite color",
				"sub_dir/file_b.txt": "dest: purple is my favorite color",
			},
			wantDestContents: map[string]string{
				"file_a.txt":         "red is my favorite color",
				"sub_dir/file_b.txt": "dest: purple is my favorite color",
			},
			wantBackupContents: map[string]string{
				"file_a.txt": "purple is my favorite color",
			},
		},
		{
			name: "simple_skip",
			templateContents: map[string]string{
				"file1.txt": "file1 contents",
				"spec.yaml": `
api_version: 'cli.abcxyz.dev/v1beta1'
kind: 'Template'
desc: 'my template'
steps:
  - desc: 'include with skip'
    action: 'include'
    params:
      paths:
      - paths: ['file1.txt']
        skip: ['file1.txt']
`,
			},
			wantDestContents: map[string]string{},
		},
		{
			name: "glob_include",
			templateContents: map[string]string{
				"file1.txt":                  "file1 contents",
				"file2.txt":                  "file2 contents",
				"file3.txt":                  "file3 contents",
				"something.md":               "md contents",
				"something.json":             "json contents",
				"python_files/skip_1.py":     "skip 1 contents",
				"python_files/skip_2.py":     "skip 2 contents",
				"python_files/include_me.py": "include_me contents",
				"spec.yaml": `
api_version: 'cli.abcxyz.dev/v1beta2'
kind: 'Template'
desc: 'my template'
steps:
  - desc: 'Include glob'
    action: 'include'
    params:
      paths:
      - paths: ['*.txt']
      - paths: ['*.md', '*.json']
        as: ['dir1', 'dir2']
      - paths: ['python_files']
        skip: ['python_files/skip*']
`,
			},
			existingDestContents: map[string]string{
				"already_exists.pdf": "already existing file contents",
			},
			wantDestContents: map[string]string{
				"already_exists.pdf":         "already existing file contents",
				"file1.txt":                  "file1 contents",
				"file2.txt":                  "file2 contents",
				"file3.txt":                  "file3 contents",
				"dir1/something.md":          "md contents",
				"dir2/something.json":        "json contents",
				"python_files/include_me.py": "include_me contents",
			},
		},
		{
			name: "for_each",
			templateContents: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'A template for the ages'
steps:
  - desc: 'Iterate over environments'
    action: 'for_each'
    params:
      iterator:
        key: 'env'
        values: ['production', 'dev']
      steps:
        - desc: 'Print a message'
          action: 'print'
          params:
            message: 'Working on environment {{.env}}'
`,
			},
			wantStdout:       "Working on environment production\nWorking on environment dev\n",
			wantDestContents: map[string]string{},
		},
		{
			name: "skip_input_validation",
			templateContents: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'My template'
inputs:
  - name: 'my_input'
    desc: 'Just a string to print'
    rules:
      - rule: 'my_input > 42' # Would fail, except we disable validation

steps:
  - action: 'print'
    desc: 'print the input value'
    params:
      message: 'my_input is {{.my_input}}'
`,
			},
			flagInputs:              map[string]string{"my_input": "crocodile"},
			flagSkipInputValidation: true,
			wantStdout:              "my_input is crocodile\n",
			wantDestContents:        map[string]string{},
		},
		{
			name: "step_with_if",
			templateContents: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta1'
kind: 'Template'
desc: 'My template'

inputs:
  - name: 'my_input'
    desc: 'My input'
    default: 'true'

steps:
  - action: 'print'
    desc: 'Conditionally print hello'
    if: 'bool(my_input)'
    params:
      message: 'Hello'
  - action: 'print'
    desc: 'Conditionally print goodbye'
    if: '!bool(my_input)'
    params:
      message: 'Goodbye'`,
			},
			wantStdout:       "Hello\n",
			wantDestContents: map[string]string{},
		},
		{
			name: "step_with_if_needs_v1beta1",
			templateContents: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'My template'

steps:
  - action: 'print'
    desc: 'print the input value'
    if: 'true'
    params:
      message: 'Hello'`,
			},
			wantErr: `unknown field name "if"`,
		},
		{
			name: "if_invalid",
			templateContents: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta1'
kind: 'Template'
desc: 'My template'

steps:
  - action: 'print'
    desc: 'print the input value'
    if: 'bad_expression'
    params:
      message: 'Hello'`,
			},
			wantErr: `"if" expression "bad_expression" failed at step index 0 action "print"`,
		},
		{
			name:           "unknown_input_file_flags should be ignored",
			flagInputs:     map[string]string{"name_to_greet": "Robert"},
			inputFileNames: []string{"inputs.yaml"},
			inputFileContents: map[string]string{
				"inputs.yaml": `
unknown_key: 'unknown value'
emoji_suffix: '🐈'`,
			},
			templateContents: map[string]string{
				"myfile.txt":           "Some random stuff",
				"spec.yaml":            specContents,
				"file1.txt":            "my favorite color is blue",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantStdout: "Hello, Robert🐈.\n",
			wantDestContents: map[string]string{
				"file1.txt":            "my favorite color is red",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
		},
		{
			name:           "fail_if_input_missing_in_spec_file_but_in_inputs_file",
			inputFileNames: []string{"inputs.yaml"},
			inputFileContents: map[string]string{
				"inputs.yaml": `
name_to_greet: 'Robert'
emoji_suffix: '🐈'`, // missing in spec.yaml inputs
			},
			templateContents: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta1'
kind: 'Template'
desc: 'My template'
inputs:
  - name: 'name_to_greet'
steps:
  - action: 'print'
    desc: 'print greeting'
    params:
      message: 'Hello, {{.name_to_greet}}{{.emoji_suffix}}'`,
			},
			wantErr: "error reading template spec file",
		},
		{
			name: "git_metadata_variables_are_in_scope",
			templateContents: common.WithGitRepoAt("", map[string]string{
				".git/refs/tags/v1.2.3": common.MinimalGitHeadSHA,
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta3'
kind: 'Template'
desc: 'My template'
steps:
  - action: 'include'
    desc: 'include TF file'
    params:
      paths: ['example.tf']
  - action: 'go_template'
    desc: 'expand _git_sha reference'
    params:
      paths: ['example.tf']`,

				"example.tf": `
module "cloud_run" {
	source = "git::https://github.com/abcxyz/terraform-modules.git//modules/cloud_run?ref={{._git_sha}}"
}
module "cloud_run" {
	source = "git::https://github.com/abcxyz/terraform-modules.git//modules/cloud_run?ref={{._git_short_sha}}"
}
module "cloud_run" {
	source = "git::https://github.com/abcxyz/terraform-modules.git//modules/cloud_run?ref={{._git_tag}}"
}
`,
			}),
			wantDestContents: map[string]string{
				"example.tf": fmt.Sprintf(`
module "cloud_run" {
	source = "git::https://github.com/abcxyz/terraform-modules.git//modules/cloud_run?ref=%s"
}
module "cloud_run" {
	source = "git::https://github.com/abcxyz/terraform-modules.git//modules/cloud_run?ref=%s"
}
module "cloud_run" {
	source = "git::https://github.com/abcxyz/terraform-modules.git//modules/cloud_run?ref=%s"
}
`, common.MinimalGitHeadSHA, common.MinimalGitHeadShortSHA, "v1.2.3"),
			},
		},
		{
			name: "git_metadata_variables_are_empty_string_when_unavailable",
			templateContents: map[string]string{
				"example.txt": `"{{._git_tag}}" "{{._git_sha}}" "{{._git_short_sha}}"`,
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta3'
kind: 'Template'
desc: 'My template'
steps:
  - action: 'include'
    desc: 'include TF file'
    params:
      paths: ['example.txt']
  - action: 'go_template'
    desc: 'expand _git_sha reference'
    params:
      paths: ['example.txt']`,
			},
			wantDestContents: map[string]string{
				"example.txt": `"" "" ""`,
			},
		},
		{
			name: "git_metadata_variables_not_in_scope_on_old_api_version",
			templateContents: map[string]string{
				"example.txt": `"{{._git_tag}}" "{{._git_sha}}" "{{._git_short_sha}}"`,
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta2'
kind: 'Template'
desc: 'My template'
steps:
  - action: 'print'
    desc: 'should fail'
    params:
      message: '{{._git_tag}}'`,
			},
			wantErr: `nonexistent variable name "_git_tag"`,
		},
		{
			name: "git_metadata_variables_not_in_scope_on_old_api_version_cel",
			templateContents: map[string]string{
				"example.txt": `"{{._git_tag}}" "{{._git_sha}}" "{{._git_short_sha}}"`,
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta2'
kind: 'Template'
desc: 'My template'
steps:
  - action: 'print'
    desc: 'should fail'
    if: '_git_tag == ""' # _git_tag shouldn't be in scope. Should error out.
    params:
      message: 'Some message'`,
			},
			wantErr: `at line 7 column 9: the template referenced a nonexistent variable name "_git_tag"`,
		},
		{
			name:       "print_only_flags_are_in_scope_for_print_actions",
			flagInputs: map[string]string{},
			templateContents: map[string]string{
				"spec.yaml": `
api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'A template for the ages'
steps:
- desc: 'Print a message'
  action: 'print'
  params:
    message: '{{._flag_dest}} {{._flag_source}}'`,
			},
			overrideBuiltinVars: map[string]string{
				builtinvar.FlagDest:   "/my/dest",
				builtinvar.FlagSource: "/my/source",
			},
			wantStdout:       "/my/dest /my/source\n",
			wantDestContents: map[string]string{},
		},
		{
			name:       "print_only_flags_are_not_in_scope_outside_of_print_actions",
			flagInputs: map[string]string{},
			templateContents: map[string]string{
				"spec.yaml": `
api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'A template for the ages'
steps:
- desc: 'Include action that should fail because it uses a disallowed builtin'
  action: 'include'
  params:
    paths: ['{{._flag_dest}}']`,
			},
			overrideBuiltinVars: map[string]string{
				builtinvar.FlagDest:   "/my/dest",
				builtinvar.FlagSource: "/my/source",
			},
			wantErr: `nonexistent variable name "_flag_dest"`,
		},
		{
			name: "builtins_cant_be_set_by_regular_input",
			flagInputs: map[string]string{
				"_git_tag": "my-tag",
			},
			templateContents: map[string]string{
				"spec.yaml": `
api_version: 'cli.abcxyz.dev/v1beta3'
kind: 'Template'
desc: 'A template for the ages'
steps:
- desc: 'Print a message'
  action: 'print'
  params:
    message: '{{._git_tag}}'`,
			},
			wantErr: `input names beginning with underscore cannot be overridden by a normal user input; the bad input names were: [_git_tag]`,
		},
		{
			name: "inputs_cant_be_declared_with_leading_underscore",
			flagInputs: map[string]string{
				"_my_misnamed_input": "foo",
			},
			templateContents: map[string]string{
				"spec.yaml": `
api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'A template for the ages'
inputs:
- desc: 'This input should be rejected'
  name: '_my_misnamed_input'
steps:
- desc: 'Print a message'
  action: 'print'
  params:
    message: '{{._git_tag}}'`,
			},
			wantErr: `input names beginning with _ are reserved`,
		},
		{
			name: "overrides_cant_set_regular_inputs",
			flagInputs: map[string]string{
				"git_tag": "foo",
			},
			templateContents: map[string]string{
				"spec.yaml": `
api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'A template for the ages'
inputs:
- desc: 'My custom git tag input'
  name: 'git_tag'
steps:
- desc: 'Print a message'
  action: 'print'
  params:
    message: '{{.git_tag}}'`,
			},
			overrideBuiltinVars: map[string]string{
				"git_tag": "bar",
			},
			wantErr: "these builtin override var names are unknown and therefore invalid: [git_tag]",
		},
		{
			name: "abc_internal_is_reserved",
			templateContents: map[string]string{
				"spec.yaml": `
api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'A template for the ages'
steps:
- desc: 'Include some files and directories'
  action: 'include'
  params:
    paths:
      - paths: ['file1.txt']
        as:    ['.abc_internal']`,
				"file1.txt": "",
			},
			wantErr: `uses the reserved name ".abc_internal"`,
		},
		{
			name: "abc_internal_is_reserved_in_subdir",
			templateContents: map[string]string{
				"spec.yaml": `
api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'A template for the ages'
steps:
- desc: 'Include some files and directories'
  action: 'include'
  params:
    paths:
      - paths: ['file1.txt']
        as:    ['foo/.abc_internal']`,
				"file1.txt": "",
			},
			wantErr: `uses the reserved name ".abc_internal"`,
		},
		{
			name: "abc_internal_is_reserved_in_internal_subdir",
			templateContents: map[string]string{
				"spec.yaml": `
api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'A template for the ages'
steps:
- desc: 'Include some files and directories'
  action: 'include'
  params:
    paths:
      - paths: ['file1.txt']
        as:    ['foo/.abc_internal/bar.txt']`,
				"file1.txt": "",
			},
			wantErr: `uses the reserved name ".abc_internal"`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			dest := filepath.Join(tempDir, "dest")
			common.WriteAllDefaultMode(t, dest, tc.existingDestContents)

			inputFilePaths := make([]string, 0, len(tc.inputFileNames))
			for _, f := range tc.inputFileNames {
				inputFileDir := filepath.Join(tempDir, "inputs")
				common.WriteAllDefaultMode(t, inputFileDir, map[string]string{f: tc.inputFileContents[f]})
				inputFilePaths = append(inputFilePaths, filepath.Join(inputFileDir, f))
			}

			backupDir := filepath.Join(tempDir, "backups")
			sourceDir := filepath.Join(tempDir, "source")
			common.WriteAllDefaultMode(t, sourceDir, tc.templateContents)
			rfs := &common.RealFS{}
			stdoutBuf := &strings.Builder{}
			p := &Params{
				Backups:             true,
				BackupDir:           backupDir,
				Clock:               clk,
				DestDir:             dest,
				ForceOverwrite:      tc.flagForceOverwrite,
				Inputs:              tc.flagInputs,
				InputFiles:          inputFilePaths,
				KeepTempDirs:        tc.flagKeepTempDirs,
				Manifest:            tc.flagManifest,
				OverrideBuiltinVars: tc.overrideBuiltinVars,
				SkipInputValidation: tc.flagSkipInputValidation,
				DebugStepDiffs:      tc.flagDebugStepDiffs,
				Source:              sourceDir,
				FS: &common.ErrorFS{
					FS:           rfs,
					RemoveAllErr: tc.removeAllErr,
				},
				Stdout:      stdoutBuf,
				TempDirBase: tempDir,
			}

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))
			err := Render(ctx, p)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}
			if err != nil {
				errStr := err.Error()
				if strings.Count(errStr, " at line ") > 1 {
					t.Errorf(`this error message reported the "at line" location more than once: %q`, errStr)
				}
			}

			if diff := cmp.Diff(stdoutBuf.String(), tc.wantStdout); diff != "" {
				t.Errorf("template output was not as expected; (-got,+want): %s", diff)
			}

			var gotTemplateContents map[string]string
			templateDir, ok := testMustGlob(t, filepath.Join(tempDir, paths.TemplateDirNamePart+"*")) // the * accounts for the random cookie added by mkdirtemp
			if ok {
				gotTemplateContents = common.LoadDirWithoutMode(t, templateDir)
			}
			if diff := cmp.Diff(gotTemplateContents, tc.wantTemplateContents, common.CmpFileMode); diff != "" {
				t.Errorf("template directory contents were not as expected (-got,+want): %s", diff)
			}

			var gotScratchContents map[string]string
			scratchDir, ok := testMustGlob(t, filepath.Join(tempDir, paths.ScratchDirNamePart+"*"))
			if ok {
				gotScratchContents = common.LoadDirWithoutMode(t, scratchDir)
			}
			if diff := cmp.Diff(gotScratchContents, tc.wantScratchContents, common.CmpFileMode, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("scratch directory contents were not as expected (-got,+want): %s", diff)
			}

			gotDestContents := common.LoadDirWithoutMode(t, dest)
			if diff := cmp.Diff(gotDestContents, tc.wantDestContents, common.CmpFileMode); diff != "" {
				t.Errorf("dest directory contents were not as expected (-got,+want): %s", diff)
			}

			var gotBackupContents map[string]string
			backupSubdir, ok := testMustGlob(t, filepath.Join(backupDir, "*")) // When a backup directory is created, an unpredictable timestamp is added, hence the "*"
			if ok {
				gotBackupContents = common.LoadDirWithoutMode(t, backupSubdir)
			}
			if diff := cmp.Diff(gotBackupContents, tc.wantBackupContents, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("backups directory contents were not as expected (-got,+want): %s", diff)
			}

			var gotDebugContents map[string]string
			debugDir, ok := testMustGlob(t, filepath.Join(tempDir, paths.DebugStepDiffsDirNamePart+"*"))
			if ok {
				gotDebugContents = common.LoadDirWithoutMode(t, debugDir)
			}
			gotDebugDirExists := len(gotDebugContents) > 0
			if tc.flagDebugStepDiffs != gotDebugDirExists {
				t.Errorf("debug directory existence is %t but should be %t", gotDebugDirExists, tc.flagDebugStepDiffs)
			}
		})
	}
}

func testMustGlob(t *testing.T, glob string) (string, bool) {
	t.Helper()

	matches, err := filepath.Glob(glob)
	if err != nil {
		t.Fatalf("couldn't find template directory: %v", err)
	}
	switch len(matches) {
	case 0:
		return "", false
	case 1:
		return matches[0], true
	}
	t.Fatalf("got %d matches for glob %q, wanted 1: %s", len(matches), glob, matches)
	panic("unreachable") // silence compiler warning for "missing return"
}

func TestPromptDialog(t *testing.T) {
	t.Parallel()

	type dialogStep struct {
		waitForPrompt string
		thenRespond   string // should end with newline
	}
	cases := []struct {
		name          string
		inputs        []*spec.Input
		flagInputVals map[string]string // Simulates some inputs having already been provided by flags, like --input=foo=bar means we shouldn't prompt for "foo"
		dialog        []dialogStep
		want          map[string]string
		wantErr       string
	}{
		{
			name: "single_input_prompt",
			inputs: []*spec.Input{
				{
					Name: model.String{Val: "animal"},
					Desc: model.String{Val: "your favorite animal"},
				},
			},
			dialog: []dialogStep{
				{
					waitForPrompt: `
Input name:   animal
Description:  your favorite animal

Enter value: `,
					thenRespond: "alligator\n",
				},
			},
			want: map[string]string{
				"animal": "alligator",
			},
		},
		{
			name: "single_input_prompt_with_single_validation_rule",
			inputs: []*spec.Input{
				{
					Name: model.String{Val: "animal"},
					Desc: model.String{Val: "your favorite animal"},
					Rules: []*spec.InputRule{
						{
							Rule:    model.String{Val: "size(animal) > 1"},
							Message: model.String{Val: "length must be greater than 1"},
						},
					},
				},
			},
			dialog: []dialogStep{
				{
					waitForPrompt: `
Input name:   animal
Description:  your favorite animal
Rule:         size(animal) > 1
Rule msg:     length must be greater than 1

Enter value: `,
					thenRespond: "alligator\n",
				},
			},
			want: map[string]string{
				"animal": "alligator",
			},
		},
		{
			name: "single_input_prompt_with_multiple_validation_rules",
			inputs: []*spec.Input{
				{
					Name: model.String{Val: "animal"},
					Desc: model.String{Val: "your favorite animal"},
					Rules: []*spec.InputRule{
						{
							Rule:    model.String{Val: "size(animal) > 1"},
							Message: model.String{Val: "length must be greater than 1"},
						},
						{
							Rule:    model.String{Val: "size(animal) < 100"},
							Message: model.String{Val: "length must be less than 100"},
						},
					},
				},
			},
			dialog: []dialogStep{
				{
					waitForPrompt: `
Input name:   animal
Description:  your favorite animal
Rule 0:       size(animal) > 1
Rule 0 msg:   length must be greater than 1
Rule 1:       size(animal) < 100
Rule 1 msg:   length must be less than 100

Enter value: `,
					thenRespond: "alligator\n",
				},
			},
			want: map[string]string{
				"animal": "alligator",
			},
		},
		{
			name: "multiple_input_prompts",
			inputs: []*spec.Input{
				{
					Name: model.String{Val: "animal"},
					Desc: model.String{Val: "your favorite animal"},
				},
				{
					Name: model.String{Val: "car"},
					Desc: model.String{Val: "your favorite car"},
				},
			},
			dialog: []dialogStep{
				{
					waitForPrompt: `
Input name:   animal
Description:  your favorite animal

Enter value: `,
					thenRespond: "alligator\n",
				},
				{
					waitForPrompt: `
Input name:   car
Description:  your favorite car

Enter value: `,
					thenRespond: "Ford Bronco 🐎\n",
				},
			},
			want: map[string]string{
				"animal": "alligator",
				"car":    "Ford Bronco 🐎",
			},
		},
		{
			name: "single_input_should_not_be_prompted_if_provided_by_command_line_flags",
			inputs: []*spec.Input{
				{
					Name: model.String{Val: "animal"},
					Desc: model.String{Val: "your favorite animal"},
				},
			},
			flagInputVals: map[string]string{
				"animal": "alligator",
			},
			dialog: nil,
			want: map[string]string{
				"animal": "alligator",
			},
		},
		{
			name: "two_inputs_of_which_one_is_provided_and_one_prompted",
			inputs: []*spec.Input{
				{
					Name: model.String{Val: "animal"},
					Desc: model.String{Val: "your favorite animal"},
				},
				{
					Name: model.String{Val: "car"},
					Desc: model.String{Val: "your favorite car"},
				},
			},
			flagInputVals: map[string]string{
				"animal": "duck",
			},
			dialog: []dialogStep{
				{
					waitForPrompt: `
Input name:   car
Description:  your favorite car

Enter value: `,
					thenRespond: "Peugeot\n",
				},
			},
			want: map[string]string{
				"animal": "duck",
				"car":    "Peugeot",
			},
		},
		{
			name:   "template_has_no_inputs",
			inputs: []*spec.Input{},
		},
		{
			name: "single_input_with_default_accepted",
			inputs: []*spec.Input{
				{
					Name:    model.String{Val: "animal"},
					Desc:    model.String{Val: "your favorite animal"},
					Default: &model.String{Val: "shark"},
				},
			},
			dialog: []dialogStep{
				{
					waitForPrompt: `
Input name:   animal
Description:  your favorite animal
Default:      shark

Enter value, or leave empty to accept default: `,
					thenRespond: "\n",
				},
			},
			want: map[string]string{
				"animal": "shark",
			},
		},
		{
			name: "single_input_with_default_not_accepted",
			inputs: []*spec.Input{
				{
					Name:    model.String{Val: "animal"},
					Desc:    model.String{Val: "your favorite animal"},
					Default: &model.String{Val: "shark"},
				},
			},
			dialog: []dialogStep{
				{
					waitForPrompt: `
Input name:   animal
Description:  your favorite animal
Default:      shark

Enter value, or leave empty to accept default: `,
					thenRespond: "alligator\n",
				},
			},
			want: map[string]string{
				"animal": "alligator",
			},
		},
		{
			name: "default_empty_string_should_be_printed_quoted",
			inputs: []*spec.Input{
				{
					Name:    model.String{Val: "animal"},
					Desc:    model.String{Val: "your favorite animal"},
					Default: &model.String{Val: ""},
				},
			},
			dialog: []dialogStep{
				{
					waitForPrompt: `
Input name:   animal
Description:  your favorite animal
Default:      ""

Enter value, or leave empty to accept default: `,
					thenRespond: "\n",
				},
			},
			want: map[string]string{
				"animal": "",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cmd := &cli.BaseCommand{}

			stdinReader, stdinWriter := io.Pipe()
			stdoutReader, stdoutWriter := io.Pipe()
			_, stderrWriter := io.Pipe()

			cmd.SetStdin(stdinReader)
			cmd.SetStdout(stdoutWriter)
			cmd.SetStderr(stderrWriter)

			ctx := context.Background()
			errCh := make(chan error)
			var got map[string]string
			go func() {
				defer close(errCh)
				params := &input.ResolveParams{
					Inputs:             tc.flagInputVals,
					Prompt:             true,
					Prompter:           cmd,
					SkipPromptTTYCheck: true,
					Spec: &spec.Spec{
						Inputs: tc.inputs,
					},
				}
				var err error
				got, err = input.Resolve(ctx, params)
				errCh <- err
			}()

			for _, ds := range tc.dialog {
				readWithTimeout(t, stdoutReader, ds.waitForPrompt)
				writeWithTimeout(t, stdinWriter, ds.thenRespond)
			}

			select {
			case err := <-errCh:
				if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
					t.Fatal(diff)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for background goroutine to finish")
			}
			if diff := cmp.Diff(got, tc.want, cmpopts.EquateEmpty()); diff != "" {
				t.Fatalf("input values were different than expected (-got,+want): %s", diff)
			}
		})
	}
}

// readWithTimeout does a single read from the given reader. It calls Fatal if
// that read fails or the returned string doesn't contain wantSubStr. May leak a
// goroutine on timeout.
func readWithTimeout(tb testing.TB, r io.Reader, wantSubstr string) {
	tb.Helper()

	tb.Logf("readWith starting with %q", wantSubstr)

	var got string
	errCh := make(chan error)
	go func() {
		defer close(errCh)
		buf := make([]byte, 64*1_000)
		tb.Log("to Read")
		n, err := r.Read(buf)
		tb.Log("from Read")
		if err != nil {
			errCh <- err
			return
		}
		got = string(buf[:n])
	}()

	select {
	case err := <-errCh:
		if err != nil {
			tb.Fatal(err)
		}
	case <-time.After(time.Second):
		tb.Fatalf("timed out waiting to read %q", wantSubstr)
	}

	if !strings.Contains(got, wantSubstr) {
		tb.Fatalf("got a prompt %q, but wanted a prompt containing %q", got, wantSubstr)
	}
}

// writeWithTimeout does a single write to the given writer. It calls Fatal
// if that read doesn't contain wantSubStr. May leak a goroutine on timeout.
func writeWithTimeout(tb testing.TB, w io.Writer, msg string) {
	tb.Helper()

	tb.Logf("writeWithTimeout starting with %q", msg)

	errCh := make(chan error)
	go func() {
		defer close(errCh)
		tb.Log("to Write")
		_, err := w.Write([]byte(msg))
		tb.Log("from Write")
		errCh <- err
	}()

	select {
	case err := <-errCh:
		if err != nil {
			tb.Fatal(err)
		}
	case <-time.After(time.Second):
		tb.Fatalf("timed out waiting to write %q", msg)
	}
}
