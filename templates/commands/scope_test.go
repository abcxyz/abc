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

package commands

import (
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestScope(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		with    map[string]string
		inherit map[string]string
		want    map[string]string
	}{
		{
			name: "simple_unnested",
			with: map[string]string{
				"a": "A",
			},
			inherit: nil,
			want: map[string]string{
				"a": "A",
			},
		},
		{
			name: "outer_scope_should_be_shadowed",
			with: map[string]string{
				"a": "good",
			},
			inherit: map[string]string{
				"a": "bad",
			},
			want: map[string]string{
				"a": "good",
			},
		},
		{
			name: "mingling_of_scopes",
			with: map[string]string{
				"a":           "good",
				"other_inner": "foo",
			},
			inherit: map[string]string{
				"a":           "bad",
				"other_outer": "bar",
			},
			want: map[string]string{
				"a":           "good",
				"other_inner": "foo",
				"other_outer": "bar",
			},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			scope := newScope(tc.inherit)
			scope = scope.With(tc.with)

			if diff := cmp.Diff(scope.All(), tc.want); diff != "" {
				t.Errorf("All() returned unexpected value (-got,+want): %s", diff)
			}

			for key, want := range tc.want {
				got, ok := scope.Lookup(key)
				if !ok {
					t.Errorf("Lookup(%q) got false, want (%q,true)", key, want)
					continue
				}
				if got != want {
					t.Errorf("Lookup(%q) got %q, want %q", key, got, want)
				}
			}

			if got, ok := scope.Lookup("nonexistent"); ok {
				t.Errorf(`Lookup("nonexistent") got (%q,true), but wanted false`, got)
			}
		})
	}
}

func TestScopeDeepNesting(t *testing.T) {
	t.Parallel()

	inMaps := make([]map[string]string, 10)
	for i := 0; i <= 9; i++ {
		asStr := strconv.Itoa(i)

		inMaps[i] = map[string]string{
			"key_" + asStr: "value_" + asStr,
			"overwrite":    asStr,
		}
	}

	scope := newScope(inMaps[0])
	for i := 1; i <= 9; i++ {
		scope = scope.With(inMaps[i])
	}

	want := map[string]string{
		"key_0":     "value_0",
		"key_1":     "value_1",
		"key_2":     "value_2",
		"key_3":     "value_3",
		"key_4":     "value_4",
		"key_5":     "value_5",
		"key_6":     "value_6",
		"key_7":     "value_7",
		"key_8":     "value_8",
		"key_9":     "value_9",
		"overwrite": "9",
	}
	got := scope.All()

	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("output map wasn't as expected (-got,+want): %s", diff)
	}
}
