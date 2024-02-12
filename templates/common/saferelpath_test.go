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

package common

import (
	"testing"

	"github.com/abcxyz/pkg/testutil"
)

func TestSafeRelPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		want    string
		wantErr string
	}{
		{
			name: "plain_filename_succeeds",
			in:   "a.txt",
			want: "a.txt",
		},
		{
			name: "path_with_directories_succeeds",
			in:   "a/b.txt",
			want: "a/b.txt",
		},
		{
			name: "trailing_slash_succeeds",
			in:   "a/b/",
			want: "a/b/",
		},
		{
			name: "leading_slash_stripped",
			in:   "/a",
			want: "a",
		},
		{
			name: "leading_slash_with_more_dirs",
			in:   "/a/b/c",
			want: "a/b/c",
		},
		{
			name: "plain_slash_stripped",
			in:   "/",
			want: "",
		},
		{
			name:    "leading_dot_dot_fails",
			in:      "../a.txt",
			wantErr: "..",
		},
		{
			name:    "leading_dot_dot_with_more_dirs_fails",
			in:      "../a/b/c.txt",
			wantErr: "..",
		},
		{
			name:    "dot_dot_in_the_middle_fails",
			in:      "a/b/../c.txt",
			wantErr: "..",
		},
		{
			name:    "plain_dot_dot_fails",
			in:      "..",
			wantErr: "..",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := SafeRelPath(nil, tc.in)
			if got != tc.want {
				t.Errorf("SafeRelPath(%s): expected %q to be %q", tc.in, got, tc.want)
			}
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}
		})
	}
}
