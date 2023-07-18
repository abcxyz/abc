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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abcxyz/abc/templates/model"
	abctestutil "github.com/abcxyz/abc/testutil"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
	"github.com/benbjohnson/clock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/exp/slices"
)

func TestWalkAndModify(t *testing.T) {
	t.Parallel()

	fooToBarVisitor := func(buf []byte) ([]byte, error) {
		return bytes.ReplaceAll(buf, []byte("foo"), []byte("bar")), nil
	}

	cases := []struct {
		name            string
		visitor         walkAndModifyVisitor
		relPath         string
		initialContents map[string]string
		want            map[string]string
		wantErr         string

		// fakeable errors
		readFileErr  error
		statErr      error
		writeFileErr error
	}{
		{
			name:            "simple_single_file_replacement_should_work",
			visitor:         fooToBarVisitor,
			relPath:         "my_file.txt",
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc bar def"},
		},
		{
			name:            "multiple_replacements_should_work",
			visitor:         fooToBarVisitor,
			relPath:         "my_file.txt",
			initialContents: map[string]string{"my_file.txt": "foo foo"}, //nolint:dupword
			want:            map[string]string{"my_file.txt": "bar bar"}, //nolint:dupword
		},
		{
			name:            "dot_dir_should_work",
			visitor:         fooToBarVisitor,
			relPath:         ".",
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc bar def"},
		},
		{
			name:            "empty_path_means_root_should_work",
			visitor:         fooToBarVisitor,
			relPath:         ".",
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc bar def"},
		},
		{
			name:            "dot_dir_with_trailing_slash_should_work",
			visitor:         fooToBarVisitor,
			relPath:         "./",
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc bar def"},
		},
		{
			name:            "single_subdir_should_work",
			visitor:         fooToBarVisitor,
			relPath:         "./dir",
			initialContents: map[string]string{"dir/my_file.txt": "abc foo def"},
			want:            map[string]string{"dir/my_file.txt": "abc bar def"},
		},
		{
			name:            "named_file_in_subdir_should_work",
			visitor:         fooToBarVisitor,
			relPath:         "dir/my_file.txt",
			initialContents: map[string]string{"dir/my_file.txt": "abc foo def"},
			want:            map[string]string{"dir/my_file.txt": "abc bar def"},
		},
		{
			name:            "deeply_nested_dirs_should_work",
			visitor:         fooToBarVisitor,
			relPath:         "dir1",
			initialContents: map[string]string{"dir1/dir2/dir3/dir4/my_file.txt": "abc foo def"},
			want:            map[string]string{"dir1/dir2/dir3/dir4/my_file.txt": "abc bar def"},
		},
		{
			name:    "one_included_dir_one_excluded",
			visitor: fooToBarVisitor,
			relPath: "dir1",
			initialContents: map[string]string{
				"dir1/should_change.txt":     "abc foo def",
				"dir2/should_not_change.txt": "ghi foo jkl",
			},
			want: map[string]string{
				"dir1/should_change.txt":     "abc bar def",
				"dir2/should_not_change.txt": "ghi foo jkl",
			},
		},
		{
			name:            "nonexistent_path_should_fail",
			visitor:         fooToBarVisitor,
			relPath:         "nonexistent",
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc foo def"},
			wantErr:         "doesn't exist in the scratch directory",
		},
		{
			name:            "dot_dot_traversal_should_fail",
			visitor:         fooToBarVisitor,
			relPath:         "abc/..",
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc foo def"},
			wantErr:         `must not contain ".."`,
		},
		{
			name:            "absolute_path_should_become_relative",
			visitor:         fooToBarVisitor,
			relPath:         "/my_file.txt",
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc bar def"},
		},
		{
			name:            "empty_file_should_be_ignored",
			visitor:         fooToBarVisitor,
			relPath:         ".",
			initialContents: map[string]string{"my_file.txt": ""},
			want:            map[string]string{"my_file.txt": ""},
		},
		{
			name:            "writefile_should_not_be_called_if_contents_unchanged",
			visitor:         fooToBarVisitor,
			relPath:         ".",
			initialContents: map[string]string{"my_file.txt": "abc"},
			want:            map[string]string{"my_file.txt": "abc"},
			writeFileErr:    fmt.Errorf("WriteFile should not have been called"),
		},
		{
			name:            "stat_error_should_be_returned",
			visitor:         fooToBarVisitor,
			relPath:         ".",
			initialContents: map[string]string{"my_file.txt": "foo"},
			want:            map[string]string{"my_file.txt": "foo"},
			statErr:         fmt.Errorf("fake error for testing"),
			wantErr:         "fake error for testing",
		},
		{
			name:            "readfile_error_should_be_returned",
			visitor:         fooToBarVisitor,
			relPath:         ".",
			initialContents: map[string]string{"my_file.txt": "foo"},
			want:            map[string]string{"my_file.txt": "foo"},
			readFileErr:     fmt.Errorf("fake error for testing"),
			wantErr:         "fake error for testing",
		},
		{
			name:            "writefile_error_should_be_returned",
			visitor:         fooToBarVisitor,
			relPath:         ".",
			initialContents: map[string]string{"my_file.txt": "foo"},
			want:            map[string]string{"my_file.txt": "foo"},
			writeFileErr:    fmt.Errorf("fake error for testing"),
			wantErr:         "fake error for testing",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Convert to OS-specific paths
			abctestutil.KeysToPlatformPaths(tc.initialContents, tc.want)

			scratchDir := t.TempDir()
			abctestutil.WriteAllDefaultMode(t, scratchDir, tc.initialContents)

			fs := &errorFS{
				renderFS: &realFS{},

				readFileErr:  tc.readFileErr,
				statErr:      tc.statErr,
				writeFileErr: tc.writeFileErr,
			}
			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))
			err := walkAndModify(ctx, nil, fs, scratchDir, tc.relPath, tc.visitor)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			got := abctestutil.LoadDirWithoutMode(t, scratchDir)
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("scratch directory contents were not as expected (-got,+want): %v", diff)
			}
		})
	}
}

func TestCopyRecursive(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                  string
		srcDirContents        map[string]abctestutil.ModeAndContents
		suffix                string
		dryRun                bool
		visitor               copyVisitor
		want                  map[string]abctestutil.ModeAndContents
		wantBackups           map[string]abctestutil.ModeAndContents
		dstDirInitialContents map[string]abctestutil.ModeAndContents // only used in the tests for overwriting
		mkdirAllErr           error
		openErr               error
		openFileErr           error
		readFileErr           error
		statErr               error
		writeFileErr          error
		wantErr               string
	}{
		{
			name: "simple_success",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"file1.txt":      {Mode: 0o600, Contents: "file1 contents"},
				"dir1/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
			want: map[string]abctestutil.ModeAndContents{
				"file1.txt":      {Mode: 0o600, Contents: "file1 contents"},
				"dir1/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
		},
		{
			name: "dry_run_should_not_change_anything",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"file1.txt":      {Mode: 0o600, Contents: "file1 contents"},
				"dir1/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
			dryRun:      true,
			openFileErr: fmt.Errorf("OpenFile shouldn't be called in dry run mode"),
			mkdirAllErr: fmt.Errorf("MkdirAll shouldn't be called in dry run mode"),
		},
		{
			name: "dry_run_with_overwrite_doesnt_make_backups",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "new contents"},
			},
			dstDirInitialContents: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "old contents"},
			},
			visitor: func(relPath string, de fs.DirEntry) (copyHint, error) {
				return copyHint{
					backupIfExists: true,
					overwrite:      true,
				}, nil
			},
			want: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "old contents"},
			},

			dryRun: true,
		},
		{
			name: "dry_run_without_overwrite_should_detect_conflicting_files",
			dstDirInitialContents: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "old contents"},
			},
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"file1.txt":      {Mode: 0o600, Contents: "new contents"},
				"dir1/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
			want: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "old contents"},
			},
			dryRun: true,
			visitor: func(relPath string, de fs.DirEntry) (copyHint, error) {
				return copyHint{
					overwrite: false,
				}, nil
			},
			openFileErr: fmt.Errorf("OpenFile shouldn't be called in dry run mode"),
			mkdirAllErr: fmt.Errorf("MkdirAll shouldn't be called in dry run mode"),
			wantErr:     "file file1.txt already exists and overwriting was not enabled",
		},
		{
			name: "owner_execute_bit_should_be_preserved",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"myfile1.txt": {Mode: 0o600, Contents: "my file contents"},
				"myfile2.txt": {Mode: 0o700, Contents: "my file contents"},
			},
			want: map[string]abctestutil.ModeAndContents{
				"myfile1.txt": {Mode: 0o600, Contents: "my file contents"},
				"myfile2.txt": {Mode: 0o700, Contents: "my file contents"},
			},
		},
		{
			name:   "copying_a_file_rather_than_directory_should_work",
			suffix: "myfile1.txt",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"myfile1.txt": {Mode: 0o600, Contents: "my file contents"},
			},
			want: map[string]abctestutil.ModeAndContents{
				"myfile1.txt": {Mode: 0o600, Contents: "my file contents"},
			},
		},
		{
			name: "deep_directories_should_work",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"dir/dir/dir/dir/dir/file.txt": {Mode: 0o600, Contents: "file contents"},
			},
			want: map[string]abctestutil.ModeAndContents{
				"dir/dir/dir/dir/dir/file.txt": {Mode: 0o600, Contents: "file contents"},
			},
		},
		{
			name: "directories_with_several_files_should_work",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"f1.txt": {Mode: 0o600, Contents: "abc"},
				"f2.txt": {Mode: 0o600, Contents: "def"},
				"f3.txt": {Mode: 0o600, Contents: "ghi"},
				"f4.txt": {Mode: 0o600, Contents: "jkl"},
				"f5.txt": {Mode: 0o600, Contents: "mno"},
				"f6.txt": {Mode: 0o600, Contents: "pqr"},
				"f7.txt": {Mode: 0o600, Contents: "stu"},
				"f8.txt": {Mode: 0o600, Contents: "vwx"},
				"f9.txt": {Mode: 0o600, Contents: "yz"},
			},
			want: map[string]abctestutil.ModeAndContents{
				"f1.txt": {Mode: 0o600, Contents: "abc"},
				"f2.txt": {Mode: 0o600, Contents: "def"},
				"f3.txt": {Mode: 0o600, Contents: "ghi"},
				"f4.txt": {Mode: 0o600, Contents: "jkl"},
				"f5.txt": {Mode: 0o600, Contents: "mno"},
				"f6.txt": {Mode: 0o600, Contents: "pqr"},
				"f7.txt": {Mode: 0o600, Contents: "stu"},
				"f8.txt": {Mode: 0o600, Contents: "vwx"},
				"f9.txt": {Mode: 0o600, Contents: "yz"},
			},
		},
		{
			name: "overwriting_with_overwrite_true_should_succeed",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "new contents"},
			},
			dstDirInitialContents: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "old contents"},
			},
			visitor: func(relPath string, de fs.DirEntry) (copyHint, error) {
				return copyHint{
					overwrite: true,
				}, nil
			},
			want: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "new contents"},
			},
		},
		{
			name: "overwriting_with_overwrite_false_should_fail",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "new contents"},
			},
			dstDirInitialContents: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "old contents"},
			},
			want: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "old contents"},
			},
			wantErr: "overwriting was not enabled",
		},
		{
			name: "overwriting_dir_with_child_file_should_fail",
			visitor: func(relPath string, de fs.DirEntry) (copyHint, error) {
				return copyHint{
					overwrite: true,
				}, nil
			},
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"a": {Mode: 0o600, Contents: "file contents"},
			},
			dstDirInitialContents: map[string]abctestutil.ModeAndContents{
				"a/b.txt": {Mode: 0o600, Contents: "file contents"},
			},
			want: map[string]abctestutil.ModeAndContents{
				"a/b.txt": {Mode: 0o600, Contents: "file contents"},
			},
			wantErr: "cannot overwrite a directory with a file of the same name",
		},
		{
			name: "overwriting_file_with_dir_should_fail",
			visitor: func(relPath string, de fs.DirEntry) (copyHint, error) {
				return copyHint{
					overwrite: true,
				}, nil
			},
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"a/b.txt": {Mode: 0o600, Contents: "file contents"},
			},
			dstDirInitialContents: map[string]abctestutil.ModeAndContents{
				"a": {Mode: 0o600, Contents: "file contents"},
			},
			want: map[string]abctestutil.ModeAndContents{
				"a": {Mode: 0o600, Contents: "file contents"},
			},
			wantErr: "cannot overwrite a file with a directory of the same name",
		},
		{
			name: "skipped_files",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"file1.txt":        {Mode: 0o600, Contents: "file1 contents"},
				"dir1/file2.txt":   {Mode: 0o600, Contents: "file2 contents"},
				"skip1.txt":        {Mode: 0o600, Contents: "skip1.txt contents"},
				"subdir/skip2.txt": {Mode: 0o600, Contents: "skip2.txt contents"},
			},
			visitor: func(relPath string, de fs.DirEntry) (copyHint, error) {
				return copyHint{
					skip: slices.Contains([]string{"skip1.txt", "subdir/skip2.txt"}, relPath),
				}, nil
			},
			want: map[string]abctestutil.ModeAndContents{
				"file1.txt":      {Mode: 0o600, Contents: "file1 contents"},
				"dir1/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
		},
		{
			name: "skipped_directory_skips_all_subsequent",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"file1.txt":          {Mode: 0o600, Contents: "file1 contents"},
				"subdir/file2.txt":   {Mode: 0o600, Contents: "file2 contents"},
				"subdir/file3.txt":   {Mode: 0o600, Contents: "file3 contents"},
				"otherdir/file4.txt": {Mode: 0o600, Contents: "file4 contents"},
			},
			visitor: func(relPath string, de fs.DirEntry) (copyHint, error) {
				if relPath == "subdir" {
					return copyHint{
						skip: true,
					}, nil
				}
				if strings.HasPrefix(relPath, "subdir/") {
					panic("no children of subdir/ should have been walked")
				}
				return copyHint{}, nil
			},
			want: map[string]abctestutil.ModeAndContents{
				"file1.txt":          {Mode: 0o600, Contents: "file1 contents"},
				"otherdir/file4.txt": {Mode: 0o600, Contents: "file4 contents"},
			},
		},
		{
			name: "backup_existing",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"file1.txt":        {Mode: 0o600, Contents: "file1 new contents"},
				"subdir/file2.txt": {Mode: 0o600, Contents: "file2 new contents"},
			},
			dstDirInitialContents: map[string]abctestutil.ModeAndContents{
				"file1.txt":        {Mode: 0o600, Contents: "file1 old contents"},
				"subdir/file2.txt": {Mode: 0o600, Contents: "file2 old contents"},
			},
			visitor: func(relPath string, de fs.DirEntry) (copyHint, error) {
				return copyHint{
					backupIfExists: true,
					overwrite:      true,
				}, nil
			},
			want: map[string]abctestutil.ModeAndContents{
				"file1.txt":        {Mode: 0o600, Contents: "file1 new contents"},
				"subdir/file2.txt": {Mode: 0o600, Contents: "file2 new contents"},
			},
			wantBackups: map[string]abctestutil.ModeAndContents{
				"file1.txt":        {Mode: 0o600, Contents: "file1 old contents"},
				"subdir/file2.txt": {Mode: 0o600, Contents: "file2 old contents"},
			},
		},
		{
			name: "MkdirAll error should be returned",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"dir/file.txt": {Mode: 0o600, Contents: "file1 contents"},
			},
			mkdirAllErr: fmt.Errorf("fake error"),
			wantErr:     "MkdirAll(): fake error",
		},
		{
			name: "Open error should be returned",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"dir/file.txt": {Mode: 0o600, Contents: "file1 contents"},
			},
			openErr: fmt.Errorf("fake error"),
			wantErr: "fake error", // This error comes from WalkDir, not from our own code, so it doesn't have an "Open():" at the beginning
		},
		{
			name: "OpenFile error should be returned",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"dir/file.txt": {Mode: 0o600, Contents: "file1 contents"},
			},
			openFileErr: fmt.Errorf("fake error"),
			wantErr:     "OpenFile(): fake error",
		},
		{
			name: "Stat error should be returned",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"dir/file.txt": {Mode: 0o600, Contents: "file1 contents"},
			},
			statErr: fmt.Errorf("fake error"),
			wantErr: "fake error", // This error comes from WalkDir, not from our own code, so it doesn't have a "Stat():" at the beginning
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			fromDir := filepath.Join(tempDir, "from_dir")
			toDir := filepath.Join(tempDir, "to_dir")
			backupDir := filepath.Join(tempDir, "backups")

			// Convert to OS-specific paths
			abctestutil.KeysToPlatformPaths(
				tc.srcDirContents,
				tc.dstDirInitialContents,
				tc.want,
				tc.wantBackups,
			)

			abctestutil.WriteAll(t, fromDir, tc.srcDirContents)

			from := fromDir
			to := toDir
			if tc.suffix != "" {
				from = filepath.Join(fromDir, tc.suffix)
				to = filepath.Join(toDir, tc.suffix)
			}
			abctestutil.WriteAll(t, toDir, tc.dstDirInitialContents)
			fs := &errorFS{
				renderFS: &realFS{},

				mkdirAllErr: tc.mkdirAllErr,
				openErr:     tc.openErr,
				openFileErr: tc.openFileErr,
				statErr:     tc.statErr,
			}
			ctx := context.Background()

			clk := clock.NewMock()
			const unixTime = 1688609125
			clk.Set(time.Unix(unixTime, 0)) // Arbitrary timestamp

			backupDirMaker := func(rf renderFS) (string, error) {
				if err := os.MkdirAll(backupDir, 0o755); err != nil {
					return "", err
				}
				return backupDir, nil
			}

			err := copyRecursive(ctx, &model.ConfigPos{}, &copyParams{
				backupDirMaker: backupDirMaker,
				srcRoot:        from,
				dstRoot:        to,
				dryRun:         tc.dryRun,
				rfs:            fs,
				visitor:        tc.visitor,
			})
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			got := abctestutil.LoadDirContents(t, toDir)
			if diff := cmp.Diff(got, tc.want, cmpFileMode, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("destination directory was not as expected (-got,+want): %s", diff)
			}

			gotBackups := abctestutil.LoadDirContents(t, backupDir)
			if diff := cmp.Diff(gotBackups, tc.wantBackups, cmpFileMode, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("backups directory was not as expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestParseAndExecute(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		pos               *model.ConfigPos
		tmpl              string
		inputs            map[string]string
		want              string
		wantUnknownKeyErr bool
		wantErr           string
	}{
		{
			name: "simple_success",
			pos: &model.ConfigPos{
				Line: 1,
			},
			tmpl: "{{.greeting}}, {{.greeted_entity}}!",
			inputs: map[string]string{
				"greeting":       "Hello",
				"greeted_entity": "world",
			},
			want: "Hello, world!",
		},
		{
			name: "missing_input",
			pos: &model.ConfigPos{
				Line: 1,
			},
			tmpl: "{{.my_input}}!",
			inputs: map[string]string{
				"something_else": "🥲",
			},
			wantUnknownKeyErr: true,
			wantErr:           `failed executing template spec file at line 1: template.Execute() failed: the template referenced a nonexistent input variable name "my_input"; available variable names are [something_else]`,
		},
		{
			name: "unclosed_braces",
			tmpl: "Hello {{",
			inputs: map[string]string{
				"something_else": "🥲",
			},
			wantErr: `unclosed action`,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseAndExecuteGoTmpl(tc.pos, tc.tmpl, tc.inputs)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}
			if tc.wantUnknownKeyErr {
				as := &unknownTemplateKeyError{}
				if ok := errors.As(err, &as); !ok {
					t.Errorf("errors.As(%T)=false, wanted true, for error %v", &as, err)
				}
			}

			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("template output was not as expected, (-got,+want): %s", diff)
			}
		})
	}
}

func TestUnknownTemplateKeyError_ErrorsIsAs(t *testing.T) {
	t.Parallel()

	err1 := &unknownTemplateKeyError{
		key:           "my_key",
		availableKeys: []string{"other_key"},
		wrapped:       errors.New("wrapped"),
	}

	is := &unknownTemplateKeyError{}
	if !errors.Is(err1, is) {
		t.Errorf("errors.Is() returned false, should return true when called with an error of type %T", is)
	}

	as := &unknownTemplateKeyError{}
	if !errors.As(err1, &as) {
		t.Errorf("errors.As() returned false, should return true when called with an error of type %T", as)
	}
}
