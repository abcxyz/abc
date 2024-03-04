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
	"github.com/abcxyz/pkg/testutil"
)

func TestLocalDownloader_Download(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                     string
		copyFromDir              string
		destDirForCanonicalCheck string // not actually created or touched, treated as hypothetical render output dir when checking for canonical-ness.
		initialTempDirContents   map[string]string
		wantTemplateDirFiles     map[string]string
		wantDLMeta               *DownloadMetadata
		wantErr                  string
	}{
		{
			name:        "simple_success",
			copyFromDir: "copy_from",
			// copyToDir:                "copy_to",
			destDirForCanonicalCheck: "dest",
			initialTempDirContents: map[string]string{
				"copy_from/file1.txt":   "file1 contents",
				"copy_from/a/file2.txt": "file2 contents",
			},
			wantTemplateDirFiles: map[string]string{
				"file1.txt":   "file1 contents",
				"a/file2.txt": "file2 contents",
			},
			wantDLMeta: &DownloadMetadata{
				IsCanonical: false,
			},
		},
		{
			name:        "nonexistent_source",
			copyFromDir: "nonexistent",
			wantErr:     "nonexistent",
		},
		{
			name:                     "dest_dir_in_same_git_workspace",
			copyFromDir:              "copy_from",
			destDirForCanonicalCheck: "dest",
			initialTempDirContents: abctestutil.WithGitRepoAt("",
				map[string]string{
					"copy_from/spec.yaml": "spec contents",
					"copy_from/file1.txt": "file1 contents",
				}),
			wantTemplateDirFiles: map[string]string{
				"spec.yaml": "spec contents",
				"file1.txt": "file1 contents",
			},
			wantDLMeta: &DownloadMetadata{
				IsCanonical:     true,
				CanonicalSource: "../copy_from",
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
			name:                     "dest_dir_in_same_git_workspace_with_tag",
			copyFromDir:              "copy_from",
			destDirForCanonicalCheck: "dest",
			initialTempDirContents: abctestutil.WithGitRepoAt("",
				map[string]string{
					"copy_from/spec.yaml": "spec contents",
					"copy_from/file1.txt": "file1 contents",

					// This assumes that we're using the git repo created by
					// common.WithGitRepoAt(). We're tweaking the repo structure
					// to add a tag. The named SHA already exists in the repo.
					".git/refs/tags/mytag": abctestutil.MinimalGitHeadSHA,
				}),
			wantTemplateDirFiles: map[string]string{
				"spec.yaml": "spec contents",
				"file1.txt": "file1 contents",
			},
			wantDLMeta: &DownloadMetadata{
				IsCanonical:     true,
				CanonicalSource: "../copy_from",
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
			name:                     "dest_dir_in_same_git_workspace_with_detached_head",
			copyFromDir:              "copy_from",
			destDirForCanonicalCheck: "dest",
			initialTempDirContents: abctestutil.WithGitRepoAt("",
				map[string]string{
					"copy_from/spec.yaml": "spec contents",
					"copy_from/file1.txt": "file1 contents",

					// This assumes that we're using the git repo created by
					// common.WithGitRepoAt(). We're putting the repo in a
					// detached HEAD state so we're not on a branch. The reason
					// this creates a detached head state is because .git/HEAD
					// would normally contain a branch name, but when you put a
					// SHA in there in means you have a detached head.
					".git/HEAD": abctestutil.MinimalGitHeadSHA,
				}),
			wantTemplateDirFiles: map[string]string{
				"spec.yaml": "spec contents",
				"file1.txt": "file1 contents",
			},
			wantDLMeta: &DownloadMetadata{
				IsCanonical:     true,
				CanonicalSource: "../copy_from",
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
			name:                     "dest_dir_in_different_git_workspace",
			copyFromDir:              "copy_from",
			destDirForCanonicalCheck: "dest",
			// There are two separate git workspaces: one rooted at "dest", and
			// one at "copy_from". The template location should not be seen as
			// canonical because we're copying across the boundary of the git
			// workspace.
			initialTempDirContents: abctestutil.WithGitRepoAt("dest",
				abctestutil.WithGitRepoAt("copy_from",
					map[string]string{
						"copy_from/spec.yaml": "spec contents",
						"copy_from/file1.txt": "file1 contents",
					})),
			wantTemplateDirFiles: abctestutil.WithGitRepoAt("", map[string]string{
				"spec.yaml": "spec contents",
				"file1.txt": "file1 contents",
			}),
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
			name:                     "source_in_git_but_dest_is_not",
			copyFromDir:              "copy_from",
			destDirForCanonicalCheck: "dest",
			initialTempDirContents: abctestutil.WithGitRepoAt("copy_from",
				map[string]string{
					"copy_from/spec.yaml": "spec contents",
					"copy_from/file1.txt": "file1 contents",
				}),
			wantTemplateDirFiles: abctestutil.WithGitRepoAt("", map[string]string{
				"spec.yaml": "spec contents",
				"file1.txt": "file1 contents",
			}),
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
			name:        "source_is_not_in_git_but_dest_is",
			copyFromDir: "copy_from",
			initialTempDirContents: abctestutil.WithGitRepoAt("dest",
				map[string]string{
					"copy_from/spec.yaml": "spec contents",
					"copy_from/file1.txt": "file1 contents",
				}),
			wantTemplateDirFiles: map[string]string{
				"spec.yaml": "spec contents",
				"file1.txt": "file1 contents",
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
			abctestutil.WriteAllDefaultMode(t, tmp, tc.initialTempDirContents)
			dl := &LocalDownloader{
				SrcPath:            filepath.Join(tmp, tc.copyFromDir),
				allowDirtyTestOnly: true,
			}
			dest := filepath.Join(tmp, tc.destDirForCanonicalCheck)
			templateDir := t.TempDir()
			gotMeta, err := dl.Download(ctx, tmp, templateDir, dest)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}

			gotTemplateDir := abctestutil.LoadDirWithoutMode(t, templateDir)
			if diff := cmp.Diff(gotTemplateDir, tc.wantTemplateDirFiles, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("template directory contents were not as expected (-got,+want): %s", diff)
			}

			if diff := cmp.Diff(gotMeta, tc.wantDLMeta, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("DownloadMetadata was not as expected (-got,+want): %s", diff)
			}
		})
	}
}
