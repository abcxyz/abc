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

package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/exp/slices"

	"github.com/abcxyz/abc/templates/common"
	abctestutil "github.com/abcxyz/abc/templates/testutil"
	"github.com/abcxyz/pkg/testutil"
)

// To actually run the tests in this file, you'll need to set this environment
// variable.
//
// For example:
//
//	$ ABC_TEST_NON_HERMETIC=true go test ./...
const envName = "ABC_TEST_NON_HERMETIC"

func skipUnlessEnvEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv(envName) == "" {
		t.Skipf("skipping test because env var %q isn't set", envName)
	}
}

func TestTags(t *testing.T) {
	skipUnlessEnvEnabled(t)

	t.Parallel()

	ctx := context.Background()
	tags, err := RemoteTags(ctx, "https://github.com/abcxyz/abc.git")
	if err != nil {
		t.Error(err)
	}
	wantTag := "v0.2.0"
	if !slices.Contains(tags, wantTag) {
		t.Errorf("got versions %v, but wanted a list of versions containing %v", tags, wantTag)
	}
}

func TestClone(t *testing.T) {
	skipUnlessEnvEnabled(t)

	t.Parallel()

	cases := []struct {
		name    string
		remote  string
		version string
		wantErr string
	}{
		{
			name:    "clone_tag",
			remote:  "https://github.com/abcxyz/abc.git",
			version: "v0.2.0",
		},
		{
			name:    "alternative_tag_format_fails",
			remote:  "https://github.com/abcxyz/abc.git",
			version: "refs/tags/v0.2.0",
			wantErr: "refs/tags/v0.2.0 not found",
		},
		{
			name:    "clone_branch",
			remote:  "https://github.com/abcxyz/abc.git",
			version: "main",
		},
		{
			name:    "alternative_branch_format_fails",
			remote:  "https://github.com/abcxyz/abc.git",
			version: "refs/heads/v0.2.0",
			wantErr: "refs/heads/v0.2.0 not found",
		},
		{
			name:    "long_commit_supported",
			remote:  "https://github.com/abcxyz/abc.git",
			version: "b6687471f424efd125f9a3e156c68ed78b9d3b47",
		},
		{
			name:    "non_hexadecimal_long_commit",
			remote:  "https://github.com/abcxyz/abc.git",
			version: "z668747&-424.fd125f9a3e156c68ed78b9d3b47",
			wantErr: "z668747&-424.fd125f9a3e156c68ed78b9d3b47 not found",
		},
		{
			name:    "short_commit_not_supported",
			remote:  "https://github.com/abcxyz/abc.git",
			version: "b668747",
			wantErr: "Could not find remote branch b668747 to clone",
		},
		{
			name:    "nonexistent_remote",
			remote:  "https://example.com/foo/bar",
			wantErr: "fatal: ", // the part of the error message that's the same on win and linux
		},
		{
			name:    "symlinks_forbidden",
			remote:  "https://github.com/abcxyz/abc.git",
			version: "drevell/forbidden-symlink-for-test",
			wantErr: `one or more symlinks were found in "https://github.com/abcxyz/abc.git" at [example-symlink]`,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			outDir := t.TempDir()
			err := Clone(ctx, tc.remote, tc.version, outDir)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}
			if tc.wantErr != "" {
				return
			}

			// We check for an arbitrary file to ensure that the clone really happened.
			wantFile := "README.md"
			_, err = os.Stat(filepath.Join(outDir, wantFile))
			if err != nil {
				if common.IsStatNotExistErr(err) {
					t.Fatalf("git clone seemed to work but the output didn't contain %q, something weird happened", wantFile)
				}
				t.Error(err)
			}
		})
	}
}

func TestFindSymlinks(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		regularFiles []string
		symlinks     []string
		want         []string
	}{
		{
			name:     "one_symlink",
			symlinks: []string{"my-symlink"},
			want:     []string{"my-symlink"},
		},
		{
			name: "multi_symlinks",
			symlinks: []string{
				"my-symlink-1",
				"my-symlink-2",
			},
			want: []string{"my-symlink-1", "my-symlink-2"},
		},
		{
			name:         "mix_symlinks_and_regular",
			symlinks:     []string{"my-symlink"},
			regularFiles: []string{"my-regular-file"},
			want:         []string{"my-symlink"},
		},
		{
			name:         "no_symlinks",
			regularFiles: []string{"my-regular-file"},
		},
		{
			name:     "dot_git_is_skipped",
			symlinks: []string{".git/my-symlink"},
		},
		{
			name:     "dot_git_outside_of_root_is_not_skipped",
			symlinks: []string{"foo/.git/my-symlink"},
			want:     []string{"foo/.git/my-symlink"},
		},
		{
			name: "empty_dir",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()

			for _, r := range tc.regularFiles {
				path := filepath.Join(tempDir, r)
				if err := os.WriteFile(path, []byte("my-contents"), 0o644); err != nil { //nolint:gosec
					t.Fatal(err)
				}
			}
			for _, s := range tc.symlinks {
				path := filepath.Join(tempDir, s)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("link-dest", path); err != nil {
					t.Fatal(err)
				}
			}

			got, err := findSymlinks(tempDir)
			if err != nil {
				t.Fatal(err)
			}

			if !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHeadTags(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		dir     string
		files   map[string]string
		want    []string
		wantErr string
	}{
		{
			name:  "simple_success_no_tag",
			dir:   ".",
			files: abctestutil.WithGitRepoAt("", nil),
			want:  nil,
		},
		{
			name: "simple_success_with_tag",
			dir:  ".",
			files: abctestutil.WithGitRepoAt("",
				map[string]string{
					".git/refs/tags/v1.2.3": abctestutil.MinimalGitHeadSHA,
				}),
			want: []string{"v1.2.3"},
		},
		{
			name: "multiple_tags",
			dir:  ".",
			files: abctestutil.WithGitRepoAt("",
				map[string]string{
					".git/refs/tags/v1.2.3": abctestutil.MinimalGitHeadSHA,
					".git/refs/tags/v2.3.4": abctestutil.MinimalGitHeadSHA,
				}),
			want: []string{
				"v1.2.3",
				"v2.3.4",
			},
		},
		{
			name: "git_repo_in_subdir",
			dir:  "mysubdir",
			files: abctestutil.WithGitRepoAt("mysubdir",
				map[string]string{
					"mysubdir/.git/refs/tags/v1.2.3": abctestutil.MinimalGitHeadSHA,
				}),
			want: []string{"v1.2.3"},
		},
		{
			name:    "not_git_repo_error",
			dir:     ".",
			wantErr: "not a git repository",
		},
	}
	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			tmpDir := t.TempDir()
			abctestutil.WriteAllDefaultMode(t, tmpDir, tc.files)

			dir := filepath.Join(tmpDir, tc.dir)

			got, err := HeadTags(ctx, dir)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Errorf(diff)
			}

			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("output tags weren't as expected (-got,+want): %s", diff)
			}
		})
	}
}
