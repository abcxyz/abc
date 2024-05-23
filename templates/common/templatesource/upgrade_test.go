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
	"testing"

	"github.com/google/go-cmp/cmp"

	abctestutil "github.com/abcxyz/abc/templates/testutil"
	"github.com/abcxyz/pkg/testutil"
)

func TestForUpgrade(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		canonicalLocation string
		locType           LocationType
		gitProtocol       string
		installedInSubdir string
		dirContents       map[string]string
		version           string
		wantDownloader    Downloader
		wantErr           string
	}{
		{
			name:              "remote_git_https_no_subdir",
			canonicalLocation: "github.com/abcxyz/abc",
			locType:           RemoteGit,
			gitProtocol:       "https",
			version:           "latest",
			wantDownloader: &remoteGitDownloader{
				canonicalSource: "github.com/abcxyz/abc",
				cloner:          &realCloner{},
				remote:          "https://github.com/abcxyz/abc.git",
				tagser:          &realTagser{},
				version:         "latest",
			},
		},
		{
			name:              "remote_git_ssh_no_subdir",
			canonicalLocation: "github.com/abcxyz/abc",
			locType:           RemoteGit,
			gitProtocol:       "ssh",
			version:           "latest",
			wantDownloader: &remoteGitDownloader{
				canonicalSource: "github.com/abcxyz/abc",
				cloner:          &realCloner{},
				remote:          "git@github.com:abcxyz/abc.git",
				tagser:          &realTagser{},
				version:         "latest",
			},
		},
		{
			name:              "remote_git_https_subdir",
			canonicalLocation: "github.com/abcxyz/abc/sub",
			locType:           RemoteGit,
			gitProtocol:       "https",
			version:           "latest",
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
			name:              "remote_git_ssh_subdir",
			canonicalLocation: "github.com/abcxyz/abc/sub",
			locType:           RemoteGit,
			gitProtocol:       "ssh",
			version:           "latest",
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
			name:              "non_default_version",
			canonicalLocation: "github.com/abcxyz/abc",
			locType:           RemoteGit,
			gitProtocol:       "https",
			version:           "someversion",
			wantDownloader: &remoteGitDownloader{
				canonicalSource: "github.com/abcxyz/abc",
				cloner:          &realCloner{},
				remote:          "https://github.com/abcxyz/abc.git",
				tagser:          &realTagser{},
				version:         "someversion",
			},
		},
		{
			name:              "malformed_remote_git",
			canonicalLocation: "asdfasdfasdf",
			locType:           RemoteGit,
			gitProtocol:       "https",
			wantErr:           `failed parsing canonical location "asdfasdfasdf"`,
		},
		{
			name:              "local_dir_no_git_repo",
			canonicalLocation: "my/dir",
			locType:           "local_git",
			wantErr:           `my/dir" is not in a git workspace`,
		},
		{
			name:              "simple_local_git_repo",
			canonicalLocation: "my/dir",
			locType:           "local_git",
			dirContents:       abctestutil.WithGitRepoAt("", nil),
			wantDownloader: &LocalDownloader{
				SrcPath: "my/dir",
			},
		},
		{
			name:              "different_git_workspaces",
			canonicalLocation: "../template_dir",
			locType:           "local_git",
			installedInSubdir: "installed_dir",
			dirContents: abctestutil.WithGitRepoAt("template_dir",
				abctestutil.WithGitRepoAt("installed_dir", nil)),
			wantErr: "must be in the same git workspace",
		},
		{
			name:              "unknown_loc_type",
			locType:           "nonexistent",
			canonicalLocation: "asdf",
			gitProtocol:       "https",
			wantErr:           `unknown location type "nonexistent"`,
		},
		{
			name:              "unknown_git_protocol",
			canonicalLocation: "github.com/abcxyz/abc",
			locType:           RemoteGit,
			gitProtocol:       "nonexistent",
			wantErr:           `protocol "nonexistent" isn't usable with a template sourced from a remote git repo`,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			tempDir := t.TempDir()

			abctestutil.WriteAll(t, tempDir, tc.dirContents)

			location := tc.canonicalLocation

			installedInDir := filepath.Join(tempDir, tc.installedInSubdir)

			downloader, err := ForUpgrade(ctx, &ForUpgradeParams{
				LocType:           tc.locType,
				CanonicalLocation: location,
				InstalledDir:      installedInDir,
				GitProtocol:       tc.gitProtocol,
				Version:           tc.version,
			})
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}

			opts := []cmp.Option{
				cmp.AllowUnexported(remoteGitDownloader{}, LocalDownloader{}),
				abctestutil.TransformStructFields(
					abctestutil.TrimStringPrefixTransformer(tempDir+"/"),
					LocalDownloader{},
					"SrcPath",
				),
			}
			if diff := cmp.Diff(downloader, tc.wantDownloader, opts...); diff != "" {
				t.Errorf("downloader was not as expected: %s", diff)
			}
		})
	}
}
