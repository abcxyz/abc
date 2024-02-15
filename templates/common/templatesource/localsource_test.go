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
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	abctestutil "github.com/abcxyz/abc/templates/testutil"
	"github.com/abcxyz/pkg/sets"
	"github.com/abcxyz/pkg/testutil"
)

func TestLocalDownloader_Download(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		srcDir          string
		destDir         string
		initialContents map[string]string
		wantNewFiles    map[string]string
		wantDLMeta      *DownloadMetadata
		wantErr         string
	}{
		{
			name:    "simple_success",
			srcDir:  "src",
			destDir: "dst",
			initialContents: map[string]string{
				"src/file1.txt":   "file1 contents",
				"src/a/file2.txt": "file2 contents",
			},
			wantNewFiles: map[string]string{
				"dst/file1.txt":   "file1 contents",
				"dst/a/file2.txt": "file2 contents",
			},
			wantDLMeta: &DownloadMetadata{
				IsCanonical: false,
			},
		},
		{
			name:    "nonexistent_source",
			srcDir:  "nonexistent",
			wantErr: "nonexistent",
		},
		{
			name:    "dest_dir_in_same_git_workspace",
			srcDir:  "src",
			destDir: "dst",
			initialContents: abctestutil.WithGitRepoAt("",
				map[string]string{
					"src/spec.yaml": "file1 contents",
					"src/file1.txt": "file1 contents",
				}),
			wantNewFiles: map[string]string{
				"dst/spec.yaml": "file1 contents",
				"dst/file1.txt": "file1 contents",
			},
			wantDLMeta: &DownloadMetadata{
				IsCanonical:     true,
				CanonicalSource: "../src",
				LocationType:    "local_git",
				HasVersion:      true,
				Version:         abctestutil.MinimalGitHeadSHA,
				Vars: DownloaderVars{
					GitSHA:      abctestutil.MinimalGitHeadSHA,
					GitShortSHA: abctestutil.MinimalGitHeadShortSHA,
					GitTag:      "",
				},
			},
		},
		{
			name:    "dest_dir_in_same_git_workspace_with_tag",
			srcDir:  "src",
			destDir: "dst",
			initialContents: abctestutil.WithGitRepoAt("",
				map[string]string{
					"src/spec.yaml": "file1 contents",
					"src/file1.txt": "file1 contents",

					// This assumes that we're using the git repo created by
					// common.WithGitRepoAt(). We're tweaking the repo structure
					// to add a tag. The named SHA already exists in the repo.
					".git/refs/tags/mytag": abctestutil.MinimalGitHeadSHA,
				}),
			wantNewFiles: map[string]string{
				"dst/spec.yaml": "file1 contents",
				"dst/file1.txt": "file1 contents",
			},
			wantDLMeta: &DownloadMetadata{
				IsCanonical:     true,
				CanonicalSource: "../src",
				LocationType:    "local_git",
				HasVersion:      true,
				Version:         "mytag",
				Vars: DownloaderVars{
					GitSHA:      abctestutil.MinimalGitHeadSHA,
					GitShortSHA: abctestutil.MinimalGitHeadShortSHA,
					GitTag:      "mytag",
				},
			},
		},
		{
			name:    "dest_dir_in_same_git_workspace_with_detached_head",
			srcDir:  "src",
			destDir: "dst",
			initialContents: abctestutil.WithGitRepoAt("",
				map[string]string{
					"src/spec.yaml": "file1 contents",
					"src/file1.txt": "file1 contents",

					// This assumes that we're using the git repo created by
					// common.WithGitRepoAt(). We're putting the repo in a
					// detached HEAD state so we're not on a branch. The reason
					// this creates a detached head state is because .git/HEAD
					// would normally contain a branch name, but when you put a
					// SHA in there in means you have a detached head.
					".git/HEAD": abctestutil.MinimalGitHeadSHA,
				}),
			wantNewFiles: map[string]string{
				"dst/spec.yaml": "file1 contents",
				"dst/file1.txt": "file1 contents",
			},
			wantDLMeta: &DownloadMetadata{
				IsCanonical:     true,
				CanonicalSource: "../src",
				LocationType:    "local_git",
				HasVersion:      true,
				Version:         abctestutil.MinimalGitHeadSHA,
				Vars: DownloaderVars{
					GitSHA:      abctestutil.MinimalGitHeadSHA,
					GitShortSHA: abctestutil.MinimalGitHeadShortSHA,
					GitTag:      "",
				},
			},
		},
		{
			name:    "dest_dir_in_different_git_workspace",
			srcDir:  "src/dir1",
			destDir: "dst/dir1",
			initialContents: abctestutil.WithGitRepoAt("src",
				abctestutil.WithGitRepoAt("dst",
					map[string]string{
						"src/dir1/spec.yaml": "file1 contents",
						"src/dir1/file1.txt": "file1 contents",
					})),
			wantNewFiles: map[string]string{
				"dst/dir1/spec.yaml": "file1 contents",
				"dst/dir1/file1.txt": "file1 contents",
			},
			wantDLMeta: &DownloadMetadata{
				IsCanonical: false,
				Vars: DownloaderVars{
					GitSHA:      abctestutil.MinimalGitHeadSHA,
					GitShortSHA: abctestutil.MinimalGitHeadShortSHA,
					GitTag:      "",
				},
			},
		},
		{
			name:    "source_in_git_but_dest_is_not",
			srcDir:  "src/dir1",
			destDir: "dst",
			initialContents: abctestutil.WithGitRepoAt("src",
				map[string]string{
					"src/dir1/spec.yaml": "file1 contents",
					"src/dir1/file1.txt": "file1 contents",
				}),
			wantNewFiles: map[string]string{
				"dst/spec.yaml": "file1 contents",
				"dst/file1.txt": "file1 contents",
			},
			wantDLMeta: &DownloadMetadata{
				IsCanonical: false,
				Vars: DownloaderVars{
					GitSHA:      abctestutil.MinimalGitHeadSHA,
					GitShortSHA: abctestutil.MinimalGitHeadShortSHA,
					GitTag:      "",
				},
			},
		},
		{
			name:    "dest_in_git_but_src_is_not",
			srcDir:  "src",
			destDir: "dst",
			initialContents: abctestutil.WithGitRepoAt("dst",
				map[string]string{
					"src/spec.yaml": "file1 contents",
					"src/file1.txt": "file1 contents",
				}),
			wantNewFiles: map[string]string{
				"dst/spec.yaml": "file1 contents",
				"dst/file1.txt": "file1 contents",
			},
			wantDLMeta: &DownloadMetadata{
				IsCanonical: false,
			},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			tmp := t.TempDir()
			abctestutil.WriteAllDefaultMode(t, tmp, tc.initialContents)
			dl := &LocalDownloader{
				SrcPath:    filepath.Join(tmp, tc.srcDir),
				allowDirty: true,
			}
			dest := filepath.Join(tmp, tc.destDir)
			gotMeta, err := dl.Download(ctx, tmp, dest)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			got := abctestutil.LoadDirWithoutMode(t, tmp)
			want := sets.UnionMapKeys(tc.initialContents, tc.wantNewFiles)
			if diff := cmp.Diff(got, want); diff != "" {
				t.Errorf("output directory contents were not as expected (-got,+want): %s", diff)
			}

			if diff := cmp.Diff(gotMeta, tc.wantDLMeta, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("DownloadMetadata was not as expected (-got,+want): %s", diff)
			}
		})
	}
}
