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
	"fmt"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
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
				"--source", "helloworld@v1",
				"--spec", "my_spec.yaml",
				"--dest", "my_dir",
				"--git-protocol", "https",
				"--input", "x=y",
				"--log-level", "info",
				"--force-overwrite",
				"--keep-temp-dirs",
			},
			want: &Render{
				flagSource:         "helloworld@v1",
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
				"-s", "helloworld@v1",
			},
			want: &Render{
				flagSource:         "helloworld@v1",
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
			name:    "required_flag_is_missing",
			args:    []string{},
			wantErr: "--source is required",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := &Render{}
			err := r.parseFlags(tc.args)
			if tc.wantErr != "" {
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
			fs:      &errorFS{err: fmt.Errorf("yikes")},
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

type errorFS struct {
	fs.StatFS
	err error
}

func (e *errorFS) Stat(string) (fs.FileInfo, error) {
	return nil, e.err
}
