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
						Regex: model.String{Val: "foo"},
						With:  model.String{Val: "bar"},
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha bar gamma",
			},
		},
		{
			name: "default_multiline_false",
			initContents: map[string]string{
				"a.txt": "apple banana\nbanana apple\napple apple\n", //nolint:dupword
			},
			rr: &model.RegexReplace{
				Paths: modelStrings([]string{"."}),
				Replacements: []*model.RegexReplaceEntry{
					{
						Regex: model.String{Val: "\\n$"},
						With:  model.String{Val: ""},
					},
				},
			},
			want: map[string]string{
				"a.txt": "apple banana\nbanana apple\napple apple", //nolint:dupword
			},
		},
		{
			name: "override_multiline_true",
			initContents: map[string]string{
				"a.txt": "apple banana\nbanana apple\napple apple\n", //nolint:dupword
			},
			rr: &model.RegexReplace{
				Paths: modelStrings([]string{"."}),
				Replacements: []*model.RegexReplaceEntry{
					{
						Regex: model.String{Val: "(?m:apple$)"},
						With:  model.String{Val: "apple."},
					},
				},
			},
			want: map[string]string{
				"a.txt": "apple banana\nbanana apple.\napple apple.\n", //nolint:dupword
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
						Regex: model.String{Val: "foo"},
						With:  model.String{Val: "bar"},
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
						Regex: model.String{Val: `\b(?P<my_first_input>b...) (?P<my_second_input>g....)`},
						With:  model.String{Val: "${my_second_input} ${my_first_input}"},
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha gamma beta delta",
			},
		},
		{
			name: "numbered_subgroup_as_template_variable_should_fail",
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
						Regex: model.String{Val: "template_(?P<mygroup>[a-z]+)"},
						With:  model.String{Val: "{{.$1}}"},
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha template_foo beta",
			},
			wantErr: "regex expansions must reference the subgroup by name",
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
						Regex: model.String{Val: "template_(?P<mysubgroup>[a-z]+)"},
						With:  model.String{Val: "{{.${mysubgroup}}}"},
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
						Regex: model.String{Val: `\b(?P<mysubgroup>be..)\b`},
						With:  model.String{Val: "{{.cool_${mysubgroup}}}"},
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha BETA gamma",
			},
		},
		{
			name: "template_lookup_using_numbered_regex_subgroup_should_not_work",
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
						Regex: model.String{Val: `\b(?P<mygroup>be..)\b`},
						With:  model.String{Val: "{{.cool_${1}}}"},
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha beta gamma",
			},
			wantErr: "regex expansions must reference the subgroup by name",
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
						Regex: model.String{Val: `\b{{.to_replace}}`},
						With:  model.String{Val: `{{.replace_with}}`},
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha BETA! gamma",
			},
		},
		{
			name: "replace_subgroup",
			initContents: map[string]string{
				"a.txt": "alpha beta gamma",
			},
			inputs: map[string]string{
				"myinput": "alligator",
			},
			rr: &model.RegexReplace{
				Paths: modelStrings([]string{"."}),
				Replacements: []*model.RegexReplaceEntry{
					{
						Regex:             model.String{Val: `alpha (?P<mygroup>beta) gamma`},
						With:              model.String{Val: `{{.myinput}}`},
						SubgroupToReplace: model.String{Val: "mygroup"},
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha alligator gamma",
			},
		},
		{
			name: "replace_multiple_subgroups",
			initContents: map[string]string{
				"a.txt": "alpha beta gamma",
			},
			inputs: map[string]string{
				"reptile": "alligator",
				"tree":    "maple",
			},
			rr: &model.RegexReplace{
				Paths: modelStrings([]string{"."}),
				Replacements: []*model.RegexReplaceEntry{
					{
						Regex:             model.String{Val: `alpha (?P<mygroup>beta) gamma`},
						With:              model.String{Val: `{{.reptile}}`},
						SubgroupToReplace: model.String{Val: "mygroup"},
					},
					{
						Regex:             model.String{Val: `[a-z]+ [a-z]+ (?P<mygroup>gamma)`},
						With:              model.String{Val: `{{.tree}}`},
						SubgroupToReplace: model.String{Val: "mygroup"},
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha alligator maple",
			},
		},
		{
			name: "normal_mode_doesnt_catch_line_begin_end_as_anchors",
			initContents: map[string]string{
				"a.txt": `alpha
beta
gamma`,
			},
			rr: &model.RegexReplace{
				Paths: modelStrings([]string{"."}),
				Replacements: []*model.RegexReplaceEntry{
					{
						Regex: model.String{Val: "^beta$"},
						With:  model.String{Val: "shouldnt_appear"},
					},
				},
			},
			want: map[string]string{
				"a.txt": `alpha
beta
gamma`,
			},
		},
		{
			name: "multiline_mode_should_match_line_begin_and_end",
			initContents: map[string]string{
				"a.txt": `alpha
beta
gamma`,
			},
			rr: &model.RegexReplace{
				Paths: modelStrings([]string{"."}),
				Replacements: []*model.RegexReplaceEntry{
					{
						Regex: model.String{Val: "(?m:^beta$)"},
						With:  model.String{Val: "brontosaurus"},
					},
				},
			},
			want: map[string]string{
				"a.txt": `alpha
brontosaurus
gamma`,
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
						Regex: model.String{Val: "foo"},
						With:  model.String{Val: "bar"},
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

func TestRejectNumberedSubgroupExpand(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		wantErr string
	}{
		{
			name:    "reject_numbered",
			in:      "abc $5 def",
			wantErr: "failed executing template spec file at line 1: regex expansions must reference the subgroup by name, like ${mygroup}, rather than by number, like ${1}; we saw $5",
		},
		{
			// Note: "$$" expands to "$", this is not a subgroup reference
			name: "dollardollar_literal_should_not_be_considered",
			in:   "abc $$5 def",
		},
		{
			name: "dollardollardollardollar_literal_should_not_be_considered",
			in:   "abc $$$$5 def",
		},
		{
			name:    "dollardollardollardollardollar_literal_should_be_considered",
			in:      "abc $$$$$5 def",
			wantErr: "must reference the subgroup by name",
		},
		{
			name:    "braces",
			in:      "abc ${5} def",
			wantErr: "must reference the subgroup by name",
		},
		{
			name:    "multiple_subgroups",
			in:      "abc $3 def $5 ghi %4",
			wantErr: "must reference the subgroup by name",
		},
		{
			name: "named_subgroups",
			in:   "abc ${mygroup} def",
		},
		{
			name:    "mix_of_named_and_numbered_subgroups",
			in:      "abc ${mygroup} $5 def",
			wantErr: "we saw $5",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			in := model.String{
				Pos: &model.ConfigPos{
					Line:   1,
					Column: 1,
				},
				Val: tc.in,
			}
			if diff := testutil.DiffErrString(rejectNumberedSubgroupExpand(in), tc.wantErr); diff != "" {
				t.Error(diff)
			}
		})
	}
}
