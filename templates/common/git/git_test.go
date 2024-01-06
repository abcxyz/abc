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

	"golang.org/x/exp/slices"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/pkg/testutil"
)

// To skip the tests in this file, you'll need to set this environment
// variable.
//
// For example:
//
//	$ SKIP_TEST_NON_HERMETIC=true go test ./...
const envName = "SKIP_TEST_NON_HERMETIC"

func skipWhenEnvEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv(envName) == "true" {
		t.Skipf("skipping test because env var %q is set", envName)
	}
}

func TestTags(t *testing.T) {
	skipWhenEnvEnabled(t)

	t.Parallel()

	ctx := context.Background()
	tags, err := Tags(ctx, "https://github.com/abcxyz/abc.git")
	if err != nil {
		t.Error(err)
	}
	wantTag := "v0.2.0"
	if !slices.Contains(tags, wantTag) {
		t.Errorf("got versions %v, but wanted a list of versions containing %v", tags, wantTag)
	}
}

func TestClone(t *testing.T) {
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
			wantErr: "repository 'https://example.com/foo/bar/' not found",
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
				path := filepath.Join(tempDir, filepath.FromSlash(r))
				if err := os.WriteFile(path, []byte("my-contents"), 0o644); err != nil { //nolint:gosec
					t.Fatal(err)
				}
			}
			for _, s := range tc.symlinks {
				path := filepath.Join(tempDir, filepath.FromSlash(s))
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

			want := make([]string, 0, len(tc.want))
			for _, w := range tc.want {
				want = append(want, filepath.FromSlash(w))
			}
			if !slices.Equal(got, want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestVersionForManifest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		dir        string
		allowDirty bool
		files      map[string]string
		want       string
		wantErr    string
	}{
		{
			name:  "simple_success_no_tag",
			dir:   ".",
			files: common.WithGitRepoAt("", nil),
			want:  common.MinimalGitHeadSHA,
		},
		{
			name: "simple_success_with_tag",
			dir:  ".",
			files: common.WithGitRepoAt("",
				map[string]string{
					".git/refs/tags/v1.2.3": common.MinimalGitHeadSHA,
				}),
			want: "v1.2.3",
		},
		{
			name: "semver_ordering",
			dir:  ".",
			files: common.WithGitRepoAt("",
				map[string]string{
					".git/refs/tags/v4.5.6":  common.MinimalGitHeadSHA,
					".git/refs/tags/v11.0.0": common.MinimalGitHeadSHA,
					".git/refs/tags/v7.8.9":  common.MinimalGitHeadSHA,
				}),
			want: "v11.0.0",
		},
		{
			name: "only_v_prefix_counts_as_semver",
			dir:  ".",
			files: common.WithGitRepoAt("",
				map[string]string{
					".git/refs/tags/v4.5.6": common.MinimalGitHeadSHA,
					".git/refs/tags/11.0.0": common.MinimalGitHeadSHA,
					".git/refs/tags/v7.8.9": common.MinimalGitHeadSHA,
				}),
			want: "v7.8.9",
		},
		{
			name: "semver_before_non_semver",
			dir:  ".",
			files: common.WithGitRepoAt("",
				map[string]string{
					".git/refs/tags/v4.5.6":    common.MinimalGitHeadSHA,
					".git/refs/tags/zzzzzz":    common.MinimalGitHeadSHA,
					".git/refs/tags/999999":    common.MinimalGitHeadSHA,
					".git/refs/tags/v5xxx.0.0": common.MinimalGitHeadSHA,
				}),
			want: "v4.5.6",
		},
		{
			name: "non_semver_in_reverse_order",
			dir:  ".",
			files: common.WithGitRepoAt("",
				map[string]string{
					".git/refs/tags/a": common.MinimalGitHeadSHA,
					".git/refs/tags/z": common.MinimalGitHeadSHA,
					".git/refs/tags/j": common.MinimalGitHeadSHA,
				}),
			want: "z",
		},
		{
			name:  "not_a_git_repo",
			dir:   ".",
			files: nil,
		},
		{
			name:       "dirty_workspace_not_allowed",
			dir:        ".",
			allowDirty: false,
			files: common.WithGitRepoAt("", map[string]string{
				"my_file.txt": "my contents",
			}),
		},
		{
			name:       "dirty_workspace_allowed",
			dir:        ".",
			allowDirty: true,
			files: common.WithGitRepoAt("", map[string]string{
				"my_file.txt": "my contents",
			}),
			want: common.MinimalGitHeadSHA,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmp := t.TempDir()
			common.WriteAllDefaultMode(t, tmp, tc.files)
			ctx := context.Background()
			got, gotOK, err := VersionForManifest(ctx, filepath.Join(tmp, tc.dir), tc.allowDirty)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if (got == "") == gotOK {
				t.Errorf("ok should be true if and only if the version is non-empty")
			}

			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
