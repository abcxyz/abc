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

package templatesource

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	abctestutil "github.com/abcxyz/abc/templates/testutil"
	"github.com/abcxyz/pkg/testutil"
)

func TestRemoteGitDownloader_Download(t *testing.T) {
	t.Parallel()

	// Most subtests can use this simple set of files.
	basicFiles := map[string]string{
		"file1.txt":     "hello",
		"dir/file2.txt": "world",
	}

	cases := []struct {
		name       string
		dl         *remoteGitDownloader
		want       map[string]string
		wantDLMeta *DownloadMetadata
		wantErr    string
	}{
		{
			name: "no_subdir",
			dl: &remoteGitDownloader{
				canonicalSource: "mysource",
				remote:          "fake-remote",
				subdir:          "",
				version:         "v1.2.3",
				cloner: &fakeCloner{
					tb:         t,
					addTags:    []string{"v1.2.3"},
					out:        basicFiles,
					wantRemote: "fake-remote",
				},
			},
			want: basicFiles,
			wantDLMeta: &DownloadMetadata{
				IsCanonical:     true,
				CanonicalSource: "mysource",
				LocationType:    RemoteGit,
				Version:         "v1.2.3",
				UpgradeTrack:    "latest",
				Vars: DownloaderVars{
					GitTag:      "v1.2.3",
					GitSHA:      abctestutil.MinimalGitHeadSHA,
					GitShortSHA: abctestutil.MinimalGitHeadShortSHA,
				},
			},
		},
		{
			name: "latest_version_lookup",
			dl: &remoteGitDownloader{
				canonicalSource: "mysource",
				remote:          "fake-remote",
				subdir:          "",
				version:         "latest",
				cloner: &fakeCloner{
					tb:         t,
					addTags:    []string{"v0.0.1", "v1.2.3", "v0.1.2"},
					out:        basicFiles,
					wantRemote: "fake-remote",
				},
			},
			want: basicFiles,
			wantDLMeta: &DownloadMetadata{
				IsCanonical:     true,
				CanonicalSource: "mysource",
				LocationType:    RemoteGit,
				Version:         "v1.2.3",
				UpgradeTrack:    "latest",
				Vars: DownloaderVars{
					GitTag:      "v1.2.3",
					GitSHA:      abctestutil.MinimalGitHeadSHA,
					GitShortSHA: abctestutil.MinimalGitHeadShortSHA,
				},
			},
		},
		{
			name: "main_branch",
			dl: &remoteGitDownloader{
				canonicalSource: "mysource",
				remote:          "fake-remote",
				subdir:          "",
				version:         "main",
				cloner: &fakeCloner{
					tb:          t,
					addBranches: []string{"main"},
					addTags:     []string{"v1.2.3"},
					out:         basicFiles,
					wantRemote:  "fake-remote",
				},
			},
			want: basicFiles,
			wantDLMeta: &DownloadMetadata{
				IsCanonical:     true,
				CanonicalSource: "mysource",
				LocationType:    RemoteGit,
				Version:         "v1.2.3",
				UpgradeTrack:    "main",
				Vars: DownloaderVars{
					GitTag:      "v1.2.3",
					GitSHA:      abctestutil.MinimalGitHeadSHA,
					GitShortSHA: abctestutil.MinimalGitHeadShortSHA,
				},
			},
		},
		{
			name: "with_subdir",
			dl: &remoteGitDownloader{
				canonicalSource: "mysource",
				remote:          "fake-remote",
				subdir:          "my-subdir",
				version:         "v1.2.3",
				cloner: &fakeCloner{
					tb:      t,
					addTags: []string{"v1.2.3"},
					out: map[string]string{
						"my-subdir/file1.txt": "hello",
						"file2.txt":           "world",
					},
					wantRemote: "fake-remote",
				},
			},
			want: map[string]string{
				"file1.txt": "hello",
			},
			wantDLMeta: &DownloadMetadata{
				IsCanonical:     true,
				CanonicalSource: "mysource",
				LocationType:    RemoteGit,
				Version:         "v1.2.3",
				UpgradeTrack:    "latest",
				Vars: DownloaderVars{
					GitTag:      "v1.2.3",
					GitSHA:      abctestutil.MinimalGitHeadSHA,
					GitShortSHA: abctestutil.MinimalGitHeadShortSHA,
				},
			},
		},
		{
			name: "with_deep_subdir",
			dl: &remoteGitDownloader{
				canonicalSource: "mysource",
				remote:          "fake-remote",
				subdir:          "my/deep",
				version:         "v1.2.3",
				cloner: &fakeCloner{
					tb:      t,
					addTags: []string{"v1.2.3"},
					out: map[string]string{
						"my/deep/subdir/file1.txt": "hello",
						"file2.txt":                "world",
					},
					wantRemote: "fake-remote",
				},
			},
			want: map[string]string{
				"subdir/file1.txt": "hello",
			},
			wantDLMeta: &DownloadMetadata{
				IsCanonical:     true,
				CanonicalSource: "mysource",
				LocationType:    RemoteGit,
				Version:         "v1.2.3",
				UpgradeTrack:    "latest",
				Vars: DownloaderVars{
					GitTag:      "v1.2.3",
					GitSHA:      abctestutil.MinimalGitHeadSHA,
					GitShortSHA: abctestutil.MinimalGitHeadShortSHA,
				},
			},
		},
		{
			name: "dot_dot_subdir",
			dl: &remoteGitDownloader{
				remote:  "fake-remote",
				subdir:  "..",
				version: "v1.2.3",
			},
			wantErr: `must not contain ".."`,
			want:    map[string]string{},
		},
		{
			name: "missing_subdir",
			dl: &remoteGitDownloader{
				remote:  "fake-remote",
				subdir:  "nonexistent",
				version: "v1.2.3",
				cloner: &fakeCloner{
					tb:         t,
					out:        basicFiles,
					addTags:    []string{"v1.2.3"},
					wantRemote: "fake-remote",
				},
			},
			wantErr: `doesn't contain a subdirectory named "nonexistent"`,
			want:    map[string]string{},
		},
		{
			name: "file_instead_of_dir",
			dl: &remoteGitDownloader{
				remote:  "fake-remote",
				subdir:  "file1.txt",
				version: "v1.2.3",
				cloner: &fakeCloner{
					tb:         t,
					out:        basicFiles,
					addTags:    []string{"v1.2.3"},
					wantRemote: "fake-remote",
				},
			},
			wantErr: "is not a directory",
			want:    map[string]string{},
		},
		{
			name: "clone_by_sha",
			dl: &remoteGitDownloader{
				canonicalSource: "mysource",
				remote:          "fake-remote",
				subdir:          "",
				version:         abctestutil.MinimalGitHeadSHA,
				cloner: &fakeCloner{
					tb:         t,
					out:        basicFiles,
					wantRemote: "fake-remote",
				},
			},
			want: basicFiles,
			wantDLMeta: &DownloadMetadata{
				IsCanonical:     true,
				CanonicalSource: "mysource",
				LocationType:    RemoteGit,
				Version:         abctestutil.MinimalGitHeadSHA,
				UpgradeTrack:    "latest",
				Vars: DownloaderVars{
					GitTag:      "",
					GitSHA:      abctestutil.MinimalGitHeadSHA,
					GitShortSHA: abctestutil.MinimalGitHeadShortSHA,
				},
			},
		},
		{
			name: "clone_by_sha_with_detected_tag",
			dl: &remoteGitDownloader{
				canonicalSource: "mysource",
				remote:          "fake-remote",
				subdir:          "",
				version:         abctestutil.MinimalGitHeadSHA,
				cloner: &fakeCloner{
					tb:         t,
					addTags:    []string{"v1.2.3"},
					out:        basicFiles,
					wantRemote: "fake-remote",
				},
			},
			want: basicFiles,
			wantDLMeta: &DownloadMetadata{
				IsCanonical:     true,
				CanonicalSource: "mysource",
				LocationType:    RemoteGit,
				Version:         "v1.2.3",
				UpgradeTrack:    "latest",
				Vars: DownloaderVars{
					GitTag:      "v1.2.3",
					GitSHA:      abctestutil.MinimalGitHeadSHA,
					GitShortSHA: abctestutil.MinimalGitHeadShortSHA,
				},
			},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			tempDir := t.TempDir()
			gotDLMeta, err := tc.dl.Download(ctx, "", tempDir, "")
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}
			got := abctestutil.LoadDir(t, tempDir)
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("output files were not as expected (-got, +want): %s", diff)
			}
			if diff := cmp.Diff(gotDLMeta, tc.wantDLMeta); diff != "" {
				t.Errorf("DownloadMetadata was not as expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestResolveVersion(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		in               string
		inRemote         string
		branches         []string
		tags             []string
		wantVersion      string
		wantUpgradeTrack string
		wantErr          string
	}{
		{
			name:             "version_other_than_latest_is_returned_verbatim",
			in:               "v1.2.3",
			wantVersion:      "v1.2.3",
			wantUpgradeTrack: "latest",
		},
		{
			name:             "version_with_sha",
			in:               "b488f14a5302518e0ba347712e6dc4db4d0f7ce5",
			wantVersion:      "b488f14a5302518e0ba347712e6dc4db4d0f7ce5",
			wantUpgradeTrack: "latest",
		},
		{
			name:             "version_with_main_branch",
			in:               "main",
			branches:         []string{"main"},
			wantVersion:      "main",
			wantUpgradeTrack: "main",
		},
		{
			name:             "version_with_forward_slash",
			in:               "username/branch-name",
			wantVersion:      "username/branch-name",
			wantUpgradeTrack: "username/branch-name",
			branches:         []string{"username/branch-name"},
		},
		{
			name:             "version_with_snake_case",
			in:               "branch_name",
			wantVersion:      "branch_name",
			wantUpgradeTrack: "branch_name",
			branches:         []string{"branch_name"},
		},
		{
			name:    "empty_input",
			in:      "",
			wantErr: "cannot be empty",
		},
		{
			name:             "version_with_suffix_can_be_specifically_requested",
			in:               "v1.2.3-alpha",
			wantVersion:      "v1.2.3-alpha",
			wantUpgradeTrack: "latest",
		},
		{
			name:             "latest_lookup",
			in:               "latest",
			inRemote:         "my-remote",
			tags:             []string{"v1.2.3", "v2.3.4"},
			wantVersion:      "v2.3.4",
			wantUpgradeTrack: "latest",
		},
		{
			name:             "latest_lookup_v_prefix_is_required",
			in:               "latest",
			inRemote:         "my-remote",
			tags:             []string{"v1.2.3", "2.3.4"},
			wantVersion:      "v1.2.3",
			wantUpgradeTrack: "latest",
		},
		{
			name:             "latest_lookup_ignores_alpha",
			in:               "latest",
			inRemote:         "my-remote",
			tags:             []string{"v1.2.3", "v2.3.4-alpha"},
			wantVersion:      "v1.2.3",
			wantUpgradeTrack: "latest",
		},
		{
			name:             "latest_lookup_ignores_nonsense_tag",
			in:               "latest",
			inRemote:         "my-remote",
			tags:             []string{"v1.2.3", "nonsense"},
			wantVersion:      "v1.2.3",
			wantUpgradeTrack: "latest",
		},
		{
			name:     "no_tags_exist",
			in:       "latest",
			inRemote: "my-remote",
			tags:     []string{},
			wantErr:  `there were no semver-formatted tags beginning with "v"`,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			outDir := t.TempDir()
			createFakeGitRepo(t, tc.branches, tc.tags, outDir)

			gotVersion, gotTrack, err := resolveVersion(ctx, outDir, tc.in)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}

			if gotVersion != tc.wantVersion {
				t.Errorf("got %q, want %q", gotVersion, tc.wantVersion)
			}
			if gotTrack != tc.wantUpgradeTrack {
				t.Errorf("got upgrade track %q, want %q", gotTrack, tc.wantUpgradeTrack)
			}
		})
	}
}

type fakeCloner struct {
	tb          testing.TB
	out         map[string]string
	addTags     []string
	addBranches []string
	wantRemote  string
}

func (f *fakeCloner) Clone(ctx context.Context, remote, outDir string) error {
	if remote != f.wantRemote {
		f.tb.Errorf("got remote %q, want %q", remote, f.wantRemote)
	}

	createFakeGitRepo(f.tb, f.addBranches, f.addTags, outDir)
	abctestutil.WriteAll(f.tb, outDir, f.out)
	return nil
}

func createFakeGitRepo(tb testing.TB, branches, tags []string, outDir string) {
	tb.Helper()

	files := abctestutil.WithGitRepoAt("", nil)

	for _, tag := range tags {
		// Adding a tag is just creating a file under .git/refs/tags.
		files[".git/refs/tags/"+tag] = abctestutil.MinimalGitHeadSHA
	}
	for _, branch := range branches {
		// Adding a branch is just creating a file under .git/refs/branch.
		files[".git/refs/heads/"+branch] = abctestutil.MinimalGitHeadSHA
	}

	abctestutil.WriteAll(tb, outDir, files)
}
