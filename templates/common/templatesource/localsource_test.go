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

	"github.com/abcxyz/abc/templates/common"
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
			wantErr: "no such file",
		},
		{
			name:    "dest_dir_in_same_git_workspace",
			srcDir:  "src",
			destDir: "dst",
			initialContents: map[string]string{
				// A minimal .git directory
				".git/refs/_":    "",
				".git/objects/_": "",
				".git/HEAD":      "ref: refs/heads/main",
				"src/spec.yaml":  "file1 contents",
				"src/file1.txt":  "file1 contents",
			},
			wantNewFiles: map[string]string{
				"dst/spec.yaml": "file1 contents",
				"dst/file1.txt": "file1 contents",
			},
			wantDLMeta: &DownloadMetadata{
				IsCanonical:     true,
				CanonicalSource: "../src",
			},
		},
		{
			name:    "dest_dir_in_different_git_workspace",
			srcDir:  "src/dir1",
			destDir: "dst/dir1",
			initialContents: map[string]string{
				"src/.git/refs/_":    "",
				"src/.git/objects/_": "",
				"src/.git/HEAD":      "ref: refs/heads/main",
				"src/dir1/spec.yaml": "file1 contents",
				"src/dir1/file1.txt": "file1 contents",

				"dst/.git/refs/_":    "",
				"dst/.git/objects/_": "",
				"dst/.git/HEAD":      "ref: refs/heads/main",
			},
			wantNewFiles: map[string]string{
				"dst/dir1/spec.yaml": "file1 contents",
				"dst/dir1/file1.txt": "file1 contents",
			},
			wantDLMeta: &DownloadMetadata{
				IsCanonical: false,
			},
		},
		{
			name:    "source_in_git_but_dest_is_not",
			srcDir:  "src/dir1",
			destDir: "dst",
			initialContents: map[string]string{
				"src/.git/refs/_":    "",
				"src/.git/objects/_": "",
				"src/.git/HEAD":      "ref: refs/heads/main",
				"src/dir1/spec.yaml": "file1 contents",
				"src/dir1/file1.txt": "file1 contents",
			},
			wantNewFiles: map[string]string{
				"dst/spec.yaml": "file1 contents",
				"dst/file1.txt": "file1 contents",
			},
			wantDLMeta: &DownloadMetadata{
				IsCanonical: false,
			},
		},
		{
			name:    "dest_in_git_but_src_is_not",
			srcDir:  "src",
			destDir: "dst",
			initialContents: map[string]string{
				"src/spec.yaml": "file1 contents",
				"src/file1.txt": "file1 contents",

				"dst/.git/refs/_":    "",
				"dst/.git/objects/_": "",
				"dst/.git/HEAD":      "ref: refs/heads/main",
			},
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
			common.WriteAllDefaultMode(t, tmp, tc.initialContents)
			dl := &localDownloader{
				srcPath: filepath.Join(tmp, filepath.FromSlash(tc.srcDir)),
			}
			dest := filepath.Join(tmp, filepath.FromSlash(tc.destDir))
			gotMeta, err := dl.Download(ctx, tmp, dest)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			got := common.LoadDirWithoutMode(t, tmp)
			want := sets.UnionMapKeys(tc.initialContents, tc.wantNewFiles)
			if diff := cmp.Diff(got, want); diff != "" {
				t.Errorf("output directory contents were not as expected: %s", diff)
			}

			if diff := cmp.Diff(gotMeta, tc.wantDLMeta, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("DownloadMetadata was not as expected (-got,+want): %s", diff)
			}
		})
	}
}
