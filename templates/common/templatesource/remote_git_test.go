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
					t:           t,
					addTag:      "v1.2.3",
					out:         basicFiles,
					wantRemote:  "fake-remote",
					wantVersion: "v1.2.3",
				},
			},
			want: basicFiles,
			wantDLMeta: &DownloadMetadata{
				IsCanonical:     true,
				CanonicalSource: "mysource",
				LocationType:    RemoteGit,
				HasVersion:      true,
				Version:         "v1.2.3",
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
					t:           t,
					addTag:      "v1.2.3",
					out:         basicFiles,
					wantRemote:  "fake-remote",
					wantVersion: "v1.2.3",
				},
				tagser: &fakeTagser{
					t:          t,
					wantRemote: "fake-remote",
					out: []string{
						"v1.2.3",
						"v0.1.2",
					},
				},
			},
			want: basicFiles,
			wantDLMeta: &DownloadMetadata{
				IsCanonical:     true,
				CanonicalSource: "mysource",
				LocationType:    RemoteGit,
				HasVersion:      true,
				Version:         "v1.2.3",
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
					t:      t,
					addTag: "v1.2.3",
					out: map[string]string{
						"my-subdir/file1.txt": "hello",
						"file2.txt":           "world",
					},
					wantRemote:  "fake-remote",
					wantVersion: "v1.2.3",
				},
			},
			want: map[string]string{
				"file1.txt": "hello",
			},
			wantDLMeta: &DownloadMetadata{
				IsCanonical:     true,
				CanonicalSource: "mysource",
				LocationType:    RemoteGit,
				HasVersion:      true,
				Version:         "v1.2.3",
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
					t:      t,
					addTag: "v1.2.3",
					out: map[string]string{
						"my/deep/subdir/file1.txt": "hello",
						"file2.txt":                "world",
					},
					wantRemote:  "fake-remote",
					wantVersion: "v1.2.3",
				},
			},
			want: map[string]string{
				"subdir/file1.txt": "hello",
			},
			wantDLMeta: &DownloadMetadata{
				IsCanonical:     true,
				CanonicalSource: "mysource",
				LocationType:    RemoteGit,
				HasVersion:      true,
				Version:         "v1.2.3",
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
					t:           t,
					out:         basicFiles,
					wantRemote:  "fake-remote",
					wantVersion: "v1.2.3",
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
					t:           t,
					out:         basicFiles,
					wantRemote:  "fake-remote",
					wantVersion: "v1.2.3",
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
					t:           t,
					out:         basicFiles,
					wantRemote:  "fake-remote",
					wantVersion: abctestutil.MinimalGitHeadSHA,
				},
			},
			want: basicFiles,
			wantDLMeta: &DownloadMetadata{
				IsCanonical:     true,
				CanonicalSource: "mysource",
				LocationType:    RemoteGit,
				HasVersion:      true,
				Version:         abctestutil.MinimalGitHeadSHA,
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
					t:           t,
					addTag:      "v1.2.3",
					out:         basicFiles,
					wantRemote:  "fake-remote",
					wantVersion: abctestutil.MinimalGitHeadSHA,
				},
			},
			want: basicFiles,
			wantDLMeta: &DownloadMetadata{
				IsCanonical:     true,
				CanonicalSource: "mysource",
				LocationType:    RemoteGit,
				HasVersion:      true,
				Version:         "v1.2.3",
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
		name     string
		in       string
		inRemote string
		tagser   *fakeTagser
		want     string
		wantErr  string
	}{
		{
			name: "version_other_than_latest_is_returned_verbatim",
			in:   "v1.2.3",
			want: "v1.2.3",
		},
		{
			name: "version_with_sha",
			in:   "b488f14a5302518e0ba347712e6dc4db4d0f7ce5",
			want: "b488f14a5302518e0ba347712e6dc4db4d0f7ce5",
		},
		{
			name: "version_with_main_branch",
			in:   "main",
			want: "main",
		},
		{
			name: "version_with_forward_slash",
			in:   "username/branch-name",
			want: "username/branch-name",
		},
		{
			name: "version_with_snake_case",
			in:   "branch_name",
			want: "branch_name",
		},
		{
			name:    "empty_input",
			in:      "",
			wantErr: "cannot be empty",
		},
		{
			name: "version_with_suffix_can_be_specifically_requested",
			in:   "v1.2.3-alpha",
			want: "v1.2.3-alpha",
		},
		{
			name:     "latest_lookup",
			in:       "latest",
			inRemote: "my-remote",
			tagser: &fakeTagser{
				t:          t,
				wantRemote: "my-remote",
				out:        []string{"v1.2.3", "v2.3.4"},
			},
			want: "v2.3.4",
		},
		{
			name:     "latest_lookup_v_prefix_is_required",
			in:       "latest",
			inRemote: "my-remote",
			tagser: &fakeTagser{
				t:          t,
				wantRemote: "my-remote",
				out:        []string{"v1.2.3", "2.3.4"},
			},
			want: "v1.2.3",
		},
		{
			name:     "latest_lookup_ignores_alpha",
			in:       "latest",
			inRemote: "my-remote",
			tagser: &fakeTagser{
				t:          t,
				wantRemote: "my-remote",
				out:        []string{"v1.2.3", "v2.3.4-alpha"},
			},
			want: "v1.2.3",
		},
		{
			name:     "latest_lookup_ignores_nonsense_tag",
			in:       "latest",
			inRemote: "my-remote",
			tagser: &fakeTagser{
				t:          t,
				wantRemote: "my-remote",
				out:        []string{"v1.2.3", "nonsense"},
			},
			want: "v1.2.3",
		},
		{
			name:     "no_tags_exist",
			in:       "latest",
			inRemote: "my-remote",
			tagser: &fakeTagser{
				t:          t,
				wantRemote: "my-remote",
				out:        []string{},
			},
			wantErr: `there were no semver-formatted tags beginning with "v" in "my-remote"`,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			got, err := resolveVersion(ctx, tc.tagser, tc.inRemote, tc.in)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}

			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

type fakeCloner struct {
	t           *testing.T
	out         map[string]string
	addTag      string
	wantRemote  string
	wantVersion string
}

func (f *fakeCloner) Clone(ctx context.Context, remote, version, outDir string) error {
	if remote != f.wantRemote {
		f.t.Errorf("got remote %q, want %q", remote, f.wantRemote)
	}
	if version != f.wantVersion {
		f.t.Errorf("got version %q, want %q", version, f.wantVersion)
	}

	files := abctestutil.WithGitRepoAt("", f.out)

	if f.addTag != "" {
		// Adding a tag is just creating a file under .git/refs/tags.
		files[".git/refs/tags/"+f.addTag] = abctestutil.MinimalGitHeadSHA
	}

	abctestutil.WriteAll(f.t, outDir, files)
	return nil
}

type fakeTagser struct {
	t          *testing.T
	out        []string
	wantRemote string
}

func (f *fakeTagser) Tags(ctx context.Context, remote string) ([]string, error) {
	if remote != f.wantRemote {
		f.t.Errorf("got remote %q, want %q", remote, f.wantRemote)
	}
	return f.out, nil
}
