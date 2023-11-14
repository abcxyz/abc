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
	"strings"
	"testing"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
)

func TestParseSourceWithWorkingDir(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                string
		source              string
		gitProtocol         string
		tempDirContents     map[string]string
		dest                string
		want                Downloader
		wantCanonicalSource string
		wantErr             string
	}{
		{
			name:                "latest",
			source:              "github.com/myorg/myrepo@latest",
			wantCanonicalSource: "github.com/myorg/myrepo",
			want: &gitDownloader{
				canonicalSource: "github.com/myorg/myrepo",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "",
				version:         "latest",
				cloner:          &realCloner{},
				tagser:          &realTagser{},
			},
		},
		{
			name:                "given_version",
			source:              "github.com/myorg/myrepo@v1.2.3",
			wantCanonicalSource: "github.com/myorg/myrepo",
			want: &gitDownloader{
				canonicalSource: "github.com/myorg/myrepo",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "",
				version:         "v1.2.3",
				cloner:          &realCloner{},
				tagser:          &realTagser{},
			},
		},
		{
			name:                "version_with_weird_chars",
			source:              "github.com/myorg/myrepo@v1.2.3-foo/bar",
			wantCanonicalSource: "github.com/myorg/myrepo",
			want: &gitDownloader{
				canonicalSource: "github.com/myorg/myrepo",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "",
				version:         "v1.2.3-foo/bar",
				cloner:          &realCloner{},
				tagser:          &realTagser{},
			},
		},
		{
			name:                "subdir",
			source:              "github.com/myorg/myrepo/mysubdir@v1.2.3",
			wantCanonicalSource: "github.com/myorg/myrepo/mysubdir",
			want: &gitDownloader{
				canonicalSource: "github.com/myorg/myrepo/mysubdir",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "mysubdir",
				version:         "v1.2.3",
				cloner:          &realCloner{},
				tagser:          &realTagser{},
			},
		},
		{
			name:                "deep_subdir",
			source:              "github.com/myorg/myrepo/my/deep/subdir@v1.2.3",
			wantCanonicalSource: "github.com/myorg/myrepo/my/deep/subdir",
			want: &gitDownloader{
				canonicalSource: "github.com/myorg/myrepo/my/deep/subdir",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "my/deep/subdir",
				version:         "v1.2.3",
				cloner:          &realCloner{},
				tagser:          &realTagser{},
			},
		},
		{
			name:    "missing_version_with_@",
			source:  "github.com/myorg/myrepo@",
			wantErr: "isn't a valid template name",
		},
		{
			name:    "missing_version",
			source:  "github.com/myorg/myrepo",
			wantErr: "isn't a valid template name",
		},
		{
			name:   "local_absolute_dir",
			source: filepath.FromSlash("my/dir"),
			tempDirContents: map[string]string{
				"my/dir/spec.yaml": "my spec file contents",
			},
			want: &localDownloader{
				srcPath: filepath.FromSlash("my/dir"),
			},
		},
		{
			name:   "dest_dir_in_same_git_workspace",
			source: filepath.FromSlash("dir1/mytemplate"),
			tempDirContents: map[string]string{
				"dir1/mytemplate/spec.yaml": "my spec file contents",

				// A minimal .git directory
				"dir1/.git/refs/_":    "",
				"dir1/.git/objects/_": "",
				"dir1/.git/HEAD":      "ref: refs/heads/main",
			},
			dest:                "dir1/dest",
			wantCanonicalSource: filepath.FromSlash("../mytemplate"),
			want: &localDownloader{
				srcPath: filepath.FromSlash("dir1/mytemplate"),
			},
		},
		{
			name:   "dest_dir_in_different_git_workspace",
			source: filepath.FromSlash("dir1/mytemplate"),
			tempDirContents: map[string]string{
				"dir1/mytemplate/spec.yaml": "my spec file contents",

				// A minimal .git directory
				"dir1/.git/refs/_":    "",
				"dir1/.git/objects/_": "",
				"dir1/.git/HEAD":      "ref: refs/heads/main",

				// Another minimal .git directory
				"dir2/.git/refs/_":    "",
				"dir2/.git/objects/_": "",
				"dir2/.git/HEAD":      "ref: refs/heads/main",
			},
			dest: "dir2",
			want: &localDownloader{
				srcPath: filepath.FromSlash("dir1/mytemplate"),
			},
		},
		{
			name:   "source_in_git_but_dest_is_not",
			source: filepath.FromSlash("dir1/mytemplate"),
			tempDirContents: map[string]string{
				"dir1/mytemplate/spec.yaml": "my spec file contents",

				// A minimal .git directory
				"dir1/.git/refs/_":    "",
				"dir1/.git/objects/_": "",
				"dir1/.git/HEAD":      "ref: refs/heads/main",
			},
			dest: "dir2",
			want: &localDownloader{
				srcPath: filepath.FromSlash("dir1/mytemplate"),
			},
		},
		{
			name:   "dist_in_git_but_src_is_not",
			source: filepath.FromSlash("dir1/mytemplate"),
			tempDirContents: map[string]string{
				"dir1/mytemplate/spec.yaml": "my spec file contents",

				// A minimal .git directory
				"dir2/.git/refs/_":    "",
				"dir2/.git/objects/_": "",
				"dir2/.git/HEAD":      "ref: refs/heads/main",
			},
			dest: "dir2",
			want: &localDownloader{
				srcPath: filepath.FromSlash("dir1/mytemplate"),
			},
		},
		{
			name:    "https_remote_format_rejected",
			source:  "https://github.com/myorg/myrepo.git",
			wantErr: "isn't a valid template name",
		},
		{
			name:    "ssh_remote_format_rejected",
			source:  "git@github.com:myorg/myrepo.git",
			wantErr: "isn't a valid template name",
		},
		{
			name:    "nonexistent_local_dir",
			source:  "./my-dir",
			wantErr: "isn't a valid template name",
		},
		{
			name:   "dot_slash_forces_treating_as_local_dir",
			source: filepath.FromSlash("./github.com/myorg/myrepo/mysubdir@latest"),
			tempDirContents: map[string]string{
				"github.com/myorg/myrepo/mysubdir@latest/spec.yaml": "my spec file contents",
			},
			want: &localDownloader{
				srcPath: filepath.FromSlash("github.com/myorg/myrepo/mysubdir@latest"),
			},
		},
		{
			name:   "git_has_higher_precedence_than_local_dir",
			source: "github.com/myorg/myrepo/mysubdir@latest",
			tempDirContents: map[string]string{
				"github.com/myorg/myrepo/mysubdir@latest/spec.yaml": "my spec file contents",
			},
			wantCanonicalSource: "github.com/myorg/myrepo/mysubdir",
			want: &gitDownloader{
				canonicalSource: "github.com/myorg/myrepo/mysubdir",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "mysubdir",
				version:         "latest",
				cloner:          &realCloner{},
				tagser:          &realTagser{},
			},
		},
		{
			name:                "go_getter_with_ref_and_subdirs",
			source:              "github.com/myorg/myrepo.git//sub/dir?ref=latest",
			wantCanonicalSource: "github.com/myorg/myrepo/sub/dir",
			want: &gitDownloader{
				canonicalSource: "github.com/myorg/myrepo/sub/dir",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "sub/dir",
				version:         "latest",
				cloner:          &realCloner{},
				tagser:          &realTagser{},
			},
		},
		{
			name:                "go_getter_with_ref_no_subdirs",
			source:              "github.com/myorg/myrepo.git?ref=latest",
			wantCanonicalSource: "github.com/myorg/myrepo",
			want: &gitDownloader{
				canonicalSource: "github.com/myorg/myrepo",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "",
				version:         "latest",
				cloner:          &realCloner{},
				tagser:          &realTagser{},
			},
		},
		{
			name:                "go_getter_no_ref_no_subdirs",
			source:              "github.com/myorg/myrepo.git",
			wantCanonicalSource: "github.com/myorg/myrepo",
			want: &gitDownloader{
				canonicalSource: "github.com/myorg/myrepo",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "",
				version:         "latest",
				cloner:          &realCloner{},
				tagser:          &realTagser{},
			},
		},
		{
			name:                "go_getter_no_ref_with_subdirs",
			source:              "github.com/myorg/myrepo.git//sub/dir",
			wantCanonicalSource: "github.com/myorg/myrepo/sub/dir",
			want: &gitDownloader{
				canonicalSource: "github.com/myorg/myrepo/sub/dir",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "sub/dir",
				version:         "latest",
				cloner:          &realCloner{},
				tagser:          &realTagser{},
			},
		},
		{
			name:                "go_getter_no_ref_single_subdir",
			source:              "github.com/myorg/myrepo.git//subdir",
			wantCanonicalSource: "github.com/myorg/myrepo/subdir",
			want: &gitDownloader{
				canonicalSource: "github.com/myorg/myrepo/subdir",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "subdir",
				version:         "latest",
				cloner:          &realCloner{},
				tagser:          &realTagser{},
			},
		},
		{
			name:                "go_getter_semver_ref",
			source:              "github.com/myorg/myrepo.git?ref=v1.2.3",
			wantCanonicalSource: "github.com/myorg/myrepo",
			want: &gitDownloader{
				canonicalSource: "github.com/myorg/myrepo",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "",
				version:         "v1.2.3",
				cloner:          &realCloner{},
				tagser:          &realTagser{},
			},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			tempDir := t.TempDir()

			common.WriteAllDefaultMode(t, tempDir, tc.tempDirContents)

			params := &ParseSourceParams{
				Source:      tc.source,
				GitProtocol: tc.gitProtocol,
			}
			got, err := parseSourceWithCwd(ctx, tempDir, params)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}

			opts := []cmp.Option{
				cmp.AllowUnexported(gitDownloader{}, localDownloader{}),

				// The localDownloader may modify the provided source path if it was
				// relative. This comparer removes the tempDir prefix so that test cases
				// can still do relative filepath comparisons.
				cmp.Comparer(func(a, b localDownloader) bool {
					l := strings.TrimPrefix(a.srcPath, tempDir+string(filepath.Separator))
					r := strings.TrimPrefix(b.srcPath, tempDir+string(filepath.Separator))
					return l == r
				}),
			}
			if diff := cmp.Diff(got, tc.want, opts...); diff != "" {
				t.Errorf("downloader was not as expected (-got,+want): %s", diff)
			}

			// We can't continue with verifying the canonical source if there's
			// no Downloader.
			if got == nil {
				return
			}

			gotCanonicalSource, ok, err := got.CanonicalSource(ctx, tempDir, tc.dest)
			if err != nil {
				t.Fatalf("CanonicalSource() returned error: %v", err)
			}
			if gotCanonicalSource != tc.wantCanonicalSource {
				t.Errorf("got canonical source %q, want %q", gotCanonicalSource, tc.wantCanonicalSource)
			}
			if gotCanonicalSource == "" && ok {
				t.Errorf("CanonicalSource returned true but with an empty canonical source")
			}
		})
	}
}
