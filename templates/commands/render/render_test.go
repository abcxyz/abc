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
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/abc/templates/common"
	abctestutil "github.com/abcxyz/abc/templates/testutil"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
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
				"--input-file", "abc-inputs.yaml",
				"--force-overwrite",
				"--keep-temp-dirs",
				"--skip-input-validation",
				"--debug-scratch-contents",
				"--debug-step-diffs",
				"helloworld@v1",
			},
			want: RenderFlags{
				Source:               "helloworld@v1",
				Dest:                 "my_dir",
				GitProtocol:          "https",
				Inputs:               map[string]string{"x": "y"},
				InputFiles:           []string{"abc-inputs.yaml"},
				ForceOverwrite:       true,
				KeepTempDirs:         true,
				SkipInputValidation:  true,
				DebugScratchContents: true,
				DebugStepDiffs:       true,
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
			fs:      &common.ErrorFS{StatErr: fmt.Errorf("yikes")},
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

func TestRenderPrompt(t *testing.T) {
	t.Parallel()

	specContents := `
api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'
desc: 'A template for the ages'
inputs:
- name: 'name_of_favourite_person'
  desc: 'The name of favourite person'
steps:
- desc: 'Include some files and directories'
  action: 'include'
  params:
    paths:
      - paths: ['file1.txt', 'dir1', 'dir2/file2.txt']
- desc: 'Replace "Alice" with [input]'
  action: 'string_replace'
  params:
    paths: ['.']
    replacements:
    - to_replace: 'Alice'
      with: '{{.name_of_favourite_person}}'
`

	cases := []struct {
		name             string
		templateContents map[string]string
		flagPrompt       bool
		dialog           []abctestutil.DialogStep
		wantDestContents map[string]string
		wantErr          string
	}{
		{
			name: "simple_success",
			templateContents: map[string]string{
				"myfile.txt":           "Some random stuff",
				"spec.yaml":            specContents,
				"file1.txt":            "my favorite person is Alice",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			flagPrompt: true,
			dialog: []abctestutil.DialogStep{
				{
					WaitForPrompt: `
Input name:   name_of_favourite_person
Description:  The name of favourite person

Enter value: `,
					ThenRespond: "Bob\n",
				},
			},
			wantDestContents: map[string]string{
				"file1.txt":            "my favorite person is Bob",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			dest := filepath.Join(tempDir, "dest")
			sourceDir := filepath.Join(tempDir, "source")

			abctestutil.WriteAllDefaultMode(t, sourceDir, tc.templateContents)

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))

			var args []string
			if tc.flagPrompt {
				args = append(args, "--prompt")
			}
			args = append(args, fmt.Sprintf("--dest=%s", dest))
			args = append(args, sourceDir)

			r := &Command{skipPromptTTYCheck: true}
			stdinReader, stdinWriter := io.Pipe()
			stdoutReader, stdoutWriter := io.Pipe()
			_, stderrWriter := io.Pipe()

			r.SetStdin(stdinReader)
			r.SetStdout(stdoutWriter)
			r.SetStderr(stderrWriter)

			errCh := make(chan error)
			go func() {
				defer close(errCh)
				err := r.Run(ctx, args)
				errCh <- err
			}()

			for _, ds := range tc.dialog {
				abctestutil.ReadWithTimeout(t, stdoutReader, ds.WaitForPrompt)
				abctestutil.WriteWithTimeout(t, stdinWriter, ds.ThenRespond)
			}

			select {
			case err := <-errCh:
				if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
					t.Fatal(diff)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for background goroutine to finish")
			}

			gotDestContents := abctestutil.LoadDirWithoutMode(t, dest)
			if diff := cmp.Diff(gotDestContents, tc.wantDestContents, abctestutil.CmpFileMode); diff != "" {
				t.Errorf("dest directory contents were not as expected (-got,+want): %s", diff)
			}
		})
	}
}
