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
	"context"
	"testing"

	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
)

func TestActionRegexReplace(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		inputs       map[string]string
		initContents map[string]string
		rr           *model.RegexReplace
		want         map[string]string
		wantErr      string
	}{
		{
			name: "simple_success",
			initContents: map[string]string{
				"a.txt": "alpha foo gamma",
			},
			rr: &model.RegexReplace{
				Paths: modelStrings([]string{"."}),
				Replacements: []*model.RegexReplaceEntry{
					{
						Regex:    model.String{Val: "foo"},
						With:     model.String{Val: "bar"},
						Subgroup: model.Int{Val: 0},
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha bar gamma",
			},
		},
		{
			name: "multiple_matches_should_work",
			initContents: map[string]string{
				"a.txt": "alpha foo gamma foo",
			},
			rr: &model.RegexReplace{
				Paths: modelStrings([]string{"."}),
				Replacements: []*model.RegexReplaceEntry{
					{
						Regex:    model.String{Val: "foo"},
						With:     model.String{Val: "bar"},
						Subgroup: model.Int{Val: 0},
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha bar gamma bar",
			},
		},
		{
			name: "replacing_named_groups_should_work",
			initContents: map[string]string{
				"a.txt": "alpha beta gamma delta",
			},
			inputs: map[string]string{},
			rr: &model.RegexReplace{
				Paths: modelStrings([]string{"."}),
				Replacements: []*model.RegexReplaceEntry{
					{
						Regex:    model.String{Val: `\b(?P<my_first_input>b...) (?P<my_second_input>g....)`},
						Subgroup: model.Int{Val: 0},
						With:     model.String{Val: "${my_second_input} ${my_first_input}"},
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha gamma beta delta",
			},
		},
		{
			name: "numbered_subgroup_as_template_variable_should_work",
			initContents: map[string]string{
				"a.txt": "alpha template_foo beta",
			},
			inputs: map[string]string{
				"foo": "bar",
			},
			rr: &model.RegexReplace{
				Paths: modelStrings([]string{"."}),
				Replacements: []*model.RegexReplaceEntry{
					{
						Regex:    model.String{Val: "template_([a-z]+)"},
						Subgroup: model.Int{Val: 0},
						With:     model.String{Val: "{{.$1}}"},
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha bar beta",
			},
		},
		{
			name: "named_subgroup_template_variable_should_work",
			initContents: map[string]string{
				"a.txt": "alpha template_foo beta",
			},
			inputs: map[string]string{
				"foo": "bar",
			},
			rr: &model.RegexReplace{
				Paths: modelStrings([]string{"."}),
				Replacements: []*model.RegexReplaceEntry{
					{
						Regex:    model.String{Val: "template_(?P<mysubgroup>[a-z]+)"},
						Subgroup: model.Int{Val: 0},
						With:     model.String{Val: "{{.${mysubgroup}}}"},
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha bar beta",
			},
		},
		{
			name: "template_lookup_using_named_regex_subgroup_should_work",
			initContents: map[string]string{
				"a.txt": "alpha beta gamma",
			},
			inputs: map[string]string{
				"cool_beta": "BETA",
			},
			rr: &model.RegexReplace{
				Paths: modelStrings([]string{"."}),
				Replacements: []*model.RegexReplaceEntry{
					{
						Regex:    model.String{Val: `\b(?P<mysubgroup>be..)\b`},
						Subgroup: model.Int{Val: 0},
						With:     model.String{Val: "{{.cool_${mysubgroup}}}"},
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha BETA gamma",
			},
		},
		{
			name: "template_lookup_using_numbered_regex_subgroup_should_work",
			initContents: map[string]string{
				"a.txt": "alpha beta gamma",
			},
			inputs: map[string]string{
				"cool_beta": "BETA",
			},
			rr: &model.RegexReplace{
				Paths: modelStrings([]string{"."}),
				Replacements: []*model.RegexReplaceEntry{
					{
						Regex:    model.String{Val: `\b(be..)\b`},
						Subgroup: model.Int{Val: 0},
						With:     model.String{Val: "{{.cool_${1}}}"},
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha BETA gamma",
			},
		},
		{
			name: "numbered_subgroup_out_of_range_should_fail",
			initContents: map[string]string{
				"a.txt": "alpha beta gamma",
			},
			inputs: map[string]string{},
			rr: &model.RegexReplace{
				Paths: modelStrings([]string{"."}),
				Replacements: []*model.RegexReplaceEntry{
					{
						Regex:    model.String{Val: `\b(b...)`},
						Subgroup: model.Int{Val: 0},
						With:     model.String{Val: "{{.$9}}"},
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha beta gamma",
			},
			wantErr: "subgroup $9 is out of range; the largest subgroup in this regex is 1",
		},
		{
			name: "regex_with_template_reference_should_work",
			initContents: map[string]string{
				"a.txt": "alpha beta gamma",
			},
			inputs: map[string]string{
				"to_replace":   "beta",
				"replace_with": "BETA!",
			},
			rr: &model.RegexReplace{
				Paths: modelStrings([]string{"."}),
				Replacements: []*model.RegexReplaceEntry{
					{
						Regex:    model.String{Val: `\b{{.to_replace}}`},
						Subgroup: model.Int{Val: 0},
						With:     model.String{Val: `{{.replace_with}}`},
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha BETA! gamma",
			},
		},
		{
			name: "multiple_files_should_work",
			initContents: map[string]string{
				"a.txt": "alpha foo gamma",
				"b.txt": "sigma foo chi",
			},
			rr: &model.RegexReplace{
				Paths: modelStrings([]string{"."}),
				Replacements: []*model.RegexReplaceEntry{
					{
						Regex:    model.String{Val: "foo"},
						With:     model.String{Val: "bar"},
						Subgroup: model.Int{Val: 0},
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha bar gamma",
				"b.txt": "sigma bar chi",
			},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			scratchDir := t.TempDir()
			if err := writeAllDefaultMode(scratchDir, tc.initContents); err != nil {
				t.Fatal(err)
			}

			ctx := context.Background()
			sp := &stepParams{
				fs:         &realFS{},
				inputs:     tc.inputs,
				scratchDir: scratchDir,
			}
			err := actionRegexReplace(ctx, tc.rr, sp)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			got := loadDirWithoutMode(t, scratchDir)
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("output differed from expected, (-got,+want): %s", diff)
			}
		})
	}
}

func TestMaxSubGroup(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want int
	}{
		{
			name: "simple_success",
			in:   "abc $5 def",
			want: 5,
		},
		{
			// Note: "$$" expands to "$", this is not a subgroup reference
			name: "dollardollar_literal_should_not_be_considered",
			in:   "abc $$5 def",
			want: 0,
		},
		{
			name: "dollardollardollardollar_literal_should_not_be_considered",
			in:   "abc $$$$5 def",
			want: 0,
		},
		{
			name: "dollardollardollardollardollar_literal_should_be_considered",
			in:   "abc $$$$$5 def",
			want: 5,
		},
		{
			name: "braces_should_work",
			in:   "abc ${5} def",
			want: 5,
		},
		{
			name: "multiple_subgroup_should_work",
			in:   "abc $3 def $5 ghi %4",
			want: 5,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := maxSubgroup([]byte(tc.in)); got != tc.want {
				t.Errorf("maxSubgroup(%s)=%d, want %d", tc.in, got, tc.want)
			}
		})
	}
}
