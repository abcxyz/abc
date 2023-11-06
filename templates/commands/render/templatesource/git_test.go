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

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
)

func TestGitDownloader_Download(t *testing.T) {
	t.Parallel()

	// Most subtests can use this simple set of files.
	basicFiles := map[string]string{
		"file1.txt":     "hello",
		"dir/file2.txt": "world",
	}

	cases := []struct {
		name    string
		dl      *gitDownloader
		want    map[string]string
		wantErr string
	}{
		{
			name: "no_subdir",
			dl: &gitDownloader{
				remote:  "fake-remote",
				subdir:  "",
				version: "v1.2.3",
				cloner: &fakeCloner{
					t:               t,
					out:             basicFiles,
					wantRemote:      "fake-remote",
					wantBranchOrTag: "v1.2.3",
				},
			},
			want: basicFiles,
		},
		{
			name: "latest_version_lookup",
			dl: &gitDownloader{
				remote:  "fake-remote",
				subdir:  "",
				version: "latest",
				cloner: &fakeCloner{
					t:               t,
					out:             basicFiles,
					wantRemote:      "fake-remote",
					wantBranchOrTag: "v1.2.3",
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
		},
		{
			name: "with_subdir",
			dl: &gitDownloader{
				remote:  "fake-remote",
				subdir:  "my-subdir",
				version: "v1.2.3",
				cloner: &fakeCloner{
					t: t,
					out: map[string]string{
						"my-subdir/file1.txt": "hello",
						"file2.txt":           "world",
					},
					wantRemote:      "fake-remote",
					wantBranchOrTag: "v1.2.3",
				},
			},
			want: map[string]string{
				"file1.txt": "hello",
			},
		},
		{
			name: "with_deep_subdir",
			dl: &gitDownloader{
				remote:  "fake-remote",
				subdir:  "my/deep",
				version: "v1.2.3",
				cloner: &fakeCloner{
					t: t,
					out: map[string]string{
						"my/deep/subdir/file1.txt": "hello",
						"file2.txt":                "world",
					},
					wantRemote:      "fake-remote",
					wantBranchOrTag: "v1.2.3",
				},
			},
			want: map[string]string{
				"subdir/file1.txt": "hello",
			},
		},
		{
			name: "dot_dot_subdir",
			dl: &gitDownloader{
				remote:  "fake-remote",
				subdir:  "..",
				version: "v1.2.3",
			},
			wantErr: `must not contain ".."`,
			want:    map[string]string{},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			tempDir := t.TempDir()
			err := tc.dl.Download(ctx, tempDir)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}
			got := common.LoadDirWithoutMode(t, tempDir)
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("output files were not as expected (-got, +want): %s", diff)
			}
		})
	}
}

func TestResolveBranchOrTag(t *testing.T) {
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
			name:    "version_without_v_prefix_rejected",
			in:      "1.2.3",
			wantErr: `must start with "v"`,
		},
		{
			name:    "empty_input",
			in:      "",
			wantErr: `must start with "v"`,
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
			name:    "malformed_version_rejected",
			in:      "v👍.😀.🎉",
			wantErr: "not a valid format",
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

			got, err := resolveBranchOrTag(ctx, tc.tagser, tc.inRemote, tc.in)
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
	t               *testing.T
	out             map[string]string
	wantRemote      string
	wantBranchOrTag string
}

func (f *fakeCloner) Clone(ctx context.Context, remote, branchOrTag, outDir string) error {
	if remote != f.wantRemote {
		f.t.Errorf("got remote %q, want %q", remote, f.wantRemote)
	}
	if branchOrTag != f.wantBranchOrTag {
		f.t.Errorf("got branchOrTag %q, want %q", branchOrTag, f.wantBranchOrTag)
	}

	common.WriteAllDefaultMode(f.t, outDir, f.out)
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
