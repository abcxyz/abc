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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/model"
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

	fooToFooFooVisitor := func(buf []byte) ([]byte, error) {
		return bytes.ReplaceAll(buf, []byte("foo"), []byte("foofoo")), nil
	}

	cases := []struct {
		name            string
		visitor         walkAndModifyVisitor
		relPaths        []string
		initialContents map[string]string
		want            map[string]string
		wantErr         string

		// fakeable errors
		readFileErr  error
		writeFileErr error
	}{
		{
			name:            "simple_single_file_replacement_should_work",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"my_file.txt"},
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc bar def"},
		},
		{
			name:            "repeated_file_only_visited_once",
			visitor:         fooToFooFooVisitor,
			relPaths:        []string{"my_file.txt", "my_file.txt"},
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc foofoo def"},
		},
		{
			name:            "repeated_file_directory_only_visited_once",
			visitor:         fooToFooFooVisitor,
			relPaths:        []string{"my_file.txt", ".", "./my_file.txt", "./", "/"},
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc foofoo def"},
		},
		{
			name:            "multiple_replacements_should_work",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"my_file.txt"},
			initialContents: map[string]string{"my_file.txt": "foo foo"}, //nolint:dupword
			want:            map[string]string{"my_file.txt": "bar bar"}, //nolint:dupword
		},
		{
			name:     "multiple_replacements_multiple_paths_should_work",
			visitor:  fooToBarVisitor,
			relPaths: []string{"my_file.txt", "b/"},
			initialContents: map[string]string{
				"my_file.txt":   "foo foo", //nolint:dupword
				"b/my_file.txt": "foo foo", //nolint:dupword
			},
			want: map[string]string{"my_file.txt": "bar bar", "b/my_file.txt": "bar bar"}, //nolint:dupword
		},
		{
			name:     "dot_dir_should_work",
			visitor:  fooToBarVisitor,
			relPaths: []string{"."},
			initialContents: map[string]string{
				"my_file.txt":       "abc foo def",
				"my_other_file.txt": "abc foo fed",
			},
			want: map[string]string{
				"my_file.txt":       "abc bar def",
				"my_other_file.txt": "abc bar fed",
			},
		},
		{
			name:            "empty_path_means_root_should_work",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"."},
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc bar def"},
		},
		{
			name:            "dot_dir_with_trailing_slash_should_work",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"./"},
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc bar def"},
		},
		{
			name:            "single_subdir_should_work",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"./dir"},
			initialContents: map[string]string{"dir/my_file.txt": "abc foo def"},
			want:            map[string]string{"dir/my_file.txt": "abc bar def"},
		},
		{
			name:            "named_file_in_subdir_should_work",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"dir/my_file.txt"},
			initialContents: map[string]string{"dir/my_file.txt": "abc foo def"},
			want:            map[string]string{"dir/my_file.txt": "abc bar def"},
		},
		{
			name:            "deeply_nested_dirs_should_work",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"dir1"},
			initialContents: map[string]string{"dir1/dir2/dir3/dir4/my_file.txt": "abc foo def"},
			want:            map[string]string{"dir1/dir2/dir3/dir4/my_file.txt": "abc bar def"},
		},
		{
			name:     "one_included_dir_one_excluded",
			visitor:  fooToBarVisitor,
			relPaths: []string{"dir1"},
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
			relPaths:        []string{"nonexistent"},
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc foo def"},
			wantErr:         `glob "nonexistent" did not match any files`,
		},
		{
			name:            "absolute_path_should_become_relative",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"/my_file.txt"},
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc bar def"},
		},
		{
			name:            "empty_file_should_be_ignored",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"."},
			initialContents: map[string]string{"my_file.txt": ""},
			want:            map[string]string{"my_file.txt": ""},
		},
		{
			name:            "writefile_should_not_be_called_if_contents_unchanged",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"."},
			initialContents: map[string]string{"my_file.txt": "abc"},
			want:            map[string]string{"my_file.txt": "abc"},
			writeFileErr:    fmt.Errorf("WriteFile should not have been called"),
		},
		{
			name:            "readfile_error_should_be_returned",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"."},
			initialContents: map[string]string{"my_file.txt": "foo"},
			want:            map[string]string{"my_file.txt": "foo"},
			readFileErr:     fmt.Errorf("fake error for testing"),
			wantErr:         "fake error for testing",
		},
		{
			name:            "writefile_error_should_be_returned",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"."},
			initialContents: map[string]string{"my_file.txt": "foo"},
			want:            map[string]string{"my_file.txt": "foo"},
			writeFileErr:    fmt.Errorf("fake error for testing"),
			wantErr:         "fake error for testing",
		},
		{
			name:            "simple_glob_path_should_work",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"*.txt"},
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc bar def"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Convert to OS-specific paths
			convertKeysToPlatformPaths(tc.initialContents, tc.want)

			scratchDir := t.TempDir()
			if err := common.WriteAllDefaultMode(scratchDir, tc.initialContents); err != nil {
				t.Fatal(err)
			}

			sp := &stepParams{
				fs: &errorFS{
					renderFS: &realFS{},

					readFileErr:  tc.readFileErr,
					writeFileErr: tc.writeFileErr,
				},
				scratchDir: scratchDir,
				scope:      common.NewScope(nil),
			}

			relPathsPositions := make([]model.String, 0, len(tc.relPaths))

			for _, p := range tc.relPaths {
				relPathsPositions = append(relPathsPositions, model.String{Val: p})
			}

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))
			err := walkAndModify(ctx, sp, relPathsPositions, tc.visitor)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			got := common.LoadDirWithoutMode(t, scratchDir)
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
		srcDirContents        map[string]common.ModeAndContents
		suffix                string
		dryRun                bool
		visitor               copyVisitor
		want                  map[string]common.ModeAndContents
		wantBackups           map[string]common.ModeAndContents
		dstDirInitialContents map[string]common.ModeAndContents // only used in the tests for overwriting and backing up
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
			srcDirContents: map[string]common.ModeAndContents{
				"file1.txt":      {Mode: 0o600, Contents: "file1 contents"},
				"dir1/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
			want: map[string]common.ModeAndContents{
				"file1.txt":      {Mode: 0o600, Contents: "file1 contents"},
				"dir1/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
		},
		{
			name: "dry_run_should_not_change_anything",
			srcDirContents: map[string]common.ModeAndContents{
				"file1.txt":      {Mode: 0o600, Contents: "file1 contents"},
				"dir1/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
			dryRun:      true,
			openFileErr: fmt.Errorf("OpenFile shouldn't be called in dry run mode"),
			mkdirAllErr: fmt.Errorf("MkdirAll shouldn't be called in dry run mode"),
		},
		{
			name: "dry_run_with_overwrite_doesnt_make_backups",
			srcDirContents: map[string]common.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "new contents"},
			},
			dstDirInitialContents: map[string]common.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "old contents"},
			},
			visitor: func(relPath string, de fs.DirEntry) (copyHint, error) {
				return copyHint{
					backupIfExists: true,
					overwrite:      true,
				}, nil
			},
			want: map[string]common.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "old contents"},
			},

			dryRun: true,
		},
		{
			name: "dry_run_without_overwrite_should_detect_conflicting_files",
			dstDirInitialContents: map[string]common.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "old contents"},
			},
			srcDirContents: map[string]common.ModeAndContents{
				"file1.txt":      {Mode: 0o600, Contents: "new contents"},
				"dir1/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
			want: map[string]common.ModeAndContents{
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
			srcDirContents: map[string]common.ModeAndContents{
				"myfile1.txt": {Mode: 0o600, Contents: "my file contents"},
				"myfile2.txt": {Mode: 0o700, Contents: "my file contents"},
			},
			want: map[string]common.ModeAndContents{
				"myfile1.txt": {Mode: 0o600, Contents: "my file contents"},
				"myfile2.txt": {Mode: 0o700, Contents: "my file contents"},
			},
		},
		{
			name:   "copying_a_file_rather_than_directory_should_work",
			suffix: "myfile1.txt",
			srcDirContents: map[string]common.ModeAndContents{
				"myfile1.txt": {Mode: 0o600, Contents: "my file contents"},
			},
			want: map[string]common.ModeAndContents{
				"myfile1.txt": {Mode: 0o600, Contents: "my file contents"},
			},
		},
		{
			name: "deep_directories_should_work",
			srcDirContents: map[string]common.ModeAndContents{
				"dir/dir/dir/dir/dir/file.txt": {Mode: 0o600, Contents: "file contents"},
			},
			want: map[string]common.ModeAndContents{
				"dir/dir/dir/dir/dir/file.txt": {Mode: 0o600, Contents: "file contents"},
			},
		},
		{
			name: "directories_with_several_files_should_work",
			srcDirContents: map[string]common.ModeAndContents{
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
			want: map[string]common.ModeAndContents{
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
			srcDirContents: map[string]common.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "new contents"},
			},
			dstDirInitialContents: map[string]common.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "old contents"},
			},
			visitor: func(relPath string, de fs.DirEntry) (copyHint, error) {
				return copyHint{
					overwrite: true,
				}, nil
			},
			want: map[string]common.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "new contents"},
			},
		},
		{
			name: "overwriting_with_overwrite_false_should_fail",
			srcDirContents: map[string]common.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "new contents"},
			},
			dstDirInitialContents: map[string]common.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "old contents"},
			},
			want: map[string]common.ModeAndContents{
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
			srcDirContents: map[string]common.ModeAndContents{
				"a": {Mode: 0o600, Contents: "file contents"},
			},
			dstDirInitialContents: map[string]common.ModeAndContents{
				"a/b.txt": {Mode: 0o600, Contents: "file contents"},
			},
			want: map[string]common.ModeAndContents{
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
			srcDirContents: map[string]common.ModeAndContents{
				"a/b.txt": {Mode: 0o600, Contents: "file contents"},
			},
			dstDirInitialContents: map[string]common.ModeAndContents{
				"a": {Mode: 0o600, Contents: "file contents"},
			},
			want: map[string]common.ModeAndContents{
				"a": {Mode: 0o600, Contents: "file contents"},
			},
			wantErr: "cannot overwrite a file with a directory of the same name",
		},
		{
			name: "skipped_files",
			srcDirContents: map[string]common.ModeAndContents{
				"file1.txt":        {Mode: 0o600, Contents: "file1 contents"},
				"dir1/file2.txt":   {Mode: 0o600, Contents: "file2 contents"},
				"skip1.txt":        {Mode: 0o600, Contents: "skip1.txt contents"},
				"subdir/skip2.txt": {Mode: 0o600, Contents: "skip2.txt contents"},
				"z.txt":            {Mode: 0o600, Contents: "z.txt contents"},
			},
			visitor: func(relPath string, de fs.DirEntry) (copyHint, error) {
				return copyHint{
					skip: slices.Contains([]string{"skip1.txt", filepath.Join("subdir", "skip2.txt")}, relPath),
				}, nil
			},
			want: map[string]common.ModeAndContents{
				"file1.txt":      {Mode: 0o600, Contents: "file1 contents"},
				"dir1/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
				"z.txt":          {Mode: 0o600, Contents: "z.txt contents"},
			},
		},
		{
			name: "skipped_directory_skips_all_subsequent",
			srcDirContents: map[string]common.ModeAndContents{
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
			want: map[string]common.ModeAndContents{
				"file1.txt":          {Mode: 0o600, Contents: "file1 contents"},
				"otherdir/file4.txt": {Mode: 0o600, Contents: "file4 contents"},
			},
		},
		{
			name: "backup_existing",
			srcDirContents: map[string]common.ModeAndContents{
				"file1.txt":        {Mode: 0o600, Contents: "file1 new contents"},
				"subdir/file2.txt": {Mode: 0o600, Contents: "file2 new contents"},
			},
			dstDirInitialContents: map[string]common.ModeAndContents{
				"file1.txt":        {Mode: 0o600, Contents: "file1 old contents"},
				"subdir/file2.txt": {Mode: 0o600, Contents: "file2 old contents"},
			},
			visitor: func(relPath string, de fs.DirEntry) (copyHint, error) {
				return copyHint{
					backupIfExists: true,
					overwrite:      true,
				}, nil
			},
			want: map[string]common.ModeAndContents{
				"file1.txt":        {Mode: 0o600, Contents: "file1 new contents"},
				"subdir/file2.txt": {Mode: 0o600, Contents: "file2 new contents"},
			},
			wantBackups: map[string]common.ModeAndContents{
				"file1.txt":        {Mode: 0o600, Contents: "file1 old contents"},
				"subdir/file2.txt": {Mode: 0o600, Contents: "file2 old contents"},
			},
		},
		{
			name: "MkdirAll error should be returned",
			srcDirContents: map[string]common.ModeAndContents{
				"dir/file.txt": {Mode: 0o600, Contents: "file1 contents"},
			},
			mkdirAllErr: fmt.Errorf("fake error"),
			wantErr:     "MkdirAll(): fake error",
		},
		{
			name: "Open error should be returned",
			srcDirContents: map[string]common.ModeAndContents{
				"dir/file.txt": {Mode: 0o600, Contents: "file1 contents"},
			},
			openErr: fmt.Errorf("fake error"),
			wantErr: "fake error", // This error comes from WalkDir, not from our own code, so it doesn't have an "Open():" at the beginning
		},
		{
			name: "OpenFile error should be returned",
			srcDirContents: map[string]common.ModeAndContents{
				"dir/file.txt": {Mode: 0o600, Contents: "file1 contents"},
			},
			openFileErr: fmt.Errorf("fake error"),
			wantErr:     "OpenFile(): fake error",
		},
		{
			name: "Stat error should be returned",
			srcDirContents: map[string]common.ModeAndContents{
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
			convertKeysToPlatformPaths(
				tc.srcDirContents,
				tc.dstDirInitialContents,
				tc.want,
				tc.wantBackups,
			)

			if err := common.WriteAll(fromDir, tc.srcDirContents); err != nil {
				t.Fatal(err)
			}

			from := fromDir
			to := toDir
			if tc.suffix != "" {
				from = filepath.Join(fromDir, tc.suffix)
				to = filepath.Join(toDir, tc.suffix)
			}
			if err := common.WriteAll(toDir, tc.dstDirInitialContents); err != nil {
				t.Fatal(err)
			}
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

			err := copyRecursive(ctx, &model.ConfigPos{}, &copyParams{
				backupDirMaker: func(rf renderFS) (string, error) { return backupDir, nil },
				srcRoot:        from,
				dstRoot:        to,
				dryRun:         tc.dryRun,
				rfs:            fs,
				visitor:        tc.visitor,
			})
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			got := common.LoadDirContents(t, toDir)
			if diff := cmp.Diff(got, tc.want, common.CmpFileMode, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("destination directory was not as expected (-got,+want): %s", diff)
			}

			gotBackups := common.LoadDirContents(t, backupDir)
			if diff := cmp.Diff(gotBackups, tc.wantBackups, common.CmpFileMode, cmpopts.EquateEmpty()); diff != "" {
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
			wantErr:           `at line 1 column 0: template.Execute() failed: the template referenced a nonexistent input variable name "my_input"; available variable names are [something_else]`,
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

			got, err := parseAndExecuteGoTmpl(tc.pos, tc.tmpl, common.NewScope(tc.inputs))
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

// These are basic tests to ensure the template functions are mounted. More
// exhaustive tests are at template_funcs_test.go.
func TestTemplateFuncs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		tmpl    string
		inputs  map[string]string
		want    string
		wantErr string
	}{
		{
			name: "contains_true",
			tmpl: `{{ contains "food" "foo" }}`,
			want: "true",
		},
		{
			name: "contains_false",
			tmpl: `{{ contains "food" "bar" }}`,
			want: "false",
		},
		{
			name: "replace",
			tmpl: `{{ replace "food" "foo" "bar" 1 }}`,
			want: "bard",
		},
		{
			name: "replaceAll",
			tmpl: `{{ replaceAll "food food food" "foo" "bar" }}`,
			want: "bard bard bard", //nolint:dupword // expected
		},
		{
			name: "sortStrings",
			tmpl: `{{ split "zebra,car,foo" "," | sortStrings }}`,
			want: "[car foo zebra]",
		},
		{
			name: "split",
			tmpl: `{{ split "a,b,c" "," }}`,
			want: "[a b c]",
		},
		{
			name: "toLower",
			tmpl: `{{ toLower "AbCD" }}`,
			want: "abcd",
		},
		{
			name: "toUpper",
			tmpl: `{{ toUpper "AbCD" }}`,
			want: "ABCD",
		},
		{
			name: "trimPrefix",
			tmpl: `{{ trimPrefix "foobarbaz" "foo" }}`,
			want: "barbaz",
		},
		{
			name: "trimSuffix",
			tmpl: `{{ trimSuffix "foobarbaz" "baz" }}`,
			want: "foobar",
		},
		{
			name: "toSnakeCase",
			tmpl: `{{ toSnakeCase "foo-bar-baz" }}`,
			want: "foo_bar_baz",
		},
		{
			name: "toLowerSnakeCase",
			tmpl: `{{ toLowerSnakeCase "foo-bar-baz" }}`,
			want: "foo_bar_baz",
		},
		{
			name: "toUpperSnakeCase",
			tmpl: `{{ toUpperSnakeCase "foo-bar-baz" }}`,
			want: "FOO_BAR_BAZ",
		},
		{
			name: "toHyphenCase",
			tmpl: `{{ toHyphenCase "foo_bar_baz" }}`,
			want: "foo-bar-baz",
		},
		{
			name: "toLowerHyphenCase",
			tmpl: `{{ toLowerHyphenCase "foo_bar_baz" }}`,
			want: "foo-bar-baz",
		},
		{
			name: "toUpperHyphenCase",
			tmpl: `{{ toUpperHyphenCase "foo-bar-baz" }}`,
			want: "FOO-BAR-BAZ",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pos := &model.ConfigPos{
				Line: 1,
			}

			got, err := parseAndExecuteGoTmpl(pos, tc.tmpl, common.NewScope(map[string]string{}))
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("template output was not as expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestProcessPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		paths     []model.String
		scope     *common.Scope
		wantPaths []model.String
		wantErr   string
	}{
		{
			name:      "verify_paths_unchanged",
			paths:     modelStrings([]string{"file1.txt", "file2.txt", "subfolder1", "subfolder2/file3.txt"}),
			scope:     common.NewScope(map[string]string{}),
			wantPaths: modelStrings([]string{"file1.txt", "file2.txt", "subfolder1", filepath.FromSlash("subfolder2/file3.txt")}),
		},
		{
			name:  "go_template_in_path",
			paths: modelStrings([]string{"{{.replace_name}}.txt"}),
			scope: common.NewScope(map[string]string{
				"replace_name": "file1",
			}),
			wantPaths: modelStrings([]string{"file1.txt"}),
		},
		{
			name:    "fail_dot_dot_relative_path",
			paths:   modelStrings([]string{"../foo.txt"}),
			scope:   common.NewScope(map[string]string{}),
			wantErr: fmt.Sprintf(`path %q must not contain ".."`, filepath.FromSlash("../foo.txt")),
		},
		{
			name: "no_escaping_glob_paths",
			paths: modelStrings([]string{
				`file\1.txt`,
			}),
			scope:   common.NewScope(map[string]string{}),
			wantErr: fmt.Sprintf(`backslashes in glob paths are not permitted: %q`, `file\1.txt`),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pathsCopy := make([]model.String, 0, len(tc.paths))

			for _, p := range tc.paths {
				pathsCopy = append(pathsCopy, model.String{
					Val: p.Val,
					Pos: p.Pos,
				})
			}
			gotPaths, err := processPaths(tc.paths, tc.scope)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}
			if diff := cmp.Diff(tc.paths, pathsCopy); diff != "" {
				t.Errorf("input paths for action should not have been changed (-got,+want): %s", diff)
			}
			if diff := cmp.Diff(gotPaths, tc.wantPaths); diff != "" {
				t.Errorf("resulting paths should match expected paths from input (-got,+want): %s", diff)
			}
		})
	}
}

func TestProcessGlobs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		dirContents map[string]common.ModeAndContents
		paths       []model.String
		wantPaths   []model.String
		wantErr     string
	}{
		{
			name: "non_glob_paths",
			dirContents: map[string]common.ModeAndContents{
				"file1.txt":            {Mode: 0o600, Contents: "file1 contents"},
				"file2.txt":            {Mode: 0o600, Contents: "file2 contents"},
				"subfolder1/file3.txt": {Mode: 0o600, Contents: "file3 contents"},
				"subfolder2/file4.txt": {Mode: 0o600, Contents: "file4 contents"},
				"subfolder2/file5.txt": {Mode: 0o600, Contents: "file5 contents"},
			},
			paths: modelStrings([]string{
				"file1.txt",
				"file2.txt",
				"subfolder1",
				"subfolder2/file4.txt",
			}),
			wantPaths: modelStrings([]string{
				"file1.txt",
				"file2.txt",
				"subfolder1",
				filepath.FromSlash("subfolder2/file4.txt"),
			}),
		},
		{
			name: "star_glob_paths",
			dirContents: map[string]common.ModeAndContents{
				"file1.txt":            {Mode: 0o600, Contents: "file1 contents"},
				"file2.txt":            {Mode: 0o600, Contents: "file2 contents"},
				"subfolder1/file3.txt": {Mode: 0o600, Contents: "file3 contents"},
				"subfolder2/file4.txt": {Mode: 0o600, Contents: "file4 contents"},
				"subfolder2/file5.txt": {Mode: 0o600, Contents: "file5 contents"},
			},
			paths: modelStrings([]string{
				"*.txt",
				"subfolder2/*.txt",
			}),
			wantPaths: modelStrings([]string{
				"file1.txt",
				"file2.txt",
				filepath.FromSlash("subfolder2/file4.txt"),
				filepath.FromSlash("subfolder2/file5.txt"),
			}),
		},
		{
			name: "star_in_middle",
			dirContents: map[string]common.ModeAndContents{
				"file1.txt":            {Mode: 0o600, Contents: "file1 contents"},
				"file2.txt":            {Mode: 0o600, Contents: "file2 contents"},
				"subfolder1/file3.txt": {Mode: 0o600, Contents: "file3 contents"},
				"subfolder2/file4.txt": {Mode: 0o600, Contents: "file4 contents"},
				"subfolder2/file5.txt": {Mode: 0o600, Contents: "file5 contents"},
			},
			paths: modelStrings([]string{
				"f*e1.txt",
				"f*e2.txt",
				"sub*er2",
			}),
			wantPaths: modelStrings([]string{
				"file1.txt",
				"file2.txt",
				"subfolder2",
			}),
		},
		{
			name: "star_all_paths",
			dirContents: map[string]common.ModeAndContents{
				"file1.txt":            {Mode: 0o600, Contents: "file1 contents"},
				"file2.txt":            {Mode: 0o600, Contents: "file2 contents"},
				"subfolder1/file3.txt": {Mode: 0o600, Contents: "file3 contents"},
				"subfolder2/file4.txt": {Mode: 0o600, Contents: "file4 contents"},
				"subfolder2/file5.txt": {Mode: 0o600, Contents: "file5 contents"},
			},
			paths: modelStrings([]string{"*"}),
			wantPaths: modelStrings([]string{
				"file1.txt",
				"file2.txt",
				"subfolder1",
				"subfolder2",
			}),
		},
		{
			name: "star_matches_hidden_files",
			dirContents: map[string]common.ModeAndContents{
				".gitignore": {Mode: 0o600, Contents: ".gitignore contents"},
				".something": {Mode: 0o600, Contents: ".something contents"},
			},
			paths: modelStrings([]string{"*"}),
			wantPaths: modelStrings([]string{
				".gitignore",
				".something",
			}),
		},
		{
			name: "question_glob_paths",
			dirContents: map[string]common.ModeAndContents{
				"file1.txt":            {Mode: 0o600, Contents: "file1 contents"},
				"file2.txt":            {Mode: 0o600, Contents: "file2 contents"},
				"subfolder1/file3.txt": {Mode: 0o600, Contents: "file3 contents"},
				"subfolder2/file4.txt": {Mode: 0o600, Contents: "file4 contents"},
				"subfolder2/file5.txt": {Mode: 0o600, Contents: "file4 contents"},
			},
			paths: modelStrings([]string{
				"file?.txt",
				"subfolder2/file?.txt",
			}),
			wantPaths: modelStrings([]string{
				"file1.txt",
				"file2.txt",
				filepath.FromSlash("subfolder2/file4.txt"),
				filepath.FromSlash("subfolder2/file5.txt"),
			}),
		},
		{
			name: "no_glob_matches",
			paths: modelStrings([]string{
				"file_not_found.txt",
			}),
			wantErr: fmt.Sprintf(`glob %q did not match any files`, "file_not_found.txt"),
		},
		{
			name: "character_range_paths",
			dirContents: map[string]common.ModeAndContents{
				"abc.txt": {Mode: 0o600, Contents: "bcd contents"},
				"xyz.txt": {Mode: 0o600, Contents: "xyz contents"},
			},
			paths: modelStrings([]string{
				"[a-c][a-c][a-c].txt",
			}),
			wantPaths: modelStrings([]string{
				"abc.txt",
			}),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// pre-populate dir contents
			tempDir := t.TempDir()
			convertKeysToPlatformPaths(tc.dirContents) // Convert to OS-specific paths
			if err := common.WriteAll(tempDir, tc.dirContents); err != nil {
				t.Fatal(err)
			}

			ctx := context.Background()
			gotPaths, err := processGlobs(ctx, tc.paths, tempDir)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}
			if err != nil {
				return // err was expected as part of the test
			}
			relGotPaths := make([]model.String, 0, len(gotPaths))
			for _, p := range gotPaths {
				relPath, err := filepath.Rel(tempDir, p.Val)
				if err != nil {
					t.Fatal(err)
				}
				relGotPaths = append(relGotPaths, model.String{
					Val: relPath,
					Pos: p.Pos,
				})
			}
			if diff := cmp.Diff(relGotPaths, tc.wantPaths); diff != "" {
				t.Errorf("resulting paths should match expected paths from input (-got,+want): %s", diff)
			}
		})
	}
}
