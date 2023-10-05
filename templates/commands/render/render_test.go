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

package render

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/abc/templates/model/spec"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/go-getter/v2"
	"golang.org/x/exp/maps"
)

func TestRenderFlags_Parse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    []string
		want    RenderFlags
		wantErr string
	}{
		{
			name: "all_flags_present",
			args: []string{
				"--dest", "my_dir",
				"--git-protocol", "https",
				"--input", "x=y",
				"--log-level", "info",
				"--force-overwrite",
				"--keep-temp-dirs",
				"--skip-input-validation",
				"--debug-scratch-contents",
				"helloworld@v1",
			},
			want: RenderFlags{
				Source:               "helloworld@v1",
				Dest:                 "my_dir",
				GitProtocol:          "https",
				Inputs:               map[string]string{"x": "y"},
				LogLevel:             "info",
				ForceOverwrite:       true,
				KeepTempDirs:         true,
				SkipInputValidation:  true,
				DebugScratchContents: true,
			},
		},
		{
			name: "minimal_flags_present",
			args: []string{
				"helloworld@v1",
			},
			want: RenderFlags{
				Source:         "helloworld@v1",
				Dest:           ".",
				GitProtocol:    "https",
				Inputs:         map[string]string{},
				LogLevel:       "warn",
				ForceOverwrite: false,
				KeepTempDirs:   false,
			},
		},
		{
			name:    "required_source_is_missing",
			args:    []string{},
			wantErr: "missing <source> file",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var cmd Command
			cmd.SetLookupEnv(cli.MapLookuper(nil))

			err := cmd.Flags().Parse(tc.args)
			if err != nil || tc.wantErr != "" {
				if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
					t.Fatal(diff)
				}
				return
			}
			if diff := cmp.Diff(cmd.flags, tc.want); diff != "" {
				t.Errorf("got %#v, want %#v, diff (-got, +want): %v", cmd.flags, tc.want, diff)
			}
		})
	}
}

func TestDestOK(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		dest    string
		fs      fs.StatFS
		wantErr string
	}{
		{
			name: "dest_exists_should_succeed",
			dest: "my/dir",
			fs: fstest.MapFS{
				"my/dir/foo.txt": {},
			},
		},
		{
			name: "dest_is_file_should_fail",
			dest: "my/file",
			fs: fstest.MapFS{
				"my/file": {},
			},
			wantErr: "exists but isn't a directory",
		},
		{
			name:    "stat_returns_error",
			dest:    "my/git/dir",
			fs:      &errorFS{statErr: fmt.Errorf("yikes")},
			wantErr: "yikes",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := destOK(tc.fs, tc.dest)
			if diff := testutil.DiffErrString(got, tc.wantErr); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestRealRun(t *testing.T) {
	t.Parallel()

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
- name: 'defaulted_input'
  desc: 'The defaulted input'
  default:  'default'
steps:
- desc: 'Print a message'
  action: 'print'
  params:
    message: 'Hello, {{.name_to_greet}}{{.emoji_suffix}}'
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
		flagKeepTempDirs        bool
		flagForceOverwrite      bool
		flagSkipInputValidation bool
		getterErr               error
		removeAllErr            error
		wantScratchContents     map[string]string
		wantTemplateContents    map[string]string
		wantDestContents        map[string]string
		wantBackupContents      map[string]string
		wantFlagInputs          map[string]string
		wantStdout              string
		wantErr                 string
	}{
		{
			name: "simple_success",

			flagInputs: map[string]string{
				"name_to_greet":   "Bob",
				"emoji_suffix":    "🐈",
				"defaulted_input": "default",
			},
			templateContents: map[string]string{
				"myfile.txt":           "Some random stuff",
				"spec.yaml":            specContents,
				"file1.txt":            "my favorite color is blue",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantStdout: "Hello, Bob🐈\n",
			wantDestContents: map[string]string{
				"file1.txt":            "my favorite color is red",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
		},
		{
			name: "keep_temp_dirs_on_success_if_flag",
			flagInputs: map[string]string{
				"name_to_greet":   "Bob",
				"emoji_suffix":    "🐈",
				"defaulted_input": "default",
			},
			flagKeepTempDirs: true,
			templateContents: map[string]string{
				"spec.yaml":            specContents,
				"file1.txt":            "my favorite color is blue",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantStdout: "Hello, Bob🐈\n",
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
				"name_to_greet":   "Bob",
				"emoji_suffix":    "🐈",
				"defaulted_input": "default",
			},
			flagKeepTempDirs: true,
			templateContents: map[string]string{
				"spec.yaml": "this is an unparseable YAML file *&^#%$",
			},
			wantTemplateContents: map[string]string{
				"spec.yaml": "this is an unparseable YAML file *&^#%$",
			},
			wantErr: "error parsing spec",
		},
		{
			name: "existing_dest_file_with_overwrite_flag_should_succeed",
			flagInputs: map[string]string{
				"name_to_greet":   "Bob",
				"emoji_suffix":    "🐈",
				"defaulted_input": "default",
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
			wantStdout: "Hello, Bob🐈\n",
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
				"name_to_greet":   "Bob",
				"emoji_suffix":    "🐈",
				"defaulted_input": "default",
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
			wantStdout: "Hello, Bob🐈\n",
			wantDestContents: map[string]string{
				"file1.txt": "old contents",
			},
			wantErr: "overwriting was not enabled",
		},
		{
			name:      "getter_error",
			getterErr: fmt.Errorf("fake error for testing"),
			wantErr:   "fake error for testing",
		},
		{
			name:         "errors_are_combined",
			getterErr:    fmt.Errorf("fake getter error for testing"),
			removeAllErr: fmt.Errorf("fake removeAll error for testing"),
			wantErr:      "fake getter error for testing\nfake removeAll error for testing",
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
			wantStdout: "Hello, Bob🐈\n",
			wantDestContents: map[string]string{
				"file1.txt":            "file1 contents",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantFlagInputs: map[string]string{
				"name_to_greet":   "Bob",
				"emoji_suffix":    "🐈",
				"defaulted_input": "default",
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
			wantStdout: "Working on environment production\nWorking on environment dev\n",
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
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Convert to OS-specific paths
			convertKeysToPlatformPaths(
				tc.templateContents,
				tc.existingDestContents,
				tc.wantBackupContents,
				tc.wantDestContents,
				tc.wantScratchContents,
				tc.wantTemplateContents,
			)

			tempDir := t.TempDir()
			dest := filepath.Join(tempDir, "dest")
			if err := common.WriteAllDefaultMode(dest, tc.existingDestContents); err != nil {
				t.Fatal(err)
			}
			tempDirNamer := func(namePart string) (string, error) {
				return filepath.Join(tempDir, namePart), nil
			}
			backupDir := filepath.Join(tempDir, "backups")
			rfs := &common.RealFS{}
			fg := &fakeGetter{
				err:    tc.getterErr,
				output: tc.templateContents,
			}
			stdoutBuf := &strings.Builder{}
			rp := &runParams{
				backupDir: backupDir,
				fs: &errorFS{
					AbstractFS:   rfs,
					removeAllErr: tc.removeAllErr,
				},
				getter:       fg,
				stdout:       stdoutBuf,
				tempDirNamer: tempDirNamer,
			}
			r := &Command{
				flags: RenderFlags{
					Dest:                dest,
					ForceOverwrite:      tc.flagForceOverwrite,
					Inputs:              tc.flagInputs,
					KeepTempDirs:        tc.flagKeepTempDirs,
					SkipInputValidation: tc.flagSkipInputValidation,
					Source:              "github.com/myorg/myrepo",
				},
			}

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))
			err := r.realRun(ctx, rp)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if fg.gotSource != r.flags.Source {
				t.Errorf("fake getter got template source %s but wanted %s", fg.gotSource, r.flags.Source)
			}

			if tc.wantFlagInputs != nil {
				if diff := cmp.Diff(r.flags.Inputs, tc.wantFlagInputs); diff != "" {
					t.Errorf("flagInputs was not as expected; (-got,+want): %s", diff)
				}
			}

			if diff := cmp.Diff(stdoutBuf.String(), tc.wantStdout); diff != "" {
				t.Errorf("template output was not as expected; (-got,+want): %s", diff)
			}

			gotTemplateContents := common.LoadDirWithoutMode(t, filepath.Join(tempDir, templateDirNamePart))
			if diff := cmp.Diff(gotTemplateContents, tc.wantTemplateContents, common.CmpFileMode); diff != "" {
				t.Errorf("template directory contents were not as expected (-got,+want): %s", diff)
			}

			gotScratchContents := common.LoadDirWithoutMode(t, filepath.Join(tempDir, scratchDirNamePart))
			if diff := cmp.Diff(gotScratchContents, tc.wantScratchContents, common.CmpFileMode); diff != "" {
				t.Errorf("scratch directory contents were not as expected (-got,+want): %s", diff)
			}

			gotDestContents := common.LoadDirWithoutMode(t, dest)
			if diff := cmp.Diff(gotDestContents, tc.wantDestContents, common.CmpFileMode); diff != "" {
				t.Errorf("dest directory contents were not as expected (-got,+want): %s", diff)
			}

			gotBackupContents := common.LoadDirWithoutMode(t, backupDir)
			gotBackupContents = stripFirstPathElem(gotBackupContents)
			if diff := cmp.Diff(gotBackupContents, tc.wantBackupContents, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("backups directory contents were not as expected (-got,+want): %s", diff)
			}
		})
	}
}

// Since os.MkdirTemp adds an extra random token, we strip it back out to get
// determistic results.
func stripFirstPathElem(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		// Panic in the case where k has no slashes; this is just a test helper.
		elems := strings.Split(k, string(filepath.Separator))
		newKey := filepath.Join(elems[1:]...)
		out[newKey] = v
	}
	return out
}

func TestSafeRelPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		want    string
		wantErr string
	}{
		{
			name: "plain_filename_succeeds",
			in:   "a.txt",
			want: "a.txt",
		},
		{
			name: "path_with_directories_succeeds",
			in:   "a/b.txt",
			want: "a/b.txt",
		},
		{
			name: "trailing_slash_succeeds",
			in:   "a/b/",
			want: "a/b/",
		},
		{
			name: "leading_slash_stripped",
			in:   "/a",
			want: "a",
		},
		{
			name: "leading_slash_with_more_dirs",
			in:   "/a/b/c",
			want: "a/b/c",
		},
		{
			name: "plain_slash_stripped",
			in:   "/",
			want: "",
		},
		{
			name:    "leading_dot_dot_fails",
			in:      "../a.txt",
			wantErr: "..",
		},
		{
			name:    "leading_dot_dot_with_more_dirs_fails",
			in:      "../a/b/c.txt",
			wantErr: "..",
		},
		{
			name:    "dot_dot_in_the_middle_fails",
			in:      "a/b/../c.txt",
			wantErr: "..",
		},
		{
			name:    "plain_dot_dot_fails",
			in:      "..",
			wantErr: "..",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := safeRelPath(nil, filepath.FromSlash(tc.in))

			want := filepath.FromSlash(tc.want)
			if got != want {
				t.Errorf("safeRelPath(%s): expected %q to be %q", tc.in, got, want)
			}
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestPromptForInputs(t *testing.T) {
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

			flagInputVals := map[string]string{}
			if tc.flagInputVals != nil {
				flagInputVals = maps.Clone(tc.flagInputVals)
			}

			cmd := &Command{
				flags: RenderFlags{
					Inputs: flagInputVals,
				},
			}

			stdinReader, stdinWriter := io.Pipe()
			stdoutReader, stdoutWriter := io.Pipe()
			_, stderrWriter := io.Pipe()

			cmd.SetStdin(stdinReader)
			cmd.SetStdout(stdoutWriter)
			cmd.SetStderr(stderrWriter)

			ctx := context.Background()
			errCh := make(chan error)
			go func() {
				defer close(errCh)
				errCh <- cmd.promptForInputs(ctx, &spec.Spec{
					Inputs: tc.inputs,
				})
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
			if diff := cmp.Diff(cmd.flags.Inputs, tc.want, cmpopts.EquateEmpty()); diff != "" {
				t.Fatalf("input values were different than expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestPromptForInputs_CanceledContext(t *testing.T) {
	t.Parallel()

	cmd := &Command{
		flags: RenderFlags{
			Inputs: map[string]string{},
		},
	}

	stdinReader, _ := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	_, stderrWriter := io.Pipe()

	cmd.SetStdin(stdinReader)
	cmd.SetStdout(stdoutWriter)
	cmd.SetStderr(stderrWriter)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error)
	go func() {
		defer close(errCh)
		errCh <- cmd.promptForInputs(ctx, &spec.Spec{
			Inputs: []*spec.Input{
				{
					Name: model.String{Val: "my_input"},
				},
			},
		})
	}()

	go func() {
		for {
			// Read and discard prompt printed to the user.
			if _, err := stdoutReader.Read(make([]byte, 1024)); err != nil {
				return
			}
		}
	}()

	cancel()
	var err error
	select {
	case err = <-errCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the background goroutine to finish")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got an error %v, want context.Canceled", err)
	}

	stdoutWriter.Close() // terminate the background goroutine blocking on stdoutReader.Read()
}

func TestValidateInputs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		inputModels []*spec.Input
		inputVals   map[string]string
		want        string
	}{
		{
			name: "no-validation-rule",
			inputModels: []*spec.Input{
				{
					Name: model.String{Val: "my_input"},
				},
			},
			inputVals: map[string]string{
				"my_input": "foo",
			},
		},
		{
			name: "single-passing-validation-rule",
			inputModels: []*spec.Input{
				{
					Name: model.String{Val: "my_input"},
					Rules: []*spec.InputRule{
						{
							Rule:    model.String{Val: `size(my_input) < 5`},
							Message: model.String{Val: "Length must be less than 5"},
						},
					},
				},
			},
			inputVals: map[string]string{
				"my_input": "foo",
			},
		},
		{
			name: "single-failing-validation-rule",
			inputModels: []*spec.Input{
				{
					Name: model.String{Val: "my_input"},
					Rules: []*spec.InputRule{
						{
							Rule:    model.String{Val: `size(my_input) < 3`},
							Message: model.String{Val: "Length must be less than 3"},
						},
					},
				},
			},
			inputVals: map[string]string{
				"my_input": "foo",
			},
			want: `input validation failed:

Input name:   my_input
Input value:  foo
Rule:         size(my_input) < 3
Rule msg:     Length must be less than 3`,
		},
		{
			name: "multiple-passing-validation-rules",
			inputModels: []*spec.Input{
				{
					Name: model.String{Val: "my_input"},
					Rules: []*spec.InputRule{
						{
							Rule:    model.String{Val: `size(my_input) < 5`},
							Message: model.String{Val: "Length must be less than 5"},
						},
						{
							Rule:    model.String{Val: `my_input.startsWith("fo")`},
							Message: model.String{Val: `Must start with "fo"`},
						},
						{
							Rule:    model.String{Val: `my_input.contains("oo")`},
							Message: model.String{Val: `Must contain "oo"`},
						},
					},
				},
			},
			inputVals: map[string]string{
				"my_input": "foo",
			},
		},
		{
			name: "multiple-passing-validation-rules-one-failing",
			inputModels: []*spec.Input{
				{
					Name: model.String{Val: "my_input"},
					Rules: []*spec.InputRule{
						{
							Rule:    model.String{Val: `size(my_input) < 3`},
							Message: model.String{Val: "Length must be less than 3"},
						},
						{
							Rule:    model.String{Val: `my_input.startsWith("fo")`},
							Message: model.String{Val: `Must start with "fo"`},
						},
						{
							Rule:    model.String{Val: `my_input.contains("oo")`},
							Message: model.String{Val: `Must contain "oo"`},
						},
					},
				},
			},
			inputVals: map[string]string{
				"my_input": "foo",
			},
			want: `input validation failed:

Input name:   my_input
Input value:  foo
Rule:         size(my_input) < 3
Rule msg:     Length must be less than 3`,
		},
		{
			name: "multiple-failing-validation-rules",
			inputModels: []*spec.Input{
				{
					Name: model.String{Val: "my_input"},
					Rules: []*spec.InputRule{
						{
							Rule:    model.String{Val: `size(my_input) < 3`},
							Message: model.String{Val: "Length must be less than 3"},
						},
						{
							Rule:    model.String{Val: `my_input.startsWith("ham")`},
							Message: model.String{Val: `Must start with "ham"`},
						},
						{
							Rule:    model.String{Val: `my_input.contains("shoe")`},
							Message: model.String{Val: `Must contain "shoe"`},
						},
					},
				},
			},
			inputVals: map[string]string{
				"my_input": "foo",
			},
			want: `input validation failed:

Input name:   my_input
Input value:  foo
Rule:         size(my_input) < 3
Rule msg:     Length must be less than 3

Input name:   my_input
Input value:  foo
Rule:         my_input.startsWith("ham")
Rule msg:     Must start with "ham"

Input name:   my_input
Input value:  foo
Rule:         my_input.contains("shoe")
Rule msg:     Must contain "shoe"`,
		},
		{
			name: "cel-syntax-error",
			inputModels: []*spec.Input{
				{
					Name: model.String{Val: "my_input"},
					Rules: []*spec.InputRule{
						{
							Rule: model.String{Val: `(`},
						},
					},
				},
			},
			inputVals: map[string]string{
				"my_input": "foo",
			},
			want: `input validation failed:

Input name:   my_input
Input value:  foo
Rule:         (
CEL error:    failed compiling CEL expression: ERROR: <input>:1:2: Syntax error:`, // remainder of error omitted
		},
		{
			name: "cel-type-conversion-error",
			inputModels: []*spec.Input{
				{
					Name: model.String{Val: "my_input"},
					Rules: []*spec.InputRule{
						{
							Rule: model.String{Val: `bool(42)`},
						},
					},
				},
			},
			inputVals: map[string]string{
				"my_input": "foo",
			},
			want: `input validation failed:

Input name:   my_input
Input value:  foo
Rule:         bool(42)
CEL error:    failed compiling CEL expression: ERROR: <input>:1:5: found no matching overload for 'bool'`, // remainder of error omitted
		},
		{
			name: "cel-output-type-conversion-error",
			inputModels: []*spec.Input{
				{
					Name: model.String{Val: "my_input"},
					Rules: []*spec.InputRule{
						{
							Rule: model.String{Val: `42`},
						},
					},
				},
			},
			inputVals: map[string]string{
				"my_input": "foo",
			},
			want: `input validation failed:

Input name:   my_input
Input value:  foo
Rule:         42
CEL error:    CEL expression result couldn't be converted to bool. The CEL engine error was: unsupported type conversion from 'int' to bool`, // remainder of error omitted
		},
		{
			name: "multi-input-validation",
			inputModels: []*spec.Input{
				{
					Name: model.String{Val: "my_input"},
					Rules: []*spec.InputRule{
						{
							Rule: model.String{Val: `my_input + my_other_input == "sharknado"`},
						},
					},
				},
				{
					Name: model.String{Val: "my_other_input"},
					Rules: []*spec.InputRule{
						{
							Rule: model.String{Val: `"tor" + my_other_input + my_input == "tornadoshark"`},
						},
					},
				},
			},
			inputVals: map[string]string{
				"my_input":       "shark",
				"my_other_input": "nado",
			},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := &Command{
				flags: RenderFlags{
					Inputs: tc.inputVals,
				},
			}
			ctx := context.Background()
			err := r.validateInputs(ctx, tc.inputModels)
			if diff := testutil.DiffErrString(err, tc.want); diff != "" {
				t.Error(diff)
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

func modelStrings(ss []string) []model.String {
	out := make([]model.String, len(ss))
	for i, s := range ss {
		out[i] = model.String{
			Pos: &model.ConfigPos{}, // for the purposes of testing, "location unknown" is fine.
			Val: s,
		}
	}
	return out
}

// A renderFS implementation that can inject errors for testing.
type errorFS struct {
	common.AbstractFS

	mkdirAllErr  error
	openErr      error
	openFileErr  error
	readFileErr  error
	removeAllErr error
	statErr      error
	writeFileErr error
}

func (e *errorFS) MkdirAll(name string, mode fs.FileMode) error {
	if e.mkdirAllErr != nil {
		return e.mkdirAllErr
	}
	return e.AbstractFS.MkdirAll(name, mode)
}

func (e *errorFS) Open(name string) (fs.File, error) {
	if e.openErr != nil {
		return nil, e.openErr
	}
	return e.AbstractFS.Open(name)
}

func (e *errorFS) OpenFile(name string, flag int, mode os.FileMode) (*os.File, error) {
	if e.openFileErr != nil {
		return nil, e.openFileErr
	}
	return e.AbstractFS.OpenFile(name, flag, mode)
}

func (e *errorFS) ReadFile(name string) ([]byte, error) {
	if e.readFileErr != nil {
		return nil, e.readFileErr
	}
	return e.AbstractFS.ReadFile(name)
}

func (e *errorFS) RemoveAll(name string) error {
	if e.removeAllErr != nil {
		return e.removeAllErr
	}
	return e.AbstractFS.RemoveAll(name)
}

func (e *errorFS) Stat(name string) (fs.FileInfo, error) {
	if e.statErr != nil {
		return nil, e.statErr
	}
	return e.AbstractFS.Stat(name)
}

func (e *errorFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	if e.writeFileErr != nil {
		return e.writeFileErr
	}
	return e.AbstractFS.WriteFile(name, data, perm)
}

type fakeGetter struct {
	gotSource string
	output    map[string]string
	err       error
}

func (f *fakeGetter) Get(ctx context.Context, req *getter.Request) (*getter.GetResult, error) {
	f.gotSource = req.Src
	if f.err != nil {
		return nil, f.err
	}
	if err := common.WriteAllDefaultMode(req.Dst, f.output); err != nil {
		return nil, err
	}
	return &getter.GetResult{Dst: req.Dst}, nil
}

// convertKeysToPlatformPaths is a helper that converts the keys in all of the
// given maps to a platform-specific file path. The maps are modified in place.
func convertKeysToPlatformPaths[T any](maps ...map[string]T) {
	for _, m := range maps {
		for k, v := range m {
			if newKey := filepath.FromSlash(k); newKey != k {
				m[newKey] = v
				delete(m, k)
			}
		}
	}
}

// toPlatformPaths converts each element of each input slice from a/b/c style
// forward slash paths to platform-specific file paths. The slices are modified
// in place.
func toPlatformPaths(slices ...[]string) {
	for _, s := range slices {
		for i, elem := range s {
			s[i] = filepath.FromSlash(elem)
		}
	}
}
