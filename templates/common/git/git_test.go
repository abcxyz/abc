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
	"github.com/abcxyz/abc/templates/common/run"
	abctestutil "github.com/abcxyz/abc/templates/testutil"
	"github.com/abcxyz/pkg/testutil"
)

// To actually run the tests in this file that require a remote git repo, you'll
// need to set this environment variable.
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

func TestLocalTags(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	tempDir := t.TempDir()

	abctestutil.WriteAll(t, tempDir, abctestutil.WithGitRepoAt("", nil))

	got, err := LocalTags(ctx, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d tags, but expected 0 tags in an empty repo", len(got))
	}

	abctestutil.OverwriteJoin(t, tempDir, "myfile1.txt", "some contents")

	// If we don't do this, there will be an error on commit
	mustRun(ctx, t, "git", "config", "-f", tempDir+"/.git/config", "user.email", "fake@example.com")
	mustRun(ctx, t, "git", "config", "-f", tempDir+"/.git/config", "user.name", "Nobody")

	mustRun(ctx, t, "git", "-C", tempDir, "add", "-A")
	mustRun(ctx, t, "git", "-C", tempDir, "commit", "--no-gpg-sign", "--author", "nobody <nobody>", "-m", "my first commit")
	mustRun(ctx, t, "git", "-C", tempDir, "tag", "mytag1")

	got, err = LocalTags(ctx, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"mytag1"}
	if !slices.Equal(got, want) {
		t.Fatalf("got tags %v, want %v", got, want)
	}

	abctestutil.OverwriteJoin(t, tempDir, "myfile2.txt", "some contents")
	mustRun(ctx, t, "git", "-C", tempDir, "add", "-A")
	mustRun(ctx, t, "git", "-C", tempDir, "commit", "--no-gpg-sign", "--author", "nobody <nobody>", "-m", "my second commit")
	mustRun(ctx, t, "git", "-C", tempDir, "tag", "mytag2")

	got, err = LocalTags(ctx, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"mytag1", "mytag2"}
	if !slices.Equal(got, want) {
		t.Fatalf("got tags %v, want %v", got, want)
	}
}

func TestClone(t *testing.T) {
	skipUnlessEnvEnabled(t)

	t.Parallel()

	cases := []struct {
		name    string
		remote  string
		wantErr string
	}{
		{
			name:   "simple_success",
			remote: "https://github.com/abcxyz/abc.git",
		},
		{
			name:   "nonexistent_remote",
			remote: "https://example.com/foo/bar.git",
			// The error message is completely different between linux and mac.
			// This is the only substring that appears on both platforms. 🙃
			wantErr: "https://example.com/foo/bar.git",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			tempDir := t.TempDir()
			err := Clone(ctx, tc.remote, tempDir)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}
			if tc.wantErr != "" {
				return
			}

			// We check for an arbitrary file to ensure that the clone really happened.
			wantFile := "README.md"
			exists, err := common.Exists(filepath.Join(tempDir, wantFile))
			if err != nil {
				t.Error(err)
			}
			if !exists {
				t.Fatalf("git clone seemed to work but the output didn't contain %q, something weird happened", wantFile)
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
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()

			tmpDir := t.TempDir()
			abctestutil.WriteAll(t, tmpDir, tc.files)

			dir := filepath.Join(tmpDir, tc.dir)

			got, err := HeadTags(ctx, dir)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("output tags weren't as expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestCheckout(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		branch  string
		tag     string
		version string
		wantErr string
	}{
		{
			name:    "branch",
			branch:  "mybranch",
			version: "mybranch",
		},
		{
			name:    "tag",
			tag:     "mytag",
			version: "mytag",
		},
		{
			name:    "sha",
			version: abctestutil.MinimalGitHeadSHA,
		},
		{
			name:    "branch_tag_collision",
			branch:  "foo",
			tag:     "foo",
			version: "foo",
		},
		{
			name:    "empty_version",
			wantErr: "empty string is not a valid version",
		},
		{
			name:    "nonexistent",
			version: "some-nonexistent-branch",
			wantErr: `version "some-nonexistent-branch" doesn't exist`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			abctestutil.WriteAll(t, tempDir, abctestutil.WithGitRepoAt("", nil))

			ctx := t.Context()
			if len(tc.branch) > 0 {
				abctestutil.OverwriteJoin(t, tempDir, ".git/refs/heads/"+tc.branch, abctestutil.MinimalGitHeadSHA)
			}
			if len(tc.tag) > 0 {
				abctestutil.OverwriteJoin(t, tempDir, ".git/refs/tags/"+tc.tag, abctestutil.MinimalGitHeadSHA)
			}
			err := Checkout(ctx, tc.version, tempDir)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func mustRun(ctx context.Context, tb testing.TB, args ...string) {
	tb.Helper()
	if _, _, err := run.Simple(ctx, args...); err != nil {
		tb.Fatal(err)
	}
}
