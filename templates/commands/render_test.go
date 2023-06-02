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
	"testing"
	"testing/fstest"

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

	cases := []struct {
		name             string
		templateContents map[string]string
		keepTempDirs     bool
		getterErr        error
		removeAllErr     error
		wantErr          string
	}{
		{
			name: "simple_success",
			templateContents: map[string]string{
				"file1.txt": "My first file",
				"file2.txt": "My second file",
			},
		},
		{
			name:         "keep_temp_dirs_on_success",
			keepTempDirs: true,
			templateContents: map[string]string{
				"file1.txt": "My first file",
				"file2.txt": "My second file",
			},
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
			templateDir := filepath.Join(tempDir, "template")
			templateDirNamer := func(debugID string) (string, error) {
				return templateDir, nil
			}
			rp := &runParams{
				getter: &fakeGetter{
					wantSource: "github.com/myorg/myrepo",
					output:     tc.templateContents,
					err:        tc.getterErr,
				},
				fs: &errorFS{
					renderFS:     &realFS{},
					removeAllErr: tc.removeAllErr,
				},
				tempDirNamer: templateDirNamer,
			}
			r := &Render{
				source:           "github.com/myorg/myrepo",
				flagKeepTempDirs: tc.keepTempDirs,
			}
			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))
			err := r.realRun(ctx, rp)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			wantFiles := tc.templateContents
			if !tc.keepTempDirs {
				wantFiles = nil
			}

			got := loadDirContents(t, templateDir)
			if diff := cmp.Diff(got, wantFiles); diff != "" {
				t.Errorf("output files were not as expected (-got,+want): %s", diff)
			}
		})
	}
}

// Read all the files recursively under "dir", returning their contents as a
// map[filename]->contents. Returns nil if dir doesn't exist.
func loadDirContents(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		t.Fatal(err)
	}
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
		out[rel] = string(contents)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(): %v", err)
	}
	return out
}

// A renderFS implementation that can inject errors for testing.
type errorFS struct {
	renderFS

	statErr      error
	removeAllErr error
	openErr      error
}

func (e *errorFS) Stat(name string) (fs.FileInfo, error) {
	if e.statErr != nil {
		return nil, e.statErr
	}
	return e.renderFS.Stat(name)
}

func (e *errorFS) RemoveAll(name string) error {
	if e.removeAllErr != nil {
		return e.removeAllErr
	}
	return e.renderFS.RemoveAll(name)
}

func (e *errorFS) Open(name string) (fs.File, error) {
	if e.openErr != nil {
		return nil, e.openErr
	}
	return e.renderFS.Open(name)
}

type fakeGetter struct {
	wantSource string
	output     map[string]string
	err        error
}

func (f *fakeGetter) Get(ctx context.Context, req *getter.Request) (*getter.GetResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	if req.Src != f.wantSource {
		return nil, fmt.Errorf("got template source %s but want %s", req.Src, f.wantSource)
	}
	for path, contents := range f.output {
		fullPath := filepath.Join(req.Dst, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("MkdirAll(%q): %w", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(contents), 0o600); err != nil {
			return nil, fmt.Errorf("WriteFile(%q): %w", fullPath, err)
		}
	}
	return &getter.GetResult{Dst: req.Dst}, nil
}
