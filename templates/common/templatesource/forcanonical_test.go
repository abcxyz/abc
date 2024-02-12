// Copyright 2024 The Authors (see AUTHORS file)
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
	"strings"
	"testing"

	abctestutil "github.com/abcxyz/abc/templates/testutil"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
)

func TestForCanonical(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                     string
		location                 string
		locType                  string
		gitProtocol              string
		destDir                  string
		prependTempDirToLocation bool // set this for tests where location is a local path
		dirContents              map[string]string
		wantDownloader           Downloader
		wantErr                  string
	}{
		{
			name:        "remote_git_https_no_subdir",
			location:    "github.com/abcxyz/abc",
			locType:     "remote_git",
			gitProtocol: "https",
			wantDownloader: &remoteGitDownloader{
				canonicalSource: "github.com/abcxyz/abc",
				cloner:          &realCloner{},
				remote:          "https://github.com/abcxyz/abc.git",
				subdir:          "",
				tagser:          &realTagser{},
				version:         "latest",
			},
		},
		{
			name:        "remote_git_ssh_no_subdir",
			location:    "github.com/abcxyz/abc",
			locType:     "remote_git",
			gitProtocol: "ssh",
			wantDownloader: &remoteGitDownloader{
				canonicalSource: "github.com/abcxyz/abc",
				cloner:          &realCloner{},
				remote:          "git@github.com:abcxyz/abc.git",
				subdir:          "",
				tagser:          &realTagser{},
				version:         "latest",
			},
		},
		{
			name:        "remote_git_https_subdir",
			location:    "github.com/abcxyz/abc/sub",
			locType:     "remote_git",
			gitProtocol: "https",
			wantDownloader: &remoteGitDownloader{
				canonicalSource: "github.com/abcxyz/abc/sub",
				cloner:          &realCloner{},
				remote:          "https://github.com/abcxyz/abc.git",
				subdir:          "sub",
				tagser:          &realTagser{},
				version:         "latest",
			},
		},
		{
			name:        "remote_git_ssh_subdir",
			location:    "github.com/abcxyz/abc/sub",
			locType:     "remote_git",
			gitProtocol: "ssh",
			wantDownloader: &remoteGitDownloader{
				canonicalSource: "github.com/abcxyz/abc/sub",
				cloner:          &realCloner{},
				remote:          "git@github.com:abcxyz/abc.git",
				subdir:          "sub",
				tagser:          &realTagser{},
				version:         "latest",
			},
		},
		{
			name:        "malformed_remote_git",
			location:    "asdfasdfasdf",
			locType:     "remote_git",
			gitProtocol: "https",
			wantErr:     `failed parsing canonical location "asdfasdfasdf"`,
		},
		{
			name:                     "local_dir_no_git_repo",
			location:                 "./my/dir",
			locType:                  "local_git",
			prependTempDirToLocation: true,
			wantErr:                  `/my/dir" is not in a git workspace`,
		},
		{
			name:                     "simple_local_git_repo",
			location:                 "./my/dir",
			locType:                  "local_git",
			prependTempDirToLocation: true,
			dirContents:              abctestutil.WithGitRepoAt("", nil),
			wantDownloader: &LocalDownloader{
				SrcPath: "my/dir",
			},
		},
		{
			name:                     "different_git_workspaces",
			location:                 "git1",
			locType:                  "local_git",
			destDir:                  "git2",
			prependTempDirToLocation: true,
			dirContents: abctestutil.WithGitRepoAt("git1",
				abctestutil.WithGitRepoAt("git2", nil)),
			wantErr: "must be in the same git workspace",
		},
		{
			name:        "unknown_loc_type",
			locType:     "nonexistent",
			location:    "asdf",
			gitProtocol: "https",
			wantErr:     `unknown location type "nonexistent"`,
		},
		{
			name:        "unknown_git_protocol",
			location:    "github.com/abcxyz/abc",
			locType:     "remote_git",
			gitProtocol: "nonexistent",
			wantErr:     `protocol "nonexistent" isn't usable with a template sourced from a remote git repo`,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			tempDir := t.TempDir()

			abctestutil.WriteAllDefaultMode(t, tempDir, tc.dirContents)

			location := tc.location
			if tc.prependTempDirToLocation {
				location = filepath.Join(tempDir, location)
			}

			destDir := filepath.Join(tempDir, tc.destDir)

			downloader, err := ForCanonical(ctx, location, tc.locType, tc.gitProtocol, destDir)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}

			opts := []cmp.Option{
				cmp.AllowUnexported(remoteGitDownloader{}, LocalDownloader{}),

				// If the downloader is a local downloader, it has an
				// unpredictable temp directory in its SrcPath field that
				// couldn't be included in wantDownloader. Therefore, when
				// comparing the "got" downloader to the "want" downloader, we
				// want to strip out the temp dir.
				cmp.Transformer("strip_temp_dir", func(l *LocalDownloader) *LocalDownloader {
					cp := *l
					cp.SrcPath = strings.TrimPrefix(cp.SrcPath, tempDir+"/")
					return &cp
				}),
			}
			if diff := cmp.Diff(downloader, tc.wantDownloader, opts...); diff != "" {
				t.Errorf("downloader was not as expected: %s", diff)
			}
		})
	}
}
