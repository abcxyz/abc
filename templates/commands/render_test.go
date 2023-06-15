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
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/go-getter/v2"
)

func TestParseFlags(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    []string
		want    *Render
		wantErr string
	}{
		{
			name: "all_flags_present",
			args: []string{
				"--spec", "my_spec.yaml",
				"--dest", "my_dir",
				"--git-protocol", "https",
				"--input", "x=y",
				"--log-level", "info",
				"--force-overwrite",
				"--keep-temp-dirs",
				"helloworld@v1",
			},
			want: &Render{
				source:             "helloworld@v1",
				flagSpec:           "my_spec.yaml",
				flagDest:           "my_dir",
				flagGitProtocol:    "https",
				flagInputs:         map[string]string{"x": "y"},
				flagLogLevel:       "info",
				flagForceOverwrite: true,
				flagKeepTempDirs:   true,
			},
		},
		{
			name: "minimal_flags_present",
			args: []string{
				"helloworld@v1",
			},
			want: &Render{
				source:             "helloworld@v1",
				flagSpec:           "./spec.yaml",
				flagDest:           ".",
				flagGitProtocol:    "https",
				flagInputs:         nil,
				flagLogLevel:       "warning",
				flagForceOverwrite: false,
				flagKeepTempDirs:   false,
			},
		},
		{
			name:    "required_source_is_missing",
			args:    []string{},
			wantErr: flag.ErrHelp.Error(),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := &Render{}
			err := r.parseFlags(tc.args)
			if err != nil || tc.wantErr != "" {
				if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
					t.Fatal(diff)
				}
				return
			}
			opts := []cmp.Option{
				cmp.AllowUnexported(Render{}),
				cmpopts.IgnoreFields(Render{}, "BaseCommand"),
			}
			if diff := cmp.Diff(r, tc.want, opts...); diff != "" {
				t.Errorf("got %#v, want %#v, diff (-got, +want): %v", r, tc.want, diff)
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
			wantErr: "is not a directory",
		},
		{
			name:    "dest_doesnt_exist_should_fail",
			dest:    "my/dir",
			fs:      fstest.MapFS{},
			wantErr: "doesn't exist",
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

	specContents := `
apiVersion: 'abcxyz.dev/cli/v1alpha1'
kind: 'Template'
desc: 'A template for the ages'
inputs:
- name: 'name_to_greet'
  desc: 'A name to include in the message'
  required: true
steps:
- desc: 'Print a message'
  action: 'print'
  params:
    message: 'Hello, {{.name_to_greet}}'
- desc: 'Include some files and directories'
  action: 'include'
  params:
    paths: ['file1.txt', 'dir1', 'dir2/file2.txt']
`

	cases := []struct {
		name                 string
		templateContents     map[string]string
		existingDestContents map[string]string
		flagInputs           map[string]string
		flagKeepTempDirs     bool
		flagSpec             string
		flagForceOverwrite   bool
		getterErr            error
		removeAllErr         error
		wantScratchContents  map[string]string
		wantTemplateContents map[string]string
		wantDestContents     map[string]string
		wantStdout           string
		wantErr              string
	}{
		{
			name: "simple_success",
			flagInputs: map[string]string{
				"name_to_greet": "🐈",
			},
			templateContents: map[string]string{
				"myfile.txt":           "Some random stuff",
				"spec.yaml":            specContents,
				"file1.txt":            "file1 contents",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantStdout: "Hello, 🐈\n",
			wantDestContents: map[string]string{
				"file1.txt":            "file1 contents",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
		},
		{
			name: "keep_temp_dirs_on_success_if_flag",
			flagInputs: map[string]string{
				"name_to_greet": "🐈",
			},
			flagKeepTempDirs: true,
			flagSpec:         "spec.yaml",
			templateContents: map[string]string{
				"spec.yaml":            specContents,
				"file1.txt":            "file1 contents",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantStdout: "Hello, 🐈\n",
			wantScratchContents: map[string]string{
				"file1.txt":            "file1 contents",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantTemplateContents: map[string]string{
				"spec.yaml":            specContents,
				"file1.txt":            "file1 contents",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
			wantDestContents: map[string]string{
				"file1.txt":            "file1 contents",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
		},
		{
			name: "keep_temp_dirs_on_failure_if_flag",
			flagInputs: map[string]string{
				"name_to_greet": "🐈",
			},
			flagKeepTempDirs: true,
			flagSpec:         "spec.yaml",
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
				"name_to_greet": "🐈",
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
			wantStdout: "Hello, 🐈\n",
			wantDestContents: map[string]string{
				"file1.txt":            "new contents",
				"dir1/file_in_dir.txt": "file_in_dir contents",
				"dir2/file2.txt":       "file2 contents",
			},
		},
		{
			name: "existing_dest_file_without_overwrite_flag_should_fail",
			flagInputs: map[string]string{
				"name_to_greet": "🐈",
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
			wantStdout: "Hello, 🐈\n",
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
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			dest := filepath.Join(tempDir, "dest")
			if err := writeAllDefaultMode(dest, tc.existingDestContents); err != nil {
				t.Fatal(err)
			}
			tempDirNamer := func(namePart string) (string, error) {
				return filepath.Join(tempDir, namePart), nil
			}
			fg := &fakeGetter{
				err:    tc.getterErr,
				output: tc.templateContents,
			}
			stdoutBuf := &strings.Builder{}
			rp := &runParams{
				fs: &errorFS{
					renderFS:     &realFS{},
					removeAllErr: tc.removeAllErr,
				},
				getter:       fg,
				stdout:       stdoutBuf,
				tempDirNamer: tempDirNamer,
			}
			r := &Render{
				flagDest:           dest,
				flagForceOverwrite: tc.flagForceOverwrite,
				flagInputs:         tc.flagInputs,
				flagKeepTempDirs:   tc.flagKeepTempDirs,
				flagSpec:           "spec.yaml",
				source:             "github.com/myorg/myrepo",
			}
			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))
			err := r.realRun(ctx, rp)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if fg.gotSource != r.source {
				t.Errorf("fake getter got template source %s but wanted %s", fg.gotSource, r.source)
			}

			if diff := cmp.Diff(stdoutBuf.String(), tc.wantStdout); diff != "" {
				t.Errorf("template output was not as expected; (-got,+want): %s", diff)
			}

			gotTemplateContents := loadDirWithoutMode(t, filepath.Join(tempDir, templateDirNamePart))
			if diff := cmp.Diff(gotTemplateContents, tc.wantTemplateContents); diff != "" {
				t.Errorf("template directory contents were not as expected (-got,+want): %s", diff)
			}

			gotScratchContents := loadDirWithoutMode(t, filepath.Join(tempDir, scratchDirNamePart))
			if diff := cmp.Diff(gotScratchContents, tc.wantScratchContents); diff != "" {
				t.Errorf("scratch directory contents were not as expected (-got,+want): %s", diff)
			}

			gotDestContents := loadDirWithoutMode(t, dest)
			if diff := cmp.Diff(gotDestContents, tc.wantDestContents); diff != "" {
				t.Errorf("dest directory contents were not as expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestSafeRelPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		wantErr string
	}{
		{
			name: "plain_filename_succeeds",
			in:   "a.txt",
		},
		{
			name: "path_with_directories_succeeds",
			in:   "a/b.txt",
		},
		{
			name: "trailing_slash_succeeds",
			in:   "a/b/",
		},
		{
			name:    "leading_slash_fails",
			in:      "/a",
			wantErr: "absolute",
		},
		{
			name:    "leading_slash_with_more_dirs_fails",
			in:      "/a/b/c",
			wantErr: "absolute",
		},
		{
			name:    "leading_slash_with_more_dirs_fails",
			in:      "/a/b/c",
			wantErr: "absolute",
		},
		{
			name:    "plain_slash_fails",
			in:      "/",
			wantErr: "absolute",
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

			got := safeRelPath(nil, tc.in)

			if testutil.DiffErrString(got, tc.wantErr) != "" {
				t.Errorf("safeRelPath(%s)=%s, want %s", tc.in, got, tc.wantErr)
			}
		})
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
