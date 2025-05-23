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

package render

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/abc/templates/common"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta6"
	abctestutil "github.com/abcxyz/abc/templates/testutil"
	mdl "github.com/abcxyz/abc/templates/testutil/model"
	"github.com/abcxyz/pkg/testutil"
)

func TestActionRegexNameLookup(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		inputs       map[string]string
		initContents map[string]string
		rr           *spec.RegexNameLookup
		want         map[string]string
		wantErr      string
	}{
		{
			name: "simple_success",
			initContents: map[string]string{
				"a.txt": "alpha beta gamma delta",
			},
			inputs: map[string]string{
				"my_input": "foo",
			},
			rr: &spec.RegexNameLookup{
				Paths: mdl.Strings("."),
				Replacements: []*spec.RegexNameLookupEntry{
					{
						Regex: mdl.S(`\b(?P<my_input>b...) (?P<my_input>g....)`),
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha foo foo delta", //nolint:dupword
			},
		},
		{
			name: "same_file_only_processed_once",
			initContents: map[string]string{
				"a.txt": "alpha foo gamma delta",
			},
			inputs: map[string]string{
				"my_input": "foofoo",
			},
			rr: &spec.RegexNameLookup{
				Paths: mdl.Strings("."),
				Replacements: []*spec.RegexNameLookupEntry{
					{
						Regex: mdl.S(`(?P<my_input>foo)`),
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha foofoo gamma delta",
			},
		},
		{
			name: "missing_template_variable_should_fail",
			initContents: map[string]string{
				"a.txt": "alpha beta gamma",
			},
			inputs: map[string]string{},
			rr: &spec.RegexNameLookup{
				Paths: mdl.Strings("."),
				Replacements: []*spec.RegexNameLookupEntry{
					{
						Regex: mdl.S("(?P<mysubgroup>beta)"),
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha beta gamma",
			},
			wantErr: `there was no template input variable matching the subgroup name "mysubgroup"`,
		},
		{
			name: "named_group_autolookup_should_reject_unnamed_subgroups",
			initContents: map[string]string{
				"a.txt": "alpha beta gamma",
			},
			inputs: map[string]string{
				"my_input": "foo",
			},
			rr: &spec.RegexNameLookup{
				Paths: mdl.Strings("."),
				Replacements: []*spec.RegexNameLookupEntry{
					{
						Regex: mdl.S(`\b(?P<my_input>b...) (g....)`),
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha beta gamma",
			},
			wantErr: `must be named`,
		},
		{
			name: "template_expr_in_regex_and_groupname_should_work",
			initContents: map[string]string{
				"a.txt": "alpha beta gamma",
			},
			inputs: map[string]string{
				"regex_to_match": "b...",
				"group_name":     "mygroup",
				"mygroup":        "omega",
			},
			rr: &spec.RegexNameLookup{
				Paths: mdl.Strings("."),
				Replacements: []*spec.RegexNameLookupEntry{
					{
						Regex: mdl.S(`(?P<{{.group_name}}>{{.regex_to_match}})`),
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha omega gamma",
			},
		},
		{
			name: "multiple_files_should_work",
			initContents: map[string]string{
				"a.txt": "alpha beta gamma delta",
				"b.txt": "alpha beta gamma delta",
			},
			inputs: map[string]string{
				"my_input": "foo",
			},
			rr: &spec.RegexNameLookup{
				Paths: mdl.Strings("."),
				Replacements: []*spec.RegexNameLookupEntry{
					{
						Regex: mdl.S(`\b(?P<my_input>b...) (?P<my_input>g....)`),
					},
				},
			},
			want: map[string]string{
				"a.txt": "alpha foo foo delta", //nolint:dupword
				"b.txt": "alpha foo foo delta", //nolint:dupword
			},
		},
		{
			name: "templated_filename",
			initContents: map[string]string{
				"a.txt": "alpha beta gamma",
			},
			rr: &spec.RegexNameLookup{
				Paths: mdl.Strings("{{.filename}}"),
				Replacements: []*spec.RegexNameLookupEntry{
					{
						Regex: mdl.S("(?P<cake>beta)"),
					},
				},
			},
			inputs: map[string]string{
				"filename": "a.txt",
				"cake":     "chocolate",
			},
			want: map[string]string{
				"a.txt": "alpha chocolate gamma",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			scratchDir := t.TempDir()
			abctestutil.WriteAll(t, scratchDir, tc.initContents)

			ctx := t.Context()
			sp := &stepParams{
				scope:      common.NewScope(tc.inputs, nil),
				scratchDir: scratchDir,
				rp: &Params{
					FS: &common.RealFS{},
				},
			}
			err := actionRegexNameLookup(ctx, tc.rr, sp)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			got := abctestutil.LoadDir(t, scratchDir)
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("output differed from expected, (-got,+want): %s", diff)
			}
		})
	}
}
