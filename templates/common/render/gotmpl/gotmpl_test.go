// Copyright 2023 The Authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package gotmpl

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/render/gotmpl/funcs"
	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/abc/templates/model/spec/features"
	"github.com/abcxyz/pkg/testutil"
)

// These are basic tests to ensure the template functions are mounted. More
// exhaustive tests are at template_funcs_test.go.
func TestTemplateFuncs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		tmpl     string
		inputs   map[string]string
		features features.Features
		want     string
		wantErr  string
	}{
		{
			name: "contains_true",
			tmpl: `{{ contains "food" "foo" }}`,
			want: "true",
		},
		{
			name: "contains_false",
			tmpl: `{{ contains "food" "bar" }}`,
			want: "false",
		},
		{
			name: "replace",
			tmpl: `{{ replace "food" "foo" "bar" 1 }}`,
			want: "bard",
		},
		{
			name: "replaceAll",
			tmpl: `{{ replaceAll "food food food" "foo" "bar" }}`,
			want: "bard bard bard", //nolint:dupword // expected
		},
		{
			name: "sortStrings",
			tmpl: `{{ split "zebra,car,foo" "," | sortStrings }}`,
			want: "[car foo zebra]",
		},
		{
			name: "split",
			tmpl: `{{ split "a,b,c" "," }}`,
			want: "[a b c]",
		},
		{
			name: "toLower",
			tmpl: `{{ toLower "AbCD" }}`,
			want: "abcd",
		},
		{
			name: "toUpper",
			tmpl: `{{ toUpper "AbCD" }}`,
			want: "ABCD",
		},
		{
			name: "trimPrefix",
			tmpl: `{{ trimPrefix "foobarbaz" "foo" }}`,
			want: "barbaz",
		},
		{
			name: "trimSuffix",
			tmpl: `{{ trimSuffix "foobarbaz" "baz" }}`,
			want: "foobar",
		},
		{
			name: "toSnakeCase",
			tmpl: `{{ toSnakeCase "foo-bar-baz" }}`,
			want: "foo_bar_baz",
		},
		{
			name: "toLowerSnakeCase",
			tmpl: `{{ toLowerSnakeCase "foo-bar-baz" }}`,
			want: "foo_bar_baz",
		},
		{
			name: "toUpperSnakeCase",
			tmpl: `{{ toUpperSnakeCase "foo-bar-baz" }}`,
			want: "FOO_BAR_BAZ",
		},
		{
			name: "toHyphenCase",
			tmpl: `{{ toHyphenCase "foo_bar_baz" }}`,
			want: "foo-bar-baz",
		},
		{
			name: "toLowerHyphenCase",
			tmpl: `{{ toLowerHyphenCase "foo_bar_baz" }}`,
			want: "foo-bar-baz",
		},
		{
			name: "toUpperHyphenCase",
			tmpl: `{{ toUpperHyphenCase "foo-bar-baz" }}`,
			want: "FOO-BAR-BAZ",
		},
		{
			name:     "formatTime_fails_on_old_spec_file",
			tmpl:     `{{ formatTime 1709846071000 "2006-01-02T15:04:05" }}`,
			features: features.Features{SkipTime: true},
			wantErr:  `function "formatTime" not defined`,
		},
		{
			name: "formatTime_succeeds_on_new_spec_file",
			tmpl: `{{ formatTime "1709846071000" "2006-01-02T15:04:05" }}`,
			want: "2024-03-07T21:14:31",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pos := &model.ConfigPos{
				Line: 1,
			}

			funcs := funcs.Funcs(tc.features)
			scope := common.NewScope(map[string]string{}, funcs)
			got, err := ParseExec(pos, tc.tmpl, scope)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("template output was not as expected (-got,+want): %s", diff)
			}
		})
	}
}
