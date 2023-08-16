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

package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/go-getter/v2"
	"golang.org/x/exp/maps"
)

// cmpFileMode is a cmp option that handles the conflict between Unix and
// Windows systems file permissions.
var cmpFileMode = cmp.Comparer(func(a, b fs.FileMode) bool {
	// Windows really only has 2 file modes: 0666 and 0444[1]. Thus we only check
	// the first bit, since we know the remaining bits will be the same.
	// Furthermore, there's no reliable way to know whether the executive bit is
	// set[2], so we ignore it.
	//
	// I tried doing fancy bitmasking stuff here, but apparently umasks on Windows
	// are undefined(?), so we've resorted to substring matching - hooray.
	//
	// [1]: https://medium.com/@MichalPristas/go-and-file-perms-on-windows-3c944d55dd44
	// [2]: https://github.com/golang/go/issues/41809
	if runtime.GOOS == "windows" {
		// A filemode of 0644 would show as "-rw-r--r--", but on Windows we only
		// care about the first bit (which is the first 3 characters in the output
		// string).
		return a.Perm().String()[1:3] == b.Perm().String()[1:3]
	}

	return a == b
})

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
				"helloworld@v1",
			},
			want: RenderFlags{
				Source:         "helloworld@v1",
				Dest:           "my_dir",
				GitProtocol:    "https",
				Inputs:         map[string]string{"x": "y"},
				LogLevel:       "info",
				ForceOverwrite: true,
				KeepTempDirs:   true,
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

			var cmd RenderCommand
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
apiVersion: 'cli.abcxyz.dev/v1alpha1'
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
    paths: ['file1.txt', 'dir1', 'dir2/file2.txt']
- desc: 'Replace "blue" with "red"'
  action: 'string_replace'
  params:
    paths: ['.']
    replacements:
    - to_replace: 'blue'
      with: 'red'
`

	cases := []struct {
		name                 string
		templateContents     map[string]string
		existingDestContents map[string]string
		flagInputs           map[string]string
		flagKeepTempDirs     bool
		flagForceOverwrite   bool
		getterErr            error
		removeAllErr         error
		wantScratchContents  map[string]string
		wantTemplateContents map[string]string
		wantDestContents     map[string]string
		wantBackupContents   map[string]string
		wantFlagInputs       map[string]string
		wantStdout           string
		wantErr              string
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
			wantErr: "error parsing YAML spec file",
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
apiVersion: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'my template'
steps:
  - desc: 'Include from destination'
    action: 'include'
    params:
        from: 'destination'
        paths:
          - 'myfile.txt'
          - 'subdir_a'
          - 'subdir_b/file_b.txt'
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
apiVersion: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'my template'
steps:
  - desc: 'Include from destination'
    action: 'include'
    params:
        from: 'destination'
        paths:
          - 'file_a.txt'
  - desc: 'Include from template'
    action: 'include'
    params:
        paths:
        - 'file_b.txt'
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
				"spec.yaml": `apiVersion: 'cli.abcxyz.dev/v1alpha1'
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
			if err := writeAllDefaultMode(dest, tc.existingDestContents); err != nil {
				t.Fatal(err)
			}
			tempDirNamer := func(namePart string) (string, error) {
				return filepath.Join(tempDir, namePart), nil
			}
			backupDir := filepath.Join(tempDir, "backups")
			rfs := &realFS{}
			fg := &fakeGetter{
				err:    tc.getterErr,
				output: tc.templateContents,
			}
			stdoutBuf := &strings.Builder{}
			rp := &runParams{
				backupDir: backupDir,
				fs: &errorFS{
					renderFS:     rfs,
					removeAllErr: tc.removeAllErr,
				},
				getter:       fg,
				stdout:       stdoutBuf,
				tempDirNamer: tempDirNamer,
			}
			r := &RenderCommand{
				flags: RenderFlags{
					Dest:           dest,
					ForceOverwrite: tc.flagForceOverwrite,
					Inputs:         tc.flagInputs,
					KeepTempDirs:   tc.flagKeepTempDirs,
					Source:         "github.com/myorg/myrepo",
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

			gotTemplateContents := loadDirWithoutMode(t, filepath.Join(tempDir, templateDirNamePart))
			if diff := cmp.Diff(gotTemplateContents, tc.wantTemplateContents, cmpFileMode); diff != "" {
				t.Errorf("template directory contents were not as expected (-got,+want): %s", diff)
			}

			gotScratchContents := loadDirWithoutMode(t, filepath.Join(tempDir, scratchDirNamePart))
			if diff := cmp.Diff(gotScratchContents, tc.wantScratchContents, cmpFileMode); diff != "" {
				t.Errorf("scratch directory contents were not as expected (-got,+want): %s", diff)
			}

			gotDestContents := loadDirWithoutMode(t, dest)
			if diff := cmp.Diff(gotDestContents, tc.wantDestContents, cmpFileMode); diff != "" {
				t.Errorf("dest directory contents were not as expected (-got,+want): %s", diff)
			}

			gotBackupContents := loadDirWithoutMode(t, backupDir)
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
		inputs        []*model.Input
		flagInputVals map[string]string // Simulates some inputs having already been provided by flags, like --input=foo=bar means we shouldn't prompt for "foo"
		dialog        []dialogStep
		want          map[string]string
		wantErr       string
	}{
		{
			name: "single_input_prompt",
			inputs: []*model.Input{
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
			name: "multiple_input_prompts",
			inputs: []*model.Input{
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
			inputs: []*model.Input{
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
			inputs: []*model.Input{
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
			inputs: []*model.Input{},
		},
		{
			name: "single_input_with_default_accepted",
			inputs: []*model.Input{
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
			inputs: []*model.Input{
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
			inputs: []*model.Input{
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

			cmd := &RenderCommand{
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
				errCh <- cmd.promptForInputs(ctx, &model.Spec{
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

	cmd := &RenderCommand{
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
		errCh <- cmd.promptForInputs(ctx, &model.Spec{
			Inputs: []*model.Input{
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

// Read all the files recursively under "dir", returning their contents as a
// map[filename]->contents. Returns nil if dir doesn't exist.
func loadDirContents(t *testing.T, dir string) map[string]modeAndContents {
	t.Helper()

	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		t.Fatal(err)
	}
	out := map[string]modeAndContents{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("ReadFile(): %w", err)
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("Rel(): %w", err)
		}
		fi, err := d.Info()
		if err != nil {
			t.Fatal(err)
		}
		out[rel] = modeAndContents{
			Mode:     fi.Mode(),
			Contents: string(contents),
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(): %v", err)
	}
	return out
}

func loadDirWithoutMode(t *testing.T, dir string) map[string]string {
	t.Helper()

	withMode := loadDirContents(t, dir)
	if withMode == nil {
		return nil
	}
	out := map[string]string{}
	for name, mc := range withMode {
		out[name] = mc.Contents
	}
	return out
}

// A renderFS implementation that can inject errors for testing.
type errorFS struct {
	renderFS

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
	return e.renderFS.MkdirAll(name, mode)
}

func (e *errorFS) Open(name string) (fs.File, error) {
	if e.openErr != nil {
		return nil, e.openErr
	}
	return e.renderFS.Open(name)
}

func (e *errorFS) OpenFile(name string, flag int, mode os.FileMode) (*os.File, error) {
	if e.openFileErr != nil {
		return nil, e.openFileErr
	}
	return e.renderFS.OpenFile(name, flag, mode)
}

func (e *errorFS) ReadFile(name string) ([]byte, error) {
	if e.readFileErr != nil {
		return nil, e.readFileErr
	}
	return e.renderFS.ReadFile(name)
}

func (e *errorFS) RemoveAll(name string) error {
	if e.removeAllErr != nil {
		return e.removeAllErr
	}
	return e.renderFS.RemoveAll(name)
}

func (e *errorFS) Stat(name string) (fs.FileInfo, error) {
	if e.statErr != nil {
		return nil, e.statErr
	}
	return e.renderFS.Stat(name)
}

func (e *errorFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	if e.writeFileErr != nil {
		return e.writeFileErr
	}
	return e.renderFS.WriteFile(name, data, perm)
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
	if err := writeAllDefaultMode(req.Dst, f.output); err != nil {
		return nil, err
	}
	return &getter.GetResult{Dst: req.Dst}, nil
}

type modeAndContents struct {
	Mode     os.FileMode
	Contents string
}

// writeAllDefaultMode wraps writeAll and sets file permissions to 0600.
func writeAllDefaultMode(root string, files map[string]string) error {
	withMode := map[string]modeAndContents{}
	for name, contents := range files {
		withMode[name] = modeAndContents{
			Mode:     0o600,
			Contents: contents,
		}
	}
	return writeAll(root, withMode)
}

// writeAll saves the given file contents with the given permissions.
func writeAll(root string, files map[string]modeAndContents) error {
	for path, mc := range files {
		fullPath := filepath.Join(root, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("MkdirAll(%q): %w", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(mc.Contents), mc.Mode); err != nil {
			return fmt.Errorf("WriteFile(%q): %w", fullPath, err)
		}
		// The user's may have prevented the file from being created with the
		// desired permissions. Use chmod to really set the desired permissions.
		if err := os.Chmod(fullPath, mc.Mode); err != nil {
			return fmt.Errorf("Chmod(): %w", err)
		}
	}

	return nil
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
