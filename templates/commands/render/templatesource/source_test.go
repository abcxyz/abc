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

	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
)

func TestParseSource(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		in       string
		protocol string
		want     templateDownloader
		wantErr  string
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
			wantErr: "isn't something that we know how to download",
		},
		{
			name:    "missing_version",
			in:      "github.com/myorg/myrepo",
			wantErr: "isn't something that we know how to download",
		},
		{
			name:    "local_relative_dir_not_supported_yet",
			in:      "my/dir",
			wantErr: "isn't something that we know how to download",
		},
		{
			name:    "local_absolute_dir_not_supported_yet",
			in:      "/my/dir",
			wantErr: "isn't something that we know how to download",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			dl, err := parseSource(ctx, realSourceParsers, tc.in, tc.protocol)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}
			if diff := cmp.Diff(dl, tc.want, cmp.AllowUnexported(gitDownloader{})); diff != "" {
				t.Errorf("templateDownloader was not as expected (-got,+want): %s", diff)
			}
		})
	}
}
