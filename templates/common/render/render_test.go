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
	"os"
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
	"github.com/abcxyz/abc/templates/common/tempdir"
	"github.com/abcxyz/abc/templates/common/templatesource"
	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/abc/templates/model/decode"
	manifest "github.com/abcxyz/abc/templates/model/manifest/v1alpha1"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta6"
	abctestutil "github.com/abcxyz/abc/templates/testutil"
	mdl "github.com/abcxyz/abc/templates/testutil/model"
	"github.com/abcxyz/abc/templates/testutil/prompt"
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
		name                       string
		templateContents           map[string]string
		existingDestContents       map[string]string
		flagInputs                 map[string]string
		inputFileNames             []string
		inputFileContents          map[string]string
		flagAcceptDefaults         bool
		flagContinueWithoutPatches bool
		flagKeepTempDirs           bool
		flagForceOverwrite         bool
		flagIgnoreUnknownInputs    bool
		flagSkipInputValidation    bool
		flagBackfillManifestOnly   bool
		flagUpgradeChannel         string
		flagDebugStepDiffs         bool
		flagNoopIfInputsMatch      map[string]string
		overrideBuiltinVars        map[string]string
		removeAllErr               error
		wantScratchContents        map[string]string
		wantTemplateContents       map[string]string
		wantDestContents           map[string]string
		wantBackupContents         map[string]string
		wantStdout                 string
		wantNoopInputsMatched      bool
		wantErr                    string

		// manifests are part of the destination directory, but are compared
		// separately because they change every time we add a new api_version
		// and we don't want to change a bunch of "wanted" strings every time.
		wantManifest *manifest.Manifest
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
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				Inputs: []*manifest.Input{
					{Name: mdl.S("emoji_suffix"), Value: mdl.S("🐈")},
					{Name: mdl.S("ending_punctuation"), Value: mdl.S("!")},
					{Name: mdl.S("name_to_greet"), Value: mdl.S("Bob")},
				},
				OutputFiles: []*manifest.OutputFile{
					{File: mdl.S("dir1/file_in_dir.txt")},
					{File: mdl.S("dir2/file2.txt")},
					{File: mdl.S("file1.txt")},
				},
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
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				Inputs: []*manifest.Input{
					{
						Name:  mdl.S("emoji_suffix"),
						Value: mdl.S("\U0001F408"),
					},
					{
						Name:  mdl.S("ending_punctuation"),
						Value: mdl.S("!"),
					},
					{
						Name:  mdl.S("name_to_greet"),
						Value: mdl.S("Bob"),
					},
				},
				OutputFiles: []*manifest.OutputFile{
					{
						File: mdl.S("dir1/file_in_dir.txt"),
					},
					{
						File: mdl.S("dir2/file2.txt"),
					},
					{
						File: mdl.S("file1.txt"),
					},
				},
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
			wantStdout: "Hello, Bob🐈!\n",
			wantDestContents: map[string]string{
				"file1.txt":            "my favorite color is red",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				Inputs: []*manifest.Input{
					{
						Name:  mdl.S("emoji_suffix"),
						Value: mdl.S("\U0001F408"),
					},
					{
						Name:  mdl.S("ending_punctuation"),
						Value: mdl.S("!"),
					},
					{
						Name:  mdl.S("name_to_greet"),
						Value: mdl.S("Bob"),
					},
				},
				OutputFiles: []*manifest.OutputFile{
					{
						File: mdl.S("dir1/file_in_dir.txt"),
					},
					{
						File: mdl.S("dir2/file2.txt"),
					},
					{
						File: mdl.S("file1.txt"),
					},
				},
			},
		},
		{
			name: "simple_success_with_manifest_only_flag",
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
			flagBackfillManifestOnly: true,
			wantDestContents:         map[string]string{},
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				Inputs: []*manifest.Input{
					{
						Name:  mdl.S("emoji_suffix"),
						Value: mdl.S("\U0001F408"),
					},
					{
						Name:  mdl.S("ending_punctuation"),
						Value: mdl.S("!"),
					},
					{
						Name:  mdl.S("name_to_greet"),
						Value: mdl.S("Bob"),
					},
				},
				OutputFiles: []*manifest.OutputFile{
					{
						File: mdl.S("dir1/file_in_dir.txt"),
					},
					{
						File: mdl.S("dir2/file2.txt"),
					},
					{
						File: mdl.S("file1.txt"),
					},
				},
			},
		},
		{
			name: "preexisting_dest_files_with_manifest_only_flag",
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
			flagBackfillManifestOnly: true,
			existingDestContents: map[string]string{
				"file1.txt": "existing contents",
			},
			wantDestContents: map[string]string{
				"file1.txt": "existing contents",
			},
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				Inputs: []*manifest.Input{
					{
						Name:  mdl.S("emoji_suffix"),
						Value: mdl.S("\U0001F408"),
					},
					{
						Name:  mdl.S("ending_punctuation"),
						Value: mdl.S("!"),
					},
					{
						Name:  mdl.S("name_to_greet"),
						Value: mdl.S("Bob"),
					},
				},
				OutputFiles: []*manifest.OutputFile{
					{
						File: mdl.S("dir1/file_in_dir.txt"),
					},
					{
						File: mdl.S("dir2/file2.txt"),
					},
					{
						File: mdl.S("file1.txt"),
					},
				},
			},
		},
		{
			name: "manifest_only_flag_with_include_from_destination",
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
            - paths: ['myfile.txt']
              from: 'destination'
  - desc: 'Replace "purple" with "red"'
    action: 'string_replace'
    params:
        paths: ['.']
        replacements:
          - to_replace: 'purple'
            with: 'red'`,
			},
			flagBackfillManifestOnly:   true,
			flagContinueWithoutPatches: true,
			existingDestContents: map[string]string{
				"myfile.txt": "red",
			},
			wantDestContents: map[string]string{
				"myfile.txt": "red",
			},
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				Inputs:           []*manifest.Input{},
				OutputFiles: []*manifest.OutputFile{
					{
						File: mdl.S("myfile.txt"),
					},
				},
			},
		},
		{
			name:           "simple_success_with_input_file_flag",
			inputFileNames: []string{"inputs.yaml"},
			inputFileContents: map[string]string{
				"inputs.yaml": `
name_to_greet: 'Bob'
emoji_suffix: '🐈'
ending_punctuation: '.'`,
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
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				Inputs: []*manifest.Input{
					{Name: mdl.S("emoji_suffix"), Value: mdl.S("🐈")},
					{Name: mdl.S("ending_punctuation"), Value: mdl.S(".")},
					{Name: mdl.S("name_to_greet"), Value: mdl.S("Bob")},
				},
				OutputFiles: []*manifest.OutputFile{
					{File: mdl.S("dir1/file_in_dir.txt")},
					{File: mdl.S("dir2/file2.txt")},
					{File: mdl.S("file1.txt")},
				},
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
			flagAcceptDefaults: true,
			wantStdout:         "Hello, Robert🐈.\n",
			wantDestContents: map[string]string{
				"file1.txt":            "my favorite color is red",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				Inputs: []*manifest.Input{
					{
						Name:  mdl.S("emoji_suffix"),
						Value: mdl.S("\U0001F408"),
					},
					{
						Name:  mdl.S("ending_punctuation"),
						Value: mdl.S("."),
					},
					{
						Name:  mdl.S("name_to_greet"),
						Value: mdl.S("Robert"),
					},
				},
				OutputFiles: []*manifest.OutputFile{
					{
						File: mdl.S("dir1/file_in_dir.txt"),
					},
					{
						File: mdl.S("dir2/file2.txt"),
					},
					{
						File: mdl.S("file1.txt"),
					},
				},
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
			flagAcceptDefaults: true,
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
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				Inputs: []*manifest.Input{
					{Name: mdl.S("emoji_suffix"), Value: mdl.S("🐈")},
					{Name: mdl.S("ending_punctuation"), Value: mdl.S(".")},
					{Name: mdl.S("name_to_greet"), Value: mdl.S("Bob")},
				},
				OutputFiles: []*manifest.OutputFile{
					{File: mdl.S("dir1/file_in_dir.txt")},
					{File: mdl.S("dir2/file2.txt")},
					{File: mdl.S("file1.txt")},
				},
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
			flagKeepTempDirs:   true,
			flagAcceptDefaults: true,
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
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				Inputs: []*manifest.Input{
					{Name: mdl.S("emoji_suffix"), Value: mdl.S("🐈")},
					{Name: mdl.S("ending_punctuation"), Value: mdl.S(".")},
					{Name: mdl.S("name_to_greet"), Value: mdl.S("Bob")},
				},
				OutputFiles: []*manifest.OutputFile{
					{File: mdl.S("dir1/file_in_dir.txt")},
					{File: mdl.S("dir2/file2.txt")},
					{File: mdl.S("file1.txt")},
				},
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
				"name_to_greet":      "Bob",
				"emoji_suffix":       "🐈",
				"ending_punctuation": ".",
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
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				Inputs: []*manifest.Input{
					{Name: mdl.S("emoji_suffix"), Value: mdl.S("🐈")},
					{Name: mdl.S("ending_punctuation"), Value: mdl.S(".")},
					{Name: mdl.S("name_to_greet"), Value: mdl.S("Bob")},
				},
				OutputFiles: []*manifest.OutputFile{
					{File: mdl.S("dir1/file_in_dir.txt")},
					{File: mdl.S("dir2/file2.txt")},
					{File: mdl.S("file1.txt")},
				},
			},
		},
		{
			name: "existing_dest_file_without_overwrite_flag_should_fail",
			flagInputs: map[string]string{
				"name_to_greet":      "Bob",
				"emoji_suffix":       "🐈",
				"ending_punctuation": ".",
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
			flagAcceptDefaults: true,
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
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				Inputs: []*manifest.Input{
					{Name: mdl.S("emoji_suffix"), Value: mdl.S("🐈")},
					{Name: mdl.S("ending_punctuation"), Value: mdl.S(".")},
					{Name: mdl.S("name_to_greet"), Value: mdl.S("Bob")},
				},
				OutputFiles: []*manifest.OutputFile{
					{File: mdl.S("dir1/file_in_dir.txt")},
					{File: mdl.S("dir2/file2.txt")},
					{File: mdl.S("file1.txt")},
				},
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
			// flagAcceptDefaults: ,
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
			name: "destination_include_with_manifest",
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
			wantManifest: &manifest.Manifest{
				// api_version intentionally omitted because it changes every
				// time we add a new api version
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				OutputFiles: []*manifest.OutputFile{
					{
						File: mdl.S("myfile.txt"),
						Patch: mdl.SP(`--- a/myfile.txt
+++ b/myfile.txt
@@ -1 +1 @@
-red is my favorite color
\ No newline at end of file
+purple is my favorite color
\ No newline at end of file
`),
					},
					{
						File: mdl.S("subdir_a/file_a.txt"),
						Patch: mdl.SP(`--- a/subdir_a/file_a.txt
+++ b/subdir_a/file_a.txt
@@ -1 +1 @@
-red is my favorite color
\ No newline at end of file
+purple is my favorite color
\ No newline at end of file
`),
					},
					{
						File: mdl.S("subdir_b/file_b.txt"),
						Patch: mdl.SP(`--- a/subdir_b/file_b.txt
+++ b/subdir_b/file_b.txt
@@ -1 +1 @@
-red is my favorite color
\ No newline at end of file
+purple is my favorite color
\ No newline at end of file
`),
					},
				},
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
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				OutputFiles: []*manifest.OutputFile{
					{
						File: mdl.S("file_a.txt"),
						Patch: mdl.SP(`--- a/file_a.txt
+++ b/file_a.txt
@@ -1 +1 @@
-red is my favorite color
\ No newline at end of file
+purple is my favorite color
\ No newline at end of file
`),
					},
					{File: mdl.S("file_b.txt")},
				},
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
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),

				OutputFiles: []*manifest.OutputFile{
					{
						File: mdl.S("dir/file_b.txt"),
					},
					{
						File: mdl.S("file_a.txt"),
						Patch: mdl.SP(`--- a/file_a.txt
+++ b/file_a.txt
@@ -1 +1 @@
-red is my favorite color
\ No newline at end of file
+purple is my favorite color
\ No newline at end of file
`),
					},
				},
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
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				OutputFiles: []*manifest.OutputFile{
					{
						File: mdl.S("file_a.txt"),
						Patch: mdl.SP(`--- a/file_a.txt
+++ b/file_a.txt
@@ -1 +1 @@
-red is my favorite color
\ No newline at end of file
+purple is my favorite color
\ No newline at end of file
`),
					},
				},
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
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
			},
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
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				OutputFiles: []*manifest.OutputFile{
					{File: mdl.S("dir1/something.md")},
					{File: mdl.S("dir2/something.json")},
					{File: mdl.S("file1.txt")},
					{File: mdl.S("file2.txt")},
					{File: mdl.S("file3.txt")},
					{File: mdl.S("python_files/include_me.py")},
				},
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
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
			},
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
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				Inputs: []*manifest.Input{
					{Name: mdl.S("my_input"), Value: mdl.S("crocodile")},
				},
			},
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
			flagAcceptDefaults: true,
			wantStdout:         "Hello\n",
			wantDestContents:   map[string]string{},
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				Inputs: []*manifest.Input{
					{Name: mdl.S("my_input"), Value: mdl.S("true")},
				},
			},
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
			name:           "unknown_input_file_flags_should_be_ignored",
			flagInputs:     map[string]string{"name_to_greet": "Robert"},
			inputFileNames: []string{"inputs.yaml"},
			inputFileContents: map[string]string{
				"inputs.yaml": `
unknown_key: 'unknown value'
emoji_suffix: '🐈'`,
			},
			flagAcceptDefaults: true,
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
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				Inputs: []*manifest.Input{
					{Name: mdl.S("emoji_suffix"), Value: mdl.S("🐈")},
					{Name: mdl.S("ending_punctuation"), Value: mdl.S(".")},
					{Name: mdl.S("name_to_greet"), Value: mdl.S("Robert")},
				},
				OutputFiles: []*manifest.OutputFile{
					{File: mdl.S("dir1/file_in_dir.txt")},
					{File: mdl.S("dir2/file2.txt")},
					{File: mdl.S("file1.txt")},
				},
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
			templateContents: abctestutil.WithGitRepoAt("", map[string]string{
				".git/refs/tags/v1.2.3": abctestutil.MinimalGitHeadSHA,
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
`, abctestutil.MinimalGitHeadSHA, abctestutil.MinimalGitHeadShortSHA, "v1.2.3"),
			},
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				OutputFiles: []*manifest.OutputFile{
					{File: mdl.S("example.tf")},
				},
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
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				OutputFiles: []*manifest.OutputFile{
					{File: mdl.S("example.txt")},
				},
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
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
			},
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
			name: "abc_is_reserved_as_file_name_in_dest_dir",
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
        as:    ['.abc']`,
				"file1.txt": "",
			},
			wantErr: `uses the reserved name ".abc"`,
		},
		{
			name: "abc_is_reserved_as_dir_name_in_dest_dir",
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
        as:    ['.abc/file1.txt']`,
				"file1.txt": "",
			},
			wantErr: `uses the reserved name ".abc"`,
		},
		{
			name: "abc_is_not_reserved_for_file_in_subdir",
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
        as:    ['foo/.abc']`,
				"file1.txt": "",
			},
			wantDestContents: map[string]string{
				"foo/.abc": "",
			},
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				OutputFiles: []*manifest.OutputFile{
					{File: mdl.S("foo/.abc")},
				},
			},
		},
		{
			name: "abc_is_not_reserved_as_subdir_name",
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
        as:    ['foo/.abc/bar.txt']`,
				"file1.txt": "",
			},
			wantDestContents: map[string]string{
				"foo/.abc/bar.txt": "",
			},
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				OutputFiles: []*manifest.OutputFile{
					{File: mdl.S("foo/.abc/bar.txt")},
				},
			},
		},
		{
			name: "independent_rule_validation_valid_rules",
			templateContents: abctestutil.WithGitRepoAt("", map[string]string{
				".git/refs/tags/v1.2.3": abctestutil.MinimalGitHeadSHA,
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta4'
kind: 'Template'
desc: 'My template'

rules:
  - rule: 'int(2) < int(10)'
    message: 'rule validation'

steps:
  - desc: 'print a message'
    action: 'print'
    params:
      message: 'rule validation passed'
`,
			}),
			wantStdout:       "rule validation passed\n",
			wantDestContents: map[string]string{},
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
			},
		},
		{
			name: "independent_rule_validation_invalid_rules",
			templateContents: abctestutil.WithGitRepoAt("", map[string]string{
				".git/refs/tags/v1.2.3": abctestutil.MinimalGitHeadSHA,
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta4'
kind: 'Template'
desc: 'My template'

rules:
  - rule: 'int(2) > int(10)'
    message: 'invalid rule: 2 cannot be greater than 10'
  - rule: '"abc".startsWith("z")'
    message: 'invalid rule: abc does not start with z'

steps:
  - desc: 'print a message'
    action: 'print'
    params:
      message: 'rule validation passed'
`,
			}),
			wantErr: "rules validation failed:\n\nRule:      int(2) > int(10)\nRule msg:  invalid rule: 2 cannot be greater than 10\n\nRule:      \"abc\".startsWith(\"z\")\nRule msg:  invalid rule: abc does not start with z\n",
		},
		{
			name: "independent_rule_validation_built_in_vars_in_scope",
			templateContents: abctestutil.WithGitRepoAt("", map[string]string{
				".git/refs/tags/v1.2.3": abctestutil.MinimalGitHeadSHA,
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta4'
kind: 'Template'
desc: 'My template'

rules:
  - rule: '_git_sha == "ahl8foqboh8ktqzxnymuvdcg91hvim0cfszlcstl"'
    message: 'git sha must have specific value'
  - rule: '_git_short_sha == "ahl8foq"'
    message: 'git short sha must have specific value'
  - rule: '_git_tag == "v1.2.3"'
    message: 'git tag must have specific value'

steps:
  - desc: 'print a message'
    action: 'print'
    params:
      message: |-
        git sha: {{._git_sha}}
        git short sha: {{._git_short_sha}}
        git tag: {{._git_tag}}
`,
			}),
			overrideBuiltinVars: map[string]string{
				"_git_sha":       "ahl8foqboh8ktqzxnymuvdcg91hvim0cfszlcstl",
				"_git_short_sha": "ahl8foq",
				"_git_tag":       "v1.2.3",
			},
			wantStdout:       "git sha: ahl8foqboh8ktqzxnymuvdcg91hvim0cfszlcstl\ngit short sha: ahl8foq\ngit tag: v1.2.3\n",
			wantDestContents: map[string]string{},
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
			},
		},
		{
			name: "formatTime_not_in_scope_on_old_spec",
			templateContents: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta5'
kind: 'Template'
desc: 'A template for the ages'
steps:
- desc: 'Print a message'
  action: 'print'
  params:
    message: 'The timestamp is {{formatTime ._now_ms "2006-01-02T15:04:05"}}'`,
			},
			wantErr: `function "formatTime" not defined`,
		},
		{
			name: "formatTime_is_in_scope_on_new_spec",
			templateContents: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta6'
kind: 'Template'
desc: 'A template for the ages'
steps:
- desc: 'Print a message'
  action: 'print'
  params:
    message: 'The timestamp is {{formatTime ._now_ms "2006-01-02T15:04:05"}}'`,
			},
			wantStdout: "The timestamp is 2023-12-08T23:59:02\n",
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
			},
		},
		{
			name: "_now_ms_not_in_scope_on_old_spec",
			templateContents: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta5'
kind: 'Template'
desc: 'A template for the ages'
steps:
- desc: 'Print a message'
  action: 'print'
  params:
    message: 'The timestamp is {{ ._now_ms }}'`,
			},
			wantErr: `nonexistent variable name "_now_ms"`,
		},
		{
			name: "_now_ms_is_in_scope_on_new_spec",
			templateContents: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta6'
kind: 'Template'
desc: 'A template for the ages'
steps:
- desc: 'Print a message'
  action: 'print'
  params:
    message: 'The timestamp is {{ ._now_ms }}'`,
			},
			wantStdout: "The timestamp is 1702079942000\n",
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
			},
		},
		{
			name: "flag_ignore_unknown_inputs",
			flagInputs: map[string]string{
				// This input doesn't exist. Shouldn't cause a crash though.
				"nonexistent_input": "hrm",

				"name_to_greet":      "Bob",
				"emoji_suffix":       "🐈",
				"ending_punctuation": "!",
			},
			flagIgnoreUnknownInputs: true,
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
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				Inputs: []*manifest.Input{
					{
						Name:  mdl.S("emoji_suffix"),
						Value: mdl.S("\U0001F408"),
					},
					{
						Name:  mdl.S("ending_punctuation"),
						Value: mdl.S("!"),
					},
					{
						Name:  mdl.S("name_to_greet"),
						Value: mdl.S("Bob"),
					},
				},
				OutputFiles: []*manifest.OutputFile{
					{
						File: mdl.S("dir1/file_in_dir.txt"),
					},
					{
						File: mdl.S("dir2/file2.txt"),
					},
					{
						File: mdl.S("file1.txt"),
					},
				},
			},
		},
		{
			name: "noop_on_input_match",
			flagInputs: map[string]string{
				"name_to_greet":      "Alice",
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
			flagNoopIfInputsMatch: map[string]string{
				"name_to_greet":      "Alice",
				"emoji_suffix":       "🐈",
				"ending_punctuation": "!",
			},
			wantNoopInputsMatched: true,
		},
		{
			name: "not_noop_on_input_mismatch",
			flagInputs: map[string]string{
				"name_to_greet":      "Alice",
				"emoji_suffix":       "🐈",
				"ending_punctuation": "!",
			},
			flagNoopIfInputsMatch: map[string]string{
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
			wantStdout: "Hello, Alice🐈!\n",
			wantDestContents: map[string]string{
				"file1.txt":            "my favorite color is red",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantManifest: &manifest.Manifest{
				CreationTime:     clk.Now(),
				ModificationTime: clk.Now(),
				Inputs: []*manifest.Input{
					{Name: mdl.S("emoji_suffix"), Value: mdl.S("🐈")},
					{Name: mdl.S("ending_punctuation"), Value: mdl.S("!")},
					{Name: mdl.S("name_to_greet"), Value: mdl.S("Alice")},
				},
				OutputFiles: []*manifest.OutputFile{
					{File: mdl.S("dir1/file_in_dir.txt")},
					{File: mdl.S("dir2/file2.txt")},
					{File: mdl.S("file1.txt")},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			outDir := filepath.Join(tempDir, "out_dir")
			abctestutil.WriteAll(t, outDir, tc.existingDestContents)

			inputFilePaths := make([]string, 0, len(tc.inputFileNames))
			for _, f := range tc.inputFileNames {
				inputFileDir := filepath.Join(tempDir, "inputs")
				abctestutil.WriteAll(t, inputFileDir, map[string]string{f: tc.inputFileContents[f]})
				inputFilePaths = append(inputFilePaths, filepath.Join(inputFileDir, f))
			}

			backupDir := filepath.Join(tempDir, "backups")
			sourceDir := filepath.Join(tempDir, "source")
			abctestutil.WriteAll(t, sourceDir, tc.templateContents)
			rfs := &common.RealFS{}
			stdoutBuf := &strings.Builder{}
			p := &Params{
				AcceptDefaults:         tc.flagAcceptDefaults,
				BackfillManifestOnly:   tc.flagBackfillManifestOnly,
				Backups:                true,
				BackupDir:              backupDir,
				Clock:                  clk,
				ContinueWithoutPatches: tc.flagContinueWithoutPatches,
				DebugStepDiffs:         tc.flagDebugStepDiffs,
				Downloader:             &templatesource.LocalDownloader{SrcPath: sourceDir},
				ForceOverwrite:         tc.flagForceOverwrite,
				FS: &common.ErrorFS{
					FS:           rfs,
					RemoveAllErr: tc.removeAllErr,
				},
				IgnoreUnknownInputs: tc.flagIgnoreUnknownInputs,
				InputFiles:          inputFilePaths,
				InputsFromFlags:     tc.flagInputs,
				KeepTempDirs:        tc.flagKeepTempDirs,
				NoopIfInputsMatch:   tc.flagNoopIfInputsMatch,
				OutDir:              outDir,
				OverrideBuiltinVars: tc.overrideBuiltinVars,
				SkipInputValidation: tc.flagSkipInputValidation,
				SourceForMessages:   sourceDir,
				Stdout:              stdoutBuf,
				TempDirBase:         tempDir,
				UpgradeChannel:      tc.flagUpgradeChannel,
			}

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))
			result, err := Render(ctx, p)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}
			if err != nil {
				errStr := err.Error()
				if strings.Count(errStr, " at line ") > 1 {
					t.Errorf(`this error message reported the "at line" location more than once: %q`, errStr)
				}
			}
			if err == nil {
				verifyManifest(ctx, t, result.ManifestPath != "", filepath.Join(outDir, result.ManifestPath), tc.wantManifest)
				if result.NoopInputsMatched != tc.wantNoopInputsMatched {
					t.Errorf("noopInputsMatched was %t but should be %t", result.NoopInputsMatched, tc.wantNoopInputsMatched)
				}
			}

			if diff := cmp.Diff(stdoutBuf.String(), tc.wantStdout); diff != "" {
				t.Errorf("template output was not as expected; (-got,+want): %s", diff)
			}

			var gotTemplateContents map[string]string
			templateDir, ok := abctestutil.TestMustGlob(t, filepath.Join(tempDir, tempdir.TemplateDirNamePart+"*")) // the * accounts for the random cookie added by mkdirtemp
			if ok {
				gotTemplateContents = abctestutil.LoadDir(t, templateDir)
			}
			if diff := cmp.Diff(gotTemplateContents, tc.wantTemplateContents); diff != "" {
				t.Errorf("template directory contents were not as expected (-got,+want): %s", diff)
			}

			var gotScratchContents map[string]string
			scratchDir, ok := abctestutil.TestMustGlob(t, filepath.Join(tempDir, tempdir.ScratchDirNamePart+"*"))
			if ok {
				gotScratchContents = abctestutil.LoadDir(t, scratchDir)
			}
			if diff := cmp.Diff(gotScratchContents, tc.wantScratchContents, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("scratch directory contents were not as expected (-got,+want): %s", diff)
			}

			// The manifest is verified separately, hence the SkipGlob().
			gotDestContents := abctestutil.LoadDir(t, outDir, abctestutil.SkipGlob(".abc/manifest*"))
			if diff := cmp.Diff(gotDestContents, tc.wantDestContents, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("dest directory contents were not as expected (-got,+want): %s", diff)
			}

			var gotBackupContents map[string]string
			backupSubdir, ok := abctestutil.TestMustGlob(t, filepath.Join(backupDir, "*")) // When a backup directory is created, an unpredictable timestamp is added, hence the "*"
			if ok {
				gotBackupContents = abctestutil.LoadDir(t, backupSubdir)
			}
			if diff := cmp.Diff(gotBackupContents, tc.wantBackupContents, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("backups directory contents were not as expected (-got,+want): %s", diff)
			}

			var gotDebugContents map[string]string
			debugDir, ok := abctestutil.TestMustGlob(t, filepath.Join(tempDir, tempdir.DebugStepDiffsDirNamePart+"*"))
			if ok {
				gotDebugContents = abctestutil.LoadDir(t, debugDir)
			}
			gotDebugDirExists := len(gotDebugContents) > 0
			if tc.flagDebugStepDiffs != gotDebugDirExists {
				t.Errorf("debug directory existence is %t but should be %t", gotDebugDirExists, tc.flagDebugStepDiffs)
			}
		})
	}
}

func verifyManifest(ctx context.Context, tb testing.TB, gotManifest bool, manifestPath string, want *manifest.Manifest) {
	tb.Helper()

	if !gotManifest {
		if want == nil {
			// No manifest was outputted, and none was expected, so that's a success
			return
		}
		tb.Errorf("got no manifest, but wanted %v", want)
		return
	}

	got := mustLoadManifest(ctx, tb, manifestPath)

	opts := []cmp.Option{
		// Don't force test authors to assert the line and column numbers
		cmpopts.IgnoreTypes(&model.ConfigPos{}, model.ConfigPos{}),
		cmpopts.IgnoreFields(manifest.Manifest{}, "TemplateDirhash"),
		cmpopts.IgnoreFields(manifest.OutputFile{}, "Hash"),
		cmpopts.EquateEmpty(),
	}

	if diff := cmp.Diff(got, want, opts...); diff != "" {
		tb.Errorf("the outputted manifest was not as expected (-got,+want): %s", diff)
	}
}

func TestPromptDialog(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		inputs        []*spec.Input
		flagInputVals map[string]string // Simulates some inputs having already been provided by flags, like --input=foo=bar means we shouldn't prompt for "foo"
		dialog        []prompt.DialogStep
		want          map[string]string
		wantErr       string
	}{
		{
			name: "single_input_prompt",
			inputs: []*spec.Input{
				{
					Name: mdl.S("animal"),
					Desc: mdl.S("your favorite animal"),
				},
			},
			dialog: []prompt.DialogStep{
				{
					WaitForPrompt: `
Input name:   animal
Description:  your favorite animal

Enter value: `,
					ThenRespond: "alligator\n",
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
					Name: mdl.S("animal"),
					Desc: mdl.S("your favorite animal"),
					Rules: []*spec.Rule{
						{
							Rule:    mdl.S("size(animal) > 1"),
							Message: mdl.S("length must be greater than 1"),
						},
					},
				},
			},
			dialog: []prompt.DialogStep{
				{
					WaitForPrompt: `
Input name:   animal
Description:  your favorite animal
Rule:         size(animal) > 1
Rule msg:     length must be greater than 1

Enter value: `,
					ThenRespond: "alligator\n",
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
					Name: mdl.S("animal"),
					Desc: mdl.S("your favorite animal"),
					Rules: []*spec.Rule{
						{
							Rule:    mdl.S("size(animal) > 1"),
							Message: mdl.S("length must be greater than 1"),
						},
						{
							Rule:    mdl.S("size(animal) < 100"),
							Message: mdl.S("length must be less than 100"),
						},
					},
				},
			},
			dialog: []prompt.DialogStep{
				{
					WaitForPrompt: `
Input name:   animal
Description:  your favorite animal
Rule 0:       size(animal) > 1
Rule 0 msg:   length must be greater than 1
Rule 1:       size(animal) < 100
Rule 1 msg:   length must be less than 100

Enter value: `,
					ThenRespond: "alligator\n",
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
					Name: mdl.S("animal"),
					Desc: mdl.S("your favorite animal"),
				},
				{
					Name: mdl.S("car"),
					Desc: mdl.S("your favorite car"),
				},
			},
			dialog: []prompt.DialogStep{
				{
					WaitForPrompt: `
Input name:   animal
Description:  your favorite animal

Enter value: `,
					ThenRespond: "alligator\n",
				},
				{
					WaitForPrompt: `
Input name:   car
Description:  your favorite car

Enter value: `,
					ThenRespond: "Ford Bronco 🐎\n",
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
					Name: mdl.S("animal"),
					Desc: mdl.S("your favorite animal"),
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
					Name: mdl.S("animal"),
					Desc: mdl.S("your favorite animal"),
				},
				{
					Name: mdl.S("car"),
					Desc: mdl.S("your favorite car"),
				},
			},
			flagInputVals: map[string]string{
				"animal": "duck",
			},
			dialog: []prompt.DialogStep{
				{
					WaitForPrompt: `
Input name:   car
Description:  your favorite car

Enter value: `,
					ThenRespond: "Peugeot\n",
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
					Name:    mdl.S("animal"),
					Desc:    mdl.S("your favorite animal"),
					Default: mdl.SP("shark"),
				},
			},
			dialog: []prompt.DialogStep{
				{
					WaitForPrompt: `
Input name:   animal
Description:  your favorite animal
Default:      shark

Enter value, or leave empty to accept default: `,
					ThenRespond: "\n",
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
					Name:    mdl.S("animal"),
					Desc:    mdl.S("your favorite animal"),
					Default: mdl.SP("shark"),
				},
			},
			dialog: []prompt.DialogStep{
				{
					WaitForPrompt: `
Input name:   animal
Description:  your favorite animal
Default:      shark

Enter value, or leave empty to accept default: `,
					ThenRespond: "alligator\n",
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
					Name:    mdl.S("animal"),
					Desc:    mdl.S("your favorite animal"),
					Default: mdl.SP(""),
				},
			},
			dialog: []prompt.DialogStep{
				{
					WaitForPrompt: `
Input name:   animal
Description:  your favorite animal
Default:      ""

Enter value, or leave empty to accept default: `,
					ThenRespond: "\n",
				},
			},
			want: map[string]string{
				"animal": "",
			},
		},
	}

	for _, tc := range cases {
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
				prompt.ReadWithTimeout(t, stdoutReader, ds.WaitForPrompt)
				prompt.WriteWithTimeout(t, stdinWriter, ds.ThenRespond)
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

// mustLoadManifest parses the given manifest file.
func mustLoadManifest(ctx context.Context, tb testing.TB, path string) *manifest.Manifest {
	tb.Helper()

	f, err := os.Open(path)
	if err != nil {
		tb.Fatalf("failed to open manifest file at %q: %v", path, err)
	}
	defer f.Close()

	manifestI, _, err := decode.DecodeValidateUpgrade(ctx, f, path, decode.KindManifest)
	if err != nil {
		tb.Fatalf("error reading manifest file: %v", err)
	}

	out, ok := manifestI.(*manifest.Manifest)
	if !ok {
		tb.Fatalf("internal error: manifest file did not decode to *manifest.Manifest")
	}

	return out
}
