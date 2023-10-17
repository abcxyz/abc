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
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/abcxyz/pkg/testutil"
	"golang.org/x/exp/slices"
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
	skipUnlessEnvEnabled(t)

	t.Parallel()

	cases := []struct {
		name        string
		remote      string
		branchOrTag string
		wantErr     string
	}{
		{
			name:        "clone_tag",
			remote:      "https://github.com/abcxyz/abc.git",
			branchOrTag: "v0.2.0",
		},
		{
			name:        "alternative_tag_format_fails",
			remote:      "https://github.com/abcxyz/abc.git",
			branchOrTag: "refs/tags/v0.2.0",
			wantErr:     "refs/tags/v0.2.0 not found",
		},
		{
			name:        "clone_branch",
			remote:      "https://github.com/abcxyz/abc.git",
			branchOrTag: "main",
		},
		{
			name:        "alternative_branch_format_fails",
			remote:      "https://github.com/abcxyz/abc.git",
			branchOrTag: "refs/heads/v0.2.0",
			wantErr:     "refs/heads/v0.2.0 not found",
		},
		{
			name:        "long_commit_not_supported",
			remote:      "https://github.com/abcxyz/abc.git",
			branchOrTag: "b6687471f424efd125f9a3e156c68ed78b9d3b47",
			wantErr:     "Could not find remote branch b6687471f424efd125f9a3e156c68ed78b9d3b47 to clone",
		},
		{
			name:        "short_commit_not_supported",
			remote:      "https://github.com/abcxyz/abc.git",
			branchOrTag: "b668747",
			wantErr:     "Could not find remote branch b668747 to clone",
		},
		{
			name:    "nonexistent_remote",
			remote:  "https://example.com/foo/bar",
			wantErr: "repository 'https://example.com/foo/bar/' not found",
		},
		{
			name:        "symlinks_forbidden",
			remote:      "https://github.com/abcxyz/abc.git",
			branchOrTag: "drevell/forbidden-symlink-for-test",
			wantErr:     `a symlink was found in "https://github.com/abcxyz/abc.git" at "example-symlink"`,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			outDir := t.TempDir()
			err := Clone(ctx, tc.remote, tc.branchOrTag, outDir)
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
				if errors.Is(err, os.ErrNotExist) {
					t.Fatalf("git clone seemed to work but the output didn't contain %q, something weird happened", wantFile)
				}
				t.Error(err)
			}
		})
	}
}
