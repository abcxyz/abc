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

	abctestutil "github.com/abcxyz/abc/templates/testutil"
	"github.com/abcxyz/pkg/testutil"
)

func TestParseSource(t *testing.T) {
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
			want: &remoteGitDownloader{
				canonicalSource: "github.com/myorg/myrepo",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "",
				version:         "latest",
				cloner:          &realCloner{},
			},
		},
		{
			name:                "given_version",
			source:              "github.com/myorg/myrepo@v1.2.3",
			wantCanonicalSource: "github.com/myorg/myrepo",
			want: &remoteGitDownloader{
				canonicalSource: "github.com/myorg/myrepo",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "",
				version:         "v1.2.3",
				cloner:          &realCloner{},
			},
		},
		{
			name:                "version_with_weird_chars",
			source:              "github.com/myorg/myrepo@v1.2.3-foo/bar",
			wantCanonicalSource: "github.com/myorg/myrepo",
			want: &remoteGitDownloader{
				canonicalSource: "github.com/myorg/myrepo",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "",
				version:         "v1.2.3-foo/bar",
				cloner:          &realCloner{},
			},
		},
		{
			name:                "subdir",
			source:              "github.com/myorg/myrepo/mysubdir@v1.2.3",
			wantCanonicalSource: "github.com/myorg/myrepo/mysubdir",
			want: &remoteGitDownloader{
				canonicalSource: "github.com/myorg/myrepo/mysubdir",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "mysubdir",
				version:         "v1.2.3",
				cloner:          &realCloner{},
			},
		},
		{
			name:                "deep_subdir",
			source:              "github.com/myorg/myrepo/my/deep/subdir@v1.2.3",
			wantCanonicalSource: "github.com/myorg/myrepo/my/deep/subdir",
			want: &remoteGitDownloader{
				canonicalSource: "github.com/myorg/myrepo/my/deep/subdir",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "my/deep/subdir",
				version:         "v1.2.3",
				cloner:          &realCloner{},
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
			source: "my/dir",
			tempDirContents: map[string]string{
				"my/dir/spec.yaml": "my spec file contents",
			},
			want: &LocalDownloader{
				SrcPath: "my/dir",
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
			name: "spec_yaml_shouldnt_be_in_path",
			tempDirContents: map[string]string{
				"my/dir/spec.yaml": "my spec file contents",
			},
			source:  "./my/dir/spec.yaml",
			wantErr: "the template source argument should be the name of a directory",
		},
		{
			name: "filename_rejected_as_location",
			tempDirContents: map[string]string{
				"my/dir/spec.yaml":      "my spec file contents",
				"my/dir/other_file.txt": "my spec file contents",
			},
			source: "./my/dir/other_file.txt",
			// A warning will be logged too, that's not shown here.
			wantErr: "isn't a valid template name or doesn't exist",
		},

		{
			name:   "dot_slash_forces_treating_as_local_dir",
			source: "./github.com/myorg/myrepo/mysubdir@latest",
			tempDirContents: map[string]string{
				"github.com/myorg/myrepo/mysubdir@latest/spec.yaml": "my spec file contents",
			},
			want: &LocalDownloader{
				SrcPath: "github.com/myorg/myrepo/mysubdir@latest",
			},
		},
		{
			name:   "git_has_higher_precedence_than_local_dir",
			source: "github.com/myorg/myrepo/mysubdir@latest",
			tempDirContents: map[string]string{
				"github.com/myorg/myrepo/mysubdir@latest/spec.yaml": "my spec file contents",
			},
			wantCanonicalSource: "github.com/myorg/myrepo/mysubdir",
			want: &remoteGitDownloader{
				canonicalSource: "github.com/myorg/myrepo/mysubdir",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "mysubdir",
				version:         "latest",
				cloner:          &realCloner{},
			},
		},
		{
			name:                "go_getter_with_ref_and_subdirs",
			source:              "github.com/myorg/myrepo.git//sub/dir?ref=latest",
			wantCanonicalSource: "github.com/myorg/myrepo/sub/dir",
			want: &remoteGitDownloader{
				canonicalSource: "github.com/myorg/myrepo/sub/dir",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "sub/dir",
				version:         "latest",
				cloner:          &realCloner{},
			},
		},
		{
			name:                "go_getter_with_ref_no_subdirs",
			source:              "github.com/myorg/myrepo.git?ref=latest",
			wantCanonicalSource: "github.com/myorg/myrepo",
			want: &remoteGitDownloader{
				canonicalSource: "github.com/myorg/myrepo",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "",
				version:         "latest",
				cloner:          &realCloner{},
			},
		},
		{
			name:                "go_getter_no_ref_no_subdirs",
			source:              "github.com/myorg/myrepo.git",
			wantCanonicalSource: "github.com/myorg/myrepo",
			want: &remoteGitDownloader{
				canonicalSource: "github.com/myorg/myrepo",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "",
				version:         "latest",
				cloner:          &realCloner{},
			},
		},
		{
			name:                "go_getter_no_ref_with_subdirs",
			source:              "github.com/myorg/myrepo.git//sub/dir",
			wantCanonicalSource: "github.com/myorg/myrepo/sub/dir",
			want: &remoteGitDownloader{
				canonicalSource: "github.com/myorg/myrepo/sub/dir",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "sub/dir",
				version:         "latest",
				cloner:          &realCloner{},
			},
		},
		{
			name:                "go_getter_no_ref_single_subdir",
			source:              "github.com/myorg/myrepo.git//subdir",
			wantCanonicalSource: "github.com/myorg/myrepo/subdir",
			want: &remoteGitDownloader{
				canonicalSource: "github.com/myorg/myrepo/subdir",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "subdir",
				version:         "latest",
				cloner:          &realCloner{},
			},
		},
		{
			name:                "go_getter_semver_ref",
			source:              "github.com/myorg/myrepo.git?ref=v1.2.3",
			wantCanonicalSource: "github.com/myorg/myrepo",
			want: &remoteGitDownloader{
				canonicalSource: "github.com/myorg/myrepo",
				remote:          "https://github.com/myorg/myrepo.git",
				subdir:          "",
				version:         "v1.2.3",
				cloner:          &realCloner{},
			},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			tempDir := t.TempDir()

			abctestutil.WriteAll(t, tempDir, tc.tempDirContents)

			params := &ParseSourceParams{
				CWD:             tempDir,
				Source:          tc.source,
				FlagGitProtocol: tc.gitProtocol,
			}
			got, err := ParseSource(ctx, params)
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
			if diff := cmp.Diff(got, tc.want, opts...); diff != "" {
				t.Errorf("downloader was not as expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestGitCanonicalVersion(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		dir     string
		files   map[string]string
		want    string
		wantErr string
	}{
		{
			name:  "simple_success_no_tag",
			dir:   ".",
			files: abctestutil.WithGitRepoAt("", nil),
			want:  abctestutil.MinimalGitHeadSHA,
		},
		{
			name: "simple_success_with_tag",
			dir:  ".",
			files: abctestutil.WithGitRepoAt("",
				map[string]string{
					".git/refs/tags/v1.2.3": abctestutil.MinimalGitHeadSHA,
				}),
			want: "v1.2.3",
		},
		{
			name: "semver_ordering",
			dir:  ".",
			files: abctestutil.WithGitRepoAt("",
				map[string]string{
					".git/refs/tags/v4.5.6":  abctestutil.MinimalGitHeadSHA,
					".git/refs/tags/v11.0.0": abctestutil.MinimalGitHeadSHA,
					".git/refs/tags/v7.8.9":  abctestutil.MinimalGitHeadSHA,
				}),
			want: "v11.0.0",
		},
		{
			name: "only_v_prefix_counts_as_semver",
			dir:  ".",
			files: abctestutil.WithGitRepoAt("",
				map[string]string{
					".git/refs/tags/v4.5.6": abctestutil.MinimalGitHeadSHA,
					".git/refs/tags/11.0.0": abctestutil.MinimalGitHeadSHA,
					".git/refs/tags/v7.8.9": abctestutil.MinimalGitHeadSHA,
				}),
			want: "v7.8.9",
		},
		{
			name: "semver_before_non_semver",
			dir:  ".",
			files: abctestutil.WithGitRepoAt("",
				map[string]string{
					".git/refs/tags/v4.5.6":    abctestutil.MinimalGitHeadSHA,
					".git/refs/tags/zzzzzz":    abctestutil.MinimalGitHeadSHA,
					".git/refs/tags/999999":    abctestutil.MinimalGitHeadSHA,
					".git/refs/tags/v5xxx.0.0": abctestutil.MinimalGitHeadSHA,
				}),
			want: "v4.5.6",
		},
		{
			name: "non_semver_in_reverse_order",
			dir:  ".",
			files: abctestutil.WithGitRepoAt("",
				map[string]string{
					".git/refs/tags/a": abctestutil.MinimalGitHeadSHA,
					".git/refs/tags/z": abctestutil.MinimalGitHeadSHA,
					".git/refs/tags/j": abctestutil.MinimalGitHeadSHA,
				}),
			want: "z",
		},
		{
			name:  "not_a_git_repo",
			dir:   ".",
			files: nil,
		},
		{
			name: "dirty_workspace_allowed",
			dir:  ".",
			files: abctestutil.WithGitRepoAt("", map[string]string{
				"my_file.txt": "my contents",
			}),
			want: abctestutil.MinimalGitHeadSHA,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmp := t.TempDir()
			abctestutil.WriteAll(t, tmp, tc.files)
			ctx := context.Background()
			got, gotOK, err := gitCanonicalVersion(ctx, filepath.Join(tmp, tc.dir))
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if (got == "") == gotOK {
				t.Errorf("ok should be true if and only if the version is non-empty")
			}

			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
