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

package model

import (
	"testing"

	"github.com/abcxyz/pkg/testutil"
)

func TestIsKnownAPIVersion(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		wantErr []string
	}{
		{
			name: "v1alpha1_is_accepted",
			in:   "cli.abcxyz.dev/v1alpha1",
		},
		{
			name: "v2_is_unknown",
			in:   "cli.abcxyz.dev/v2",
			wantErr: []string{
				`field "api_version" value was "cli.abcxyz.dev/v2" but must be one of`,
				`you might need to upgrade your abc CLI. See https://github.com/abcxyz/abc/#installation`,
			},
		},
		{
			name:    "empty_rejected",
			in:      "",
			wantErr: []string{`field "api_version" value was "" but must be one of`},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := IsKnownAPIVersion(nil, String{Val: tc.in}, "api_version")
			for _, wantErr := range tc.wantErr {
				if diff := testutil.DiffErrString(got, wantErr); diff != "" {
					t.Fatal(diff)
				}
			}
		})
	}
}
