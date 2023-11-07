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
	"os"
	"path/filepath"
	"testing"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
)

func TestParseSource(t *testing.T) { //nolint:paralleltest
	// We can't use t.Parallel() here because we use os.Chdir. We don't want
	// multiple goroutines messing with the working directory at the same time.
	// The test is fast enough that the peformance impact is negligible.

	cases := []struct {
		name            string
		in              string
		protocol        string
		tempDirContents map[string]string
		want            Downloader
		wantErr         string
	}{
		{
			name: "latest",
			in:   "github.com/myorg/myrepo@latest",
			want: &gitDownloader{
				remote:  "https://github.com/myorg/myrepo.git",
				subdir:  "",
				version: "latest",
				cloner:  &realCloner{},
				tagser:  &realTagser{},
			},
		},
		{
			name: "given_version",
			in:   "github.com/myorg/myrepo@v1.2.3",
			want: &gitDownloader{
				remote:  "https://github.com/myorg/myrepo.git",
				subdir:  "",
				version: "v1.2.3",
				cloner:  &realCloner{},
				tagser:  &realTagser{},
			},
		},
		{
			name: "version_with_weird_chars",
			in:   "github.com/myorg/myrepo@v1.2.3-foo/bar",
			want: &gitDownloader{
				remote:  "https://github.com/myorg/myrepo.git",
				subdir:  "",
				version: "v1.2.3-foo/bar",
				cloner:  &realCloner{},
				tagser:  &realTagser{},
			},
		},
		{
			name: "subdir",
			in:   "github.com/myorg/myrepo/mysubdir@v1.2.3",
			want: &gitDownloader{
				remote:  "https://github.com/myorg/myrepo.git",
				subdir:  "mysubdir",
				version: "v1.2.3",
				cloner:  &realCloner{},
				tagser:  &realTagser{},
			},
		},
		{
			name: "deep_subdir",
			in:   "github.com/myorg/myrepo/my/deep/subdir@v1.2.3",
			want: &gitDownloader{
				remote:  "https://github.com/myorg/myrepo.git",
				subdir:  "my/deep/subdir",
				version: "v1.2.3",
				cloner:  &realCloner{},
				tagser:  &realTagser{},
			},
		},
		{
			name:    "missing_version_with_@",
			in:      "github.com/myorg/myrepo@",
			wantErr: "isn't a valid template name",
		},
		{
			name:    "missing_version",
			in:      "github.com/myorg/myrepo",
			wantErr: "isn't a valid template name",
		},
		{
			name: "local_absolute_dir",
			in:   filepath.FromSlash("my/dir"),
			tempDirContents: map[string]string{
				"my/dir/spec.yaml": "my spec file contents",
			},
			want: &localDownloader{
				srcPath: filepath.FromSlash("my/dir"),
			},
		},
		{
			name:    "https_remote_format_rejected",
			in:      "https://github.com/myorg/myrepo.git",
			wantErr: "isn't a valid template name",
		},
		{
			name:    "ssh_remote_format_rejected",
			in:      "git@github.com:myorg/myrepo.git",
			wantErr: "isn't a valid template name",
		},
		{
			name:    "nonexistent_local_dir",
			in:      "./my-dir",
			wantErr: "isn't a valid template name",
		},
		{
			name: "dot_slash_forces_treating_as_local_dir",
			in:   filepath.FromSlash("./github.com/myorg/myrepo/mysubdir@latest"),
			tempDirContents: map[string]string{
				"github.com/myorg/myrepo/mysubdir@latest/spec.yaml": "my spec file contents",
			},
			want: &localDownloader{
				srcPath: filepath.FromSlash("./github.com/myorg/myrepo/mysubdir@latest"),
			},
		},
		{
			name: "git_has_higher_precedence_than_local_dir",
			in:   "github.com/myorg/myrepo/mysubdir@latest",
			tempDirContents: map[string]string{
				"github.com/myorg/myrepo/mysubdir@latest/spec.yaml": "my spec file contents",
			},
			want: &gitDownloader{
				remote:  "https://github.com/myorg/myrepo.git",
				subdir:  "mysubdir",
				version: "latest",
				cloner:  &realCloner{},
				tagser:  &realTagser{},
			},
		},
		{
			name: "go_getter_with_ref_and_subdirs",
			in:   "github.com/myorg/myrepo.git//sub/dir?ref=latest",
			want: &gitDownloader{
				remote:  "https://github.com/myorg/myrepo.git",
				subdir:  "sub/dir",
				version: "latest",
				cloner:  &realCloner{},
				tagser:  &realTagser{},
			},
		},
		{
			name: "go_getter_with_ref_no_subdirs",
			in:   "github.com/myorg/myrepo.git?ref=latest",
			want: &gitDownloader{
				remote:  "https://github.com/myorg/myrepo.git",
				subdir:  "",
				version: "latest",
				cloner:  &realCloner{},
				tagser:  &realTagser{},
			},
		},
		{
			name: "go_getter_no_ref_no_subdirs",
			in:   "github.com/myorg/myrepo.git",
			want: &gitDownloader{
				remote:  "https://github.com/myorg/myrepo.git",
				subdir:  "",
				version: "latest",
				cloner:  &realCloner{},
				tagser:  &realTagser{},
			},
		},
		{
			name: "go_getter_no_ref_with_subdirs",
			in:   "github.com/myorg/myrepo.git//sub/dir",
			want: &gitDownloader{
				remote:  "https://github.com/myorg/myrepo.git",
				subdir:  "sub/dir",
				version: "latest",
				cloner:  &realCloner{},
				tagser:  &realTagser{},
			},
		},
		{
			name: "go_getter_no_ref_single_subdir",
			in:   "github.com/myorg/myrepo.git//subdir",
			want: &gitDownloader{
				remote:  "https://github.com/myorg/myrepo.git",
				subdir:  "subdir",
				version: "latest",
				cloner:  &realCloner{},
				tagser:  &realTagser{},
			},
		},
		{
			name: "go_getter_semver_ref",
			in:   "github.com/myorg/myrepo.git?ref=v1.2.3",
			want: &gitDownloader{
				remote:  "https://github.com/myorg/myrepo.git",
				subdir:  "",
				version: "v1.2.3",
				cloner:  &realCloner{},
				tagser:  &realTagser{},
			},
		},
	}

	for _, tc := range cases { //nolint:paralleltest
		// We can't use t.Parallel() here because we use os.Chdir. We don't
		// want multiple goroutines messing with the working directory at
		// the same time. The test is fast enough that the peformance impact
		// is negligible.

		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			tempDir := t.TempDir()

			// cd into the temp dir, and restore the previous working dir when
			// done.
			oldWD, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(tempDir); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := os.Chdir(oldWD); err != nil {
					t.Fatal(err)
				}
			}()

			common.WriteAllDefaultMode(t, tempDir, tc.tempDirContents)

			dl, err := ParseSource(ctx, tc.in, tc.protocol)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}

			opt := cmp.AllowUnexported(gitDownloader{}, localDownloader{})
			if diff := cmp.Diff(dl, tc.want, opt); diff != "" {
				t.Errorf("downloader was not as expected (-got,+want): %s", diff)
			}
		})
	}
}
