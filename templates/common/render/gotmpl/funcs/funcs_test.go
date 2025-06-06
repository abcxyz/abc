// Copyright 2023 The Authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package funcs

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/exp/slices"

	"github.com/abcxyz/pkg/testutil"
)

func TestToSnakeCase(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		input     string
		want      string
		wantLower string
		wantUpper string
	}{
		{
			name:      "success",
			input:     "this-IS A test-123",
			want:      "this_IS_A_test_123",
			wantLower: "this_is_a_test_123",
			wantUpper: "THIS_IS_A_TEST_123",
		},
		{
			name:      "removes_special_characters",
			input:     "!@#$%^&*()+=,.<>\n\r\t/?'\"[{]}\\|`~;:`]",
			want:      "",
			wantLower: "",
			wantUpper: "",
		},
		{
			name:      "handle_empty",
			input:     "",
			want:      "",
			wantLower: "",
			wantUpper: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := toSnakeCase(tc.input)
			if got, want := got, tc.want; got != want {
				t.Errorf("expected %s to be %s", got, want)
			}

			got = toLowerSnakeCase(tc.input)
			if got, want := got, tc.wantLower; got != want {
				t.Errorf("expected lower %s to be %s", got, want)
			}

			got = toUpperSnakeCase(tc.input)
			if got, want := got, tc.wantUpper; got != want {
				t.Errorf("expected upper %s to be %s", got, want)
			}
		})
	}
}

func TestToHyphenCase(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		input     string
		want      string
		wantLower string
		wantUpper string
	}{
		{
			name:      "success",
			input:     "this_IS_A_test_123",
			want:      "this-IS-A-test-123",
			wantLower: "this-is-a-test-123",
			wantUpper: "THIS-IS-A-TEST-123",
		},
		{
			name:      "handles_empty",
			input:     "",
			want:      "",
			wantLower: "",
			wantUpper: "",
		},
		{
			name:      "removes_special_characters",
			input:     "!@#$%^&*()+=,.<>\n\r\t/?'\"[{]}\\|`~;:`]",
			want:      "",
			wantLower: "",
			wantUpper: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := toHyphenCase(tc.input)
			if got, want := got, tc.want; got != want {
				t.Errorf("expected %s to be %s", got, want)
			}

			got = toLowerHyphenCase(tc.input)
			if got, want := got, tc.wantLower; got != want {
				t.Errorf("expected lower %s to be %s", got, want)
			}

			got = toUpperHyphenCase(tc.input)
			if got, want := got, tc.wantUpper; got != want {
				t.Errorf("expected upper %s to be %s", got, want)
			}
		})
	}
}

func TestSortStrings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "nil",
			input: nil,
			want:  nil,
		},
		{
			name:  "empty",
			input: []string{},
			want:  []string{},
		},
		{
			name:  "single",
			input: []string{"foo"},
			want:  []string{"foo"},
		},
		{
			name:  "ordered",
			input: []string{"abc", "def", "hij"},
			want:  []string{"abc", "def", "hij"},
		},
		{
			name:  "sorts",
			input: []string{"foo", "bar", "baz", "app"},
			want:  []string{"app", "bar", "baz", "foo"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			original := slices.Clone(tc.input)

			got := sortStrings(tc.input)
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("incorrect strings (-got,+want): %s", diff)
			}

			if diff := cmp.Diff(tc.input, original); diff != "" {
				t.Errorf("original input was modified (-got,+want): %s", diff)
			}
		})
	}
}

func TestFormatTime(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		time    string
		layout  string
		want    string
		wantErr string
	}{
		{
			name:   "empty",
			time:   "",
			layout: "2006-01-02",
			want:   "",
		},
		{
			name:   "zero",
			time:   "0",
			layout: "2006-01-02",
			want:   "1970-01-01",
		},
		{
			name:   "real_time",
			time:   "1803901319090",
			layout: "2006-01-02",
			want:   "2027-03-01",
		},
		{
			name:    "not_int",
			time:    "banana",
			layout:  "2006-01-02",
			wantErr: "time is not an integer",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := formatTime(tc.time, tc.layout)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("incorrect strings (-got, +want):\n%s", diff)
			}
		})
	}
}
