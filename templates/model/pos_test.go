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
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestPosErrorf(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		pos     *ConfigPos
		fmtStr  string
		args    []any
		wantErr string // comparison is exact
	}{
		{
			name:    "happy_path",
			pos:     &ConfigPos{10, 11},
			fmtStr:  "Oh no! some number %d: %w",
			args:    []any{345, fmt.Errorf("wrapped error")},
			wantErr: "at line 10 column 11: Oh no! some number 345: wrapped error",
		},
		{
			name:    "nil_position",
			pos:     nil,
			fmtStr:  "foo(): %w",
			args:    []any{fmt.Errorf("wrapped error")},
			wantErr: "foo(): wrapped error",
		},
		{
			name:    "zero_position",
			pos:     &ConfigPos{},
			fmtStr:  "foo(): %w",
			args:    []any{fmt.Errorf("wrapped error")},
			wantErr: "foo(): wrapped error",
		},
		{
			name:    "no_args",
			pos:     &ConfigPos{10, 11},
			fmtStr:  "abc def",
			args:    nil,
			wantErr: "at line 10 column 11: abc def",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.pos.Errorf(tc.fmtStr, tc.args...)
			if diff := cmp.Diff(got.Error(), tc.wantErr); diff != "" {
				t.Error(diff)
			}
		})
	}
}
