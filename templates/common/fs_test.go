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

package common

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/exp/slices"

	"github.com/abcxyz/abc/templates/model"
	abctestutil "github.com/abcxyz/abc/templates/testutil"
	"github.com/abcxyz/pkg/testutil"
)

func TestCopyRecursive(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                  string
		srcDirContents        map[string]abctestutil.ModeAndContents
		suffix                string
		dryRun                bool
		hasher                func() hash.Hash
		visitor               CopyVisitor
		want                  map[string]abctestutil.ModeAndContents
		wantBackups           map[string]abctestutil.ModeAndContents
		dstDirInitialContents map[string]abctestutil.ModeAndContents // only used in the tests for overwriting and backing up
		mkdirAllErr           error
		openErr               error
		openFileErr           error
		readFileErr           error
		statErr               error
		writeFileErr          error
		wantErr               string
		wantHashesHex         map[string]string
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
			name: "dry_run_with_allow_preexisting_doesnt_make_backups",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "new contents"},
			},
			dstDirInitialContents: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "old contents"},
			},
			visitor: func(relPath string, de fs.DirEntry) (CopyHint, error) {
				return CopyHint{
					BackupIfExists:   true,
					AllowPreexisting: true,
				}, nil
			},
			want: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "old contents"},
			},

			dryRun: true,
		},
		{
			name: "dry_run_without_allow_preexisting_should_detect_conflicting_files",
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
			visitor: func(relPath string, de fs.DirEntry) (CopyHint, error) {
				return CopyHint{
					AllowPreexisting: false,
				}, nil
			},
			openFileErr: fmt.Errorf("OpenFile shouldn't be called in dry run mode"),
			mkdirAllErr: fmt.Errorf("MkdirAll shouldn't be called in dry run mode"),
			wantErr:     "file file1.txt already exists and overwriting was not enabled",
		},
		{
			name: "dry_run_should_calculate_hashes",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "file1 contents"},
			},
			hasher: sha256.New,
			dryRun: true,
			wantHashesHex: map[string]string{
				"file1.txt": "226e7cfa701fb8ba542d42e0f8bd3090cbbcc9f54d834f361c0ab8c3f4846b72",
			},
			openFileErr: fmt.Errorf("OpenFile shouldn't be called in dry run mode"),
			mkdirAllErr: fmt.Errorf("MkdirAll shouldn't be called in dry run mode"),
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
			name: "overwriting_with_allow_preexisting_true_should_succeed",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "new contents"},
			},
			dstDirInitialContents: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "old contents"},
			},
			visitor: func(relPath string, de fs.DirEntry) (CopyHint, error) {
				return CopyHint{
					AllowPreexisting: true,
				}, nil
			},
			want: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "new contents"},
			},
		},
		{
			name: "overwriting_with_allow_preexisting_false_should_fail",
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
			visitor: func(relPath string, de fs.DirEntry) (CopyHint, error) {
				return CopyHint{
					AllowPreexisting: true,
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
			visitor: func(relPath string, de fs.DirEntry) (CopyHint, error) {
				return CopyHint{
					AllowPreexisting: true,
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
				"z.txt":            {Mode: 0o600, Contents: "z.txt contents"},
			},
			visitor: func(relPath string, de fs.DirEntry) (CopyHint, error) {
				return CopyHint{
					Skip: slices.Contains([]string{"skip1.txt", filepath.Join("subdir", "skip2.txt")}, relPath),
				}, nil
			},
			want: map[string]abctestutil.ModeAndContents{
				"file1.txt":      {Mode: 0o600, Contents: "file1 contents"},
				"dir1/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
				"z.txt":          {Mode: 0o600, Contents: "z.txt contents"},
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
			visitor: func(relPath string, de fs.DirEntry) (CopyHint, error) {
				if relPath == "subdir" {
					return CopyHint{
						Skip: true,
					}, nil
				}
				if strings.HasPrefix(relPath, "subdir/") {
					panic("no children of subdir/ should have been walked")
				}
				return CopyHint{}, nil
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
			visitor: func(relPath string, de fs.DirEntry) (CopyHint, error) {
				return CopyHint{
					BackupIfExists:   true,
					AllowPreexisting: true,
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
			name: "sha256_hash",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "file1 contents"},
			},
			hasher: sha256.New,
			want: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "file1 contents"},
			},
			wantHashesHex: map[string]string{
				"file1.txt": "226e7cfa701fb8ba542d42e0f8bd3090cbbcc9f54d834f361c0ab8c3f4846b72",
			},
		},
		{
			name: "hash_other_than_sha256",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "file1 contents"},
			},
			hasher: sha512.New,
			want: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "file1 contents"},
			},
			wantHashesHex: map[string]string{
				"file1.txt": "a4b1d14ff0861c692abb6789d38c92d118a5febd000248d3b1002357ce0633d23ab12034bb1efd8d884058cec99da31cf646fb6179979b2fb231ba80e0bbc495",
			},
		},
		{
			name: "hash_in_subdir",
			srcDirContents: map[string]abctestutil.ModeAndContents{
				"subdir/file1.txt": {Mode: 0o600, Contents: "file1 contents"},
				"subdir/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
			hasher: sha256.New,
			want: map[string]abctestutil.ModeAndContents{
				"subdir/file1.txt": {Mode: 0o600, Contents: "file1 contents"},
				"subdir/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
			wantHashesHex: map[string]string{
				"subdir/file1.txt": "226e7cfa701fb8ba542d42e0f8bd3090cbbcc9f54d834f361c0ab8c3f4846b72",
				"subdir/file2.txt": "0140c0c66a644ab2dd27ac5536f20cc373d6fd1896f9838ecb4595675dda01fa",
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

			abctestutil.WriteAllMode(t, fromDir, tc.srcDirContents)

			from := fromDir
			to := toDir
			if tc.suffix != "" {
				from = filepath.Join(fromDir, tc.suffix)
				to = filepath.Join(toDir, tc.suffix)
			}
			abctestutil.WriteAllMode(t, toDir, tc.dstDirInitialContents)
			fs := &ErrorFS{
				FS: &RealFS{},

				MkdirAllErr: tc.mkdirAllErr,
				OpenErr:     tc.openErr,
				OpenFileErr: tc.openFileErr,
				StatErr:     tc.statErr,
			}
			ctx := context.Background()

			clk := clock.NewMock()
			const unixTime = 1688609125
			clk.Set(time.Unix(unixTime, 0)) // Arbitrary timestamp

			var hashes map[string][]byte
			if tc.hasher != nil {
				hashes = make(map[string][]byte)
			}

			err := CopyRecursive(ctx, &model.ConfigPos{}, &CopyParams{
				BackupDirMaker: func(rf FS) (string, error) { return backupDir, nil },
				SrcRoot:        from,
				DstRoot:        to,
				DryRun:         tc.dryRun,
				Hasher:         tc.hasher,
				OutHashes:      hashes,
				FS:             fs,
				Visitor:        tc.visitor,
			})
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			got := abctestutil.LoadDirMode(t, toDir)
			if diff := cmp.Diff(got, tc.want, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("destination directory was not as expected (-got,+want): %s", diff)
			}

			gotBackups := abctestutil.LoadDirMode(t, backupDir)
			if diff := cmp.Diff(gotBackups, tc.wantBackups, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("backups directory was not as expected (-got,+want): %s", diff)
			}

			wantHashes := map[string][]byte{}
			for filename, hexHash := range tc.wantHashesHex {
				buf, err := hex.DecodeString(hexHash)
				if err != nil {
					t.Fatal(err)
				}
				wantHashes[filename] = buf
			}
			if diff := cmp.Diff(hashes, wantHashes, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("hashes were not as expected: (-got,+want): %s", diff)
			}
		})
	}
}

func TestCopyRecursive_SymlinkHandling(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		regularFiles []string
		// regularFiles []string
		symlinks    []string
		wantErrPath string
	}{
		{
			name:        "one_symlink",
			symlinks:    []string{"my-symlink"},
			wantErrPath: "my-symlink",
		},
		{
			name:        "one_symlink_in_subdir",
			symlinks:    []string{"dir1/my-symlink"},
			wantErrPath: "dir1/my-symlink",
		},
		{
			name:         "no_symlinks",
			regularFiles: []string{"file1.txt", "dir/file1.txt"},
		},
		{
			name:        "multi_symlinks",
			symlinks:    []string{"dir/my-symlink-1", "dir/my-symlink-2"},
			wantErrPath: "dir/my-symlink-1",
		},
		{
			name:         "mix_symlinks_and_regular",
			regularFiles: []string{"my-regular-file"},
			symlinks:     []string{"my-symlink"},
			wantErrPath:  "my-symlink",
		},
		{
			name: "empty_dir",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sourceTempDir := t.TempDir()

			for _, r := range tc.regularFiles {
				path := filepath.Join(sourceTempDir, r)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				abctestutil.Overwrite(t, sourceTempDir, r, "contents")
			}
			for _, s := range tc.symlinks {
				path := filepath.Join(sourceTempDir, s)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("link-dest", path); err != nil {
					t.Fatal(err)
				}
			}

			destTempDir := t.TempDir()
			ctx := context.Background()
			err := CopyRecursive(ctx, nil, &CopyParams{
				FS:             &RealFS{},
				SrcRoot:        sourceTempDir,
				DstRoot:        destTempDir,
				ForbidSymlinks: true,
			})
			if err == nil {
				if tc.wantErrPath != "" {
					t.Fatal("got no error, but wanted ErrSymlinkForbidden")
				}
				return
			}
			if tc.wantErrPath == "" {
				t.Fatalf("got an unexpected error %q", err)
			}
			var symlinkErr *ErrSymlinkForbidden
			if !errors.As(err, &symlinkErr) {
				t.Fatalf("got unexpected error type %T: %v", err, err)
			}
			if symlinkErr.Path != tc.wantErrPath {
				t.Fatalf("got unexpected error path %q, wanted %q", symlinkErr.Path, tc.wantErrPath)
			}
		})
	}
}
