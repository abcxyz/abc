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

package v1alpha1

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gopkg.in/yaml.v3"

	"github.com/abcxyz/abc/templates/model"
	mdl "github.com/abcxyz/abc/templates/testutil/model"
	"github.com/abcxyz/pkg/testutil"
)

func TestSpecUnmarshal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		in               string
		want             *Spec
		wantUnmarshalErr string
		wantValidateErr  []string
	}{
		{
			name: "simple_template_should_succeed",
			in: `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'

desc: 'A simple template that just prints and exits'
inputs:
- name: 'person_name'
  desc: 'An optional name of a person to greet'
  default: 'default value'

steps:
- desc: 'Print a message'
  action: 'print'
  params:
    message: 'Hello, {{.or .person_name "World"}}'`,
			want: &Spec{
				Desc: mdl.S("A simple template that just prints and exits"),
				Inputs: []*Input{
					{
						Name:    mdl.S("person_name"),
						Desc:    mdl.S("An optional name of a person to greet"),
						Default: mdl.SP("default value"),
					},
				},
				Steps: []*Step{
					{
						Desc:   mdl.S("Print a message"),
						Action: mdl.S("print"),
						Print: &Print{
							Message: mdl.S(`Hello, {{.or .person_name "World"}}`),
						},
					},
				},
			},
		},
		{
			name: "apiVersion_camel_case",
			in: `apiVersion: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'

desc: 'A simple template that just prints and exits'
inputs:
- name: 'person_name'
  desc: 'An optional name of a person to greet'
  default: 'default value'

steps:
- desc: 'Print a message'
  action: 'print'
  params:
    message: 'Hello, {{.or .person_name "World"}}'`,
			want: &Spec{
				Desc: mdl.S("A simple template that just prints and exits"),
				Inputs: []*Input{
					{
						Name:    mdl.S("person_name"),
						Desc:    mdl.S("An optional name of a person to greet"),
						Default: mdl.SP("default value"),
					},
				},
				Steps: []*Step{
					{
						Desc:   mdl.S("Print a message"),
						Action: mdl.S("print"),
						Print: &Print{
							Message: mdl.S(`Hello, {{.or .person_name "World"}}`),
						},
					},
				},
			},
		},
		{
			name: "validation_of_children_should_occur_and_fail",
			in: `desc: 'A simple template that just prints and exits'
inputs:
- name: 'person_name'

steps:
- desc: 'Print a message'
  action: 'print'
  params:
    message: 'Hello, {{.or .person_name "World"}}'`,
			wantValidateErr: []string{`at line 3 column 3: field "desc" is required`},
		},
		{
			name: "check_required_fields",
			in:   "inputs:",
			wantValidateErr: []string{
				`at line 1 column 1: field "desc" is required`,
				`at line 1 column 1: field "steps" is required`,
			},
		},

		{
			name: "unknown_field_should_fail",
			in: `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'

desc: 'A simple template that just prints and exits'
inputs:
- name: 'person_name'
  desc: 'An optional name of a person to greet'
  not_a_real_field: 'oops'

steps:
- desc: 'Print a message'
  action: 'print'
  params:
    message: 'Hello'`,
			wantUnmarshalErr: `at line 8 column 3: unknown field name "not_a_real_field"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := &Spec{}

			dec := yaml.NewDecoder(strings.NewReader(tc.in))
			err := dec.Decode(got)
			if diff := testutil.DiffErrString(err, tc.wantUnmarshalErr); diff != "" {
				t.Fatal(diff)
			}
			if err != nil {
				return
			}

			err = got.Validate()
			for _, wantValidateErr := range tc.wantValidateErr {
				if diff := testutil.DiffErrString(err, wantValidateErr); diff != "" {
					t.Fatal(diff)
				}
			}
			if err != nil {
				return
			}

			opt := cmpopts.IgnoreTypes(&model.ConfigPos{}, model.ConfigPos{}) // don't force test authors to assert the line and column numbers
			if diff := cmp.Diff(got, tc.want, opt); diff != "" {
				t.Errorf("unmarshaling didn't yield expected struct. Diff (-got +want): %s", diff)
			}
		})
	}
}

func TestUnmarshalInput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		in               string
		want             *Input
		wantUnmarshalErr string
		wantValidateErr  string
	}{
		{
			name: "simple_case_should_pass",
			in: `name: 'person_name'
desc: "The name of a person to greet"
default: "default"`,
			want: &Input{
				Name:    mdl.S("person_name"),
				Desc:    mdl.S("The name of a person to greet"),
				Default: mdl.SP("default"),
			},
		},
		{
			name: "missing_default_is_nil",
			in: `name: 'person_name'
desc: "The name of a person to greet"`,
			want: &Input{
				Name:    mdl.S("person_name"),
				Desc:    mdl.S("The name of a person to greet"),
				Default: nil,
			},
		},
		{
			name:            "missing_required_fields_should_fail",
			in:              `desc: 'a thing'`,
			wantValidateErr: `at line 1 column 1: field "name" is required`,
		},
		{
			name: "unexpected_field_should_fail",
			in: `name: 'a'
desc: 'b'
nonexistent_field: 'oops'`,
			wantUnmarshalErr: `at line 3 column 1: unknown field name "nonexistent_field"`,
		},
		{
			name: "reserved_input_name",
			in: `desc: 'foo'
name: '_name_with_leading_underscore'`,
			wantValidateErr: "are reserved",
		},
		{
			name: "validation_rule",
			in: `desc: 'foo'
name: 'a'
rules:
  - rule: 'size(a) > 5'
    message: 'my message'`,
			want: &Input{
				Name: mdl.S("a"),
				Desc: mdl.S("foo"),
				Rules: []*InputRule{
					{
						Rule:    mdl.S("size(a) > 5"),
						Message: mdl.S("my message"),
					},
				},
			},
		},
		{
			name: "validation_rule_without_message",
			in: `desc: 'foo'
name: 'a'
rules:
  - rule: 'size(a) > 5'`,
			want: &Input{
				Name: mdl.S("a"),
				Desc: mdl.S("foo"),
				Rules: []*InputRule{
					{
						Rule: mdl.S("size(a) > 5"),
					},
				},
			},
		},
		{
			name: "multiple_validation_rules",
			in: `desc: 'foo'
name: 'a'
rules:
  - rule: 'size(a) > 5'
    message: 'my message'
  - rule: 'size(a) < 100'
    message: 'my other message'`,
			want: &Input{
				Name: mdl.S("a"),
				Desc: mdl.S("foo"),
				Rules: []*InputRule{
					{
						Rule:    mdl.S("size(a) > 5"),
						Message: mdl.S("my message"),
					},
					{
						Rule:    mdl.S("size(a) < 100"),
						Message: mdl.S("my other message"),
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := &Input{}
			dec := yaml.NewDecoder(strings.NewReader(tc.in))
			err := dec.Decode(got)
			if diff := testutil.DiffErrString(err, tc.wantUnmarshalErr); diff != "" {
				t.Fatal(diff)
			}
			if err != nil {
				return
			}

			err = got.Validate()
			if diff := testutil.DiffErrString(err, tc.wantValidateErr); diff != "" {
				t.Fatal(diff)
			}
			if err != nil {
				return
			}

			if diff := cmp.Diff(got, tc.want, cmpopts.IgnoreTypes(&model.ConfigPos{}, model.ConfigPos{})); diff != "" {
				t.Errorf("unmarshaling didn't yield expected struct. Diff (-got +want): %s", diff)
			}
		})
	}
}

func TestUnmarshalStep(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		in               string
		want             *Step
		wantUnmarshalErr string
		wantValidateErr  string
	}{
		{
			name: "append_success",
			in: `desc: 'mydesc'
action: 'append'
params:
  paths: ['a.txt', 'b.txt']
  with: 'jkl'
  skip_ensure_newline: true`,
			want: &Step{
				Desc:   mdl.S("mydesc"),
				Action: mdl.S("append"),
				Append: &Append{
					Paths: []model.String{
						{Val: "a.txt"},
						{Val: "b.txt"},
					},
					With:              mdl.S("jkl"),
					SkipEnsureNewline: model.Bool{Val: true},
				},
			},
		},
		{
			name: "append_missing_with_field_should_fail",
			in: `desc: 'mydesc'
action: 'append'
params:
  paths: ['a.txt']`,
			wantValidateErr: `at line 4 column 3: field "with" is required`,
		},
		{
			name: "append_missing_paths_field_should_fail",
			in: `desc: 'mydesc'
action: 'append'
params:
  with: 'def'`,
			wantValidateErr: `at line 4 column 3: field "paths" is required`,
		},
		{
			name: "append_non_bool_skip_ensure_newline_field_should_fail",
			in: `desc: 'mydesc'
action: 'append'
params:
  paths: ['a.txt']
  with: 'jkl'
  skip_ensure_newline: pizza`,
			wantUnmarshalErr: "cannot unmarshal !!str `pizza` into bool",
		},
		{
			name: "print_success",
			in: `desc: 'Print a message'
action: 'print'
params:
  message: 'Hello'`,
			want: &Step{
				Desc:   mdl.S("Print a message"),
				Action: mdl.S("print"),
				Print: &Print{
					Message: mdl.S("Hello"),
				},
			},
		},
		{
			name: "print_empty_message",
			in: `desc: 'Print a message'
action: 'print'
params:
  message: ''`,
			wantValidateErr: `field "message" is required`,
		},
		{
			name: "print_empty_message",
			in: `desc: 'Print a message'
action: 'print'
params:
  message: 'hello'
  extra_field: 'oops'`,
			wantUnmarshalErr: `at line 5 column 3: unknown field name "extra_field"`,
		},
		{
			name: "print_missing_message",
			in: `desc: 'Print a message'
action: 'print'
params: `,
			wantValidateErr: `at line 1 column 1: field "message" is required`,
		},
		{
			name: "include_success_paths_are_string", // not path objects, paths are just strings
			in: `desc: 'mydesc'
action: 'include'
params:
  paths: ['a/b/c', 'x/y.txt']
  from: 'destination'`,
			want: &Step{
				Desc:   mdl.S("mydesc"),
				Action: mdl.S("include"),
				Include: &Include{
					Paths: []*IncludePath{
						{
							From: mdl.S("destination"),
							Paths: []model.String{
								{
									Val: "a/b/c",
								},
								{
									Val: "x/y.txt",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "include_success_paths_are_objects",
			in: `desc: 'mydesc'
action: 'include'
params:
  paths:
  - paths: ['a/b/c', 'x/y.txt']
    from: 'destination'`,
			want: &Step{
				Desc:   mdl.S("mydesc"),
				Action: mdl.S("include"),
				Include: &Include{
					Paths: []*IncludePath{
						{
							From: mdl.S("destination"),
							Paths: []model.String{
								{
									Val: "a/b/c",
								},
								{
									Val: "x/y.txt",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "include_with_as",
			in: `desc: 'mydesc'
action: 'include'
params:
  paths:
    - paths: ['a/b/c', 'd/e/f']
      as: ['x/y/z', 'q/r/s']`,
			want: &Step{
				Desc:   mdl.S("mydesc"),
				Action: mdl.S("include"),
				Include: &Include{
					Paths: []*IncludePath{
						{
							Paths: []model.String{
								{
									Val: "a/b/c",
								},
								{
									Val: "d/e/f",
								},
							},
							As: []model.String{
								{
									Val: "x/y/z",
								},
								{
									Val: "q/r/s",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "include_with_skip",
			in: `desc: 'mydesc'
action: 'include'
params:
  paths:
    - paths: ['.']
      skip: ['x/y']`,
			want: &Step{
				Desc:   mdl.S("mydesc"),
				Action: mdl.S("include"),
				Include: &Include{
					Paths: []*IncludePath{
						{
							Paths: []model.String{
								{
									Val: ".",
								},
							},
							Skip: []model.String{
								{
									Val: "x/y",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "include_from_destination",
			in: `desc: 'mydesc'
action: 'include'
params:
  paths:
    - paths: ['.']
      from: 'destination'`,
			want: &Step{
				Desc:   mdl.S("mydesc"),
				Action: mdl.S("include"),
				Include: &Include{
					Paths: []*IncludePath{
						{
							Paths: []model.String{
								{
									Val: ".",
								},
							},
							From: model.String{
								Val: "destination",
							},
						},
					},
				},
			},
		},
		{
			name: "include_paths_heterogeneous_list",
			in: `desc: 'mydesc'
action: 'include'
params:
  paths:
    - 'a.txt'
    - paths: ['b.txt']`,
			wantUnmarshalErr: "Lists of paths must be homogeneous, either all strings or all objects",
		},
		{
			name: "other_include_fields_forbidden_with_path_objects",
			in: `desc: 'mydesc'
action: 'include'
params:
  from: 'destination'
  paths:
    - paths:
      - 'a.txt'`,
			wantUnmarshalErr: `unknown field name "from"`,
		},
		{
			name: "include_from_invalid",
			in: `desc: 'mydesc'
action: 'include'
params:
  paths:
    - paths: ['.']
      from: 'invalid'`,
			want: &Step{
				Desc:   mdl.S("mydesc"),
				Action: mdl.S("include"),
				Include: &Include{
					Paths: []*IncludePath{
						{
							Paths: []model.String{
								{
									Val: ".",
								},
							},
							From: model.String{
								Val: "invalid",
							},
						},
					},
				},
			},
			wantValidateErr: `"from" must be one of`,
		},
		{
			name: "wrong_number_of_as_paths",
			in: `desc: 'mydesc'
action: 'include'
params:
  paths:
    - paths: ['a/b/c', 'd/e/f']
      as: ['x/y/z', 'q/r/s', 't/u/v']`,
			want: &Step{
				Desc:   mdl.S("mydesc"),
				Action: mdl.S("include"),
				Include: &Include{
					Paths: []*IncludePath{
						{
							Paths: []model.String{
								{
									Val: "a/b/c",
								},
								{
									Val: "d/e/f",
								},
							},
							As: []model.String{
								{
									Val: "x/y/z",
								},
								{
									Val: "q/r/s",
								},
								{
									Val: "t/u/v",
								},
							},
						},
					},
				},
			},
			wantValidateErr: `the size of "as" (3) must be the same as the size of "paths" (2)`,
		},
		{
			name: "missing_action_field_should_fail",
			in: `desc: 'mydesc'
params:
  paths:
    - 'a/b/c'
    - 'x/y.txt'`,
			wantUnmarshalErr: `missing "action" field`,
		},
		{
			name: "empty_include_paths_should_fail",
			in: `desc: 'mydesc'
action: 'include'
params:
  paths: []`,
			wantValidateErr: `at line 4 column 3: field "paths" is required`,
		},
		{
			name: "missing_include_paths_should_fail",
			in: `desc: 'mydesc'
action: 'include'
params:`,
			wantValidateErr: `at line 1 column 1: field "paths" is required`,
		},
		{
			name: "unknown_params_should_fail",
			in: `desc: 'mydesc'
action: 'include'
params:
  nonexistent: 'foo'
  paths: ['a.txt']`,
			wantUnmarshalErr: `at line 4 column 3: unknown field name "nonexistent"`,
		},
		{
			name: "regex_replace_success",
			in: `desc: 'mydesc'
action: 'regex_replace'
params:
  paths: ['a.txt', 'b.txt']
  replacements:
  - regex: 'my_(?P<groupname>regex)'
    subgroup_to_replace: 'groupname'
    with: 'some_template'
  - regex: 'my_other_regex'
    with: 'whatever'`,
			want: &Step{
				Desc:   mdl.S("mydesc"),
				Action: mdl.S("regex_replace"),
				RegexReplace: &RegexReplace{
					Paths: []model.String{
						{Val: "a.txt"},
						{Val: "b.txt"},
					},
					Replacements: []*RegexReplaceEntry{
						{
							Regex:             mdl.S("my_(?P<groupname>regex)"),
							SubgroupToReplace: mdl.S("groupname"),
							With:              mdl.S("some_template"),
						},
						{
							Regex: mdl.S("my_other_regex"),
							With:  mdl.S("whatever"),
						},
					},
				},
			},
		},

		{
			name: "regex_replace_invalid_subgroup_should_fail",
			in: `desc: 'mydesc'
action: 'regex_replace'
params:
  paths: ['a.txt']
  replacements:
  - regex: '(?p<x>y)'
    subgroup_to_replace: 1
    with: 'some_template'`,
			wantValidateErr: `at line 7 column 26: subgroup name must be a letter followed by zero or more alphanumerics`,
		},
		{
			name: "regex_missing_fields_should_fail",
			in: `desc: 'mydesc'
action: 'regex_replace'
params:
  paths: ['a.txt']
  replacements:
  - subgroup_to_replace: xyz`,
			wantValidateErr: `at line 6 column 5: field "regex" is required
at line 6 column 5: field "with" is required`,
		},

		{
			name: "regex_replace_negative_numbered_subgroup_should_fail",
			in: `desc: 'mydesc'
action: 'regex_replace'
params:
  paths: ['a.txt']
  replacements:
  - regex: 'my_regex'
    subgroup_to_replace: -1
    with: 'some_template'`,
			wantValidateErr: `at line 7 column 26: subgroup name must be a letter followed by zero or more alphanumerics`,
		},
		{
			name: "regex_missing_fields_should_fail",
			in: `desc: 'mydesc'
action: 'regex_replace'
params:
  paths: ['a.txt']
  replacements:
  - subgroup_to_replace: xyz`,
			wantValidateErr: `at line 6 column 5: field "regex" is required
at line 6 column 5: field "with" is required`,
		},
		{
			name: "regex_name_lookup_success",
			in: `desc: 'mydesc'
action: 'regex_name_lookup'
params:
  paths: ['a.txt', 'b.txt']
  replacements:
  - regex: '(?P<mygroup>myregex'
  - regex: '(?P<myothergroup>myotherregex'`,
			want: &Step{
				Desc:   mdl.S("mydesc"),
				Action: mdl.S("regex_name_lookup"),
				RegexNameLookup: &RegexNameLookup{
					Paths: []model.String{
						{Val: "a.txt"},
						{Val: "b.txt"},
					},
					Replacements: []*RegexNameLookupEntry{
						{Regex: mdl.S("(?P<mygroup>myregex")},
						{Regex: mdl.S("(?P<myothergroup>myotherregex")},
					},
				},
			},
		},
		{
			name: "regex_name_lookup_extra_fields_should_fail",
			in: `desc: 'mydesc'
action: 'regex_name_lookup'
params:
  paths: ['a.txt']
  replacements:
  - fakefield: 'abc' `,
			wantUnmarshalErr: `unknown field name "fakefield"`,
		},
		{
			name: "string_replace_success",
			in: `desc: 'mydesc'
action: 'string_replace'
params:
  paths: ['a.txt', 'b.txt']
  replacements:
  - to_replace: 'abc'
    with: 'def'
  - to_replace: 'ghi'
    with: 'jkl'`,
			want: &Step{
				Desc:   mdl.S("mydesc"),
				Action: mdl.S("string_replace"),
				StringReplace: &StringReplace{
					Paths: []model.String{
						{Val: "a.txt"},
						{Val: "b.txt"},
					},
					Replacements: []*StringReplacement{
						{
							ToReplace: mdl.S("abc"),
							With:      mdl.S("def"),
						},
						{
							ToReplace: mdl.S("ghi"),
							With:      mdl.S("jkl"),
						},
					},
				},
			},
		},
		{
			name: "string_replace_missing_replacements_field_should_fail",
			in: `desc: 'mydesc'
action: 'string_replace'
params:
  paths: ['a.txt']`,
			wantValidateErr: `at line 4 column 3: field "replacements" is required`,
		},
		{
			name: "string_replace_missing_paths_field_should_fail",
			in: `desc: 'mydesc'
action: 'string_replace'
params:
  replacements:
  - to_replace: 'abc'
    with: 'def'`,
			wantValidateErr: `at line 4 column 3: field "paths" is required`,
		},
		{
			name: "go_template_success",
			in: `desc: 'mydesc'
action: 'go_template'
params:
  paths: ['my/path/1', 'my/path/2']`,
			want: &Step{
				Desc:   mdl.S("mydesc"),
				Action: mdl.S("go_template"),
				GoTemplate: &GoTemplate{
					Paths: []model.String{
						{Val: "my/path/1"},
						{Val: "my/path/2"},
					},
				},
			},
		},
		{
			name: "go_template_missing_paths_should_fail",
			in: `desc: 'mydesc'
action: 'go_template'
params:
  paths: []`,
			wantValidateErr: `at line 4 column 3: field "paths" is required`,
		},
		{
			name: "for_each_range_over_list",
			in: `desc: 'mydesc'
action: 'for_each'
params:
  iterator:
    key: 'environment'
    values: ['dev', 'prod']
  steps:
    - desc: 'print some stuff'
      action: 'print'
      params:
        message: 'Hello, {{.name}}'
    - desc: 'another action'
      action: 'print'
      params:
        message: 'yet another message'
`,
			want: &Step{
				Desc:   mdl.S("mydesc"),
				Action: mdl.S("for_each"),
				ForEach: &ForEach{
					Iterator: &ForEachIterator{
						Key: mdl.S("environment"),
						Values: []model.String{
							{Val: "dev"},
							{Val: "prod"},
						},
					},
					Steps: []*Step{
						{
							Desc:   mdl.S("print some stuff"),
							Action: mdl.S("print"),
							Print: &Print{
								Message: mdl.S(`Hello, {{.name}}`),
							},
						},
						{
							Desc:   mdl.S("another action"),
							Action: mdl.S("print"),
							Print: &Print{
								Message: mdl.S("yet another message"),
							},
						},
					},
				},
			},
		},
		{
			name: "for_each_range_over_expression",
			in: `desc: 'mydesc'
action: 'for_each'
params:
  iterator:
    key: 'environment'
    values_from: 'my_cel_expression'
  steps:
    - desc: 'print some stuff'
      action: 'print'
      params:
        message: 'Hello, {{.name}}'
    - desc: 'another action'
      action: 'print'
      params:
        message: 'yet another message'
`,
			want: &Step{
				Desc:   mdl.S("mydesc"),
				Action: mdl.S("for_each"),
				ForEach: &ForEach{
					Iterator: &ForEachIterator{
						Key:        mdl.S("environment"),
						ValuesFrom: mdl.SP("my_cel_expression"),
					},
					Steps: []*Step{
						{
							Desc:   mdl.S("print some stuff"),
							Action: mdl.S("print"),
							Print: &Print{
								Message: mdl.S(`Hello, {{.name}}`),
							},
						},
						{
							Desc:   mdl.S("another action"),
							Action: mdl.S("print"),
							Print: &Print{
								Message: mdl.S("yet another message"),
							},
						},
					},
				},
			},
		},
		{
			name: "for_each_missing_values",
			in: `desc: 'mydesc'
action: 'for_each'
params:
  iterator:
    key: 'environment'
    # missing the "values" here
  steps:
    - desc: 'print some stuff'
      action: 'print'
      params:
        message: 'Hello, {{.name}}'
`,
			wantValidateErr: `exactly one of the fields "values" or "values_from" must be set`,
		},
		{
			name: "for_each_missing_key",
			in: `desc: 'mydesc'
action: 'for_each'
params:
  iterator:
    # key: 'environment'
    values: ['dev', 'prod']
  steps:
    - desc: 'print some stuff'
      action: 'print'
      params:
        message: 'Hello, {{.name}}'
`,
			wantValidateErr: `field "key" is required`,
		},
		{
			name: "for_each_missing_iterator",
			in: `desc: 'mydesc'
action: 'for_each'
params:
  steps:
    - desc: 'print some stuff'
      action: 'print'
      params:
        message: 'Hello, {{.name}}'
`,
			wantValidateErr: `field "iterator" is required`,
		},
		{
			name: "for_each_values_and_values_from",
			in: `desc: 'mydesc'
action: 'for_each'
params:
  iterator:
    key: 'environment'
    values: ['dev', 'prod']
    values_from: 'cel_expression'
  steps:
    - desc: 'print some stuff'
      action: 'print'
      params:
        message: 'Hello, {{.name}}'
`,
			wantValidateErr: `exactly one of the fields "values" or "values_from" must be set`,
		},
		{
			name: "for_each_no_steps",
			in: `desc: 'mydesc'
action: 'for_each'
params:
  iterator:
    key: 'environment'
    values: ['dev', 'prod']
`,
			wantValidateErr: `field "steps" is required`,
		},
		{
			name: "for_each_values_wrong_type",
			in: `desc: 'mydesc'
action: 'for_each'
params:
  iterator:
    key: 'environment'
    values: 'prod'  # invalid, should be a list of string
  steps:
    - desc: 'print some stuff'
      action: 'print'
      params:
        message: 'Hello, {{.name}}'
`,
			wantUnmarshalErr: `line 6: cannot unmarshal`,
		},
		{
			name: "for_each_values_from_wrong_type",
			in: `desc: 'mydesc'
action: 'for_each'
params:
  iterator:
    key: 'environment'
    values_from: ['dev', 'prod'] # invalid, should be string
  steps:
    - desc: 'print some stuff'
      action: 'print'
      params:
        message: 'Hello, {{.name}}'
`,
			wantUnmarshalErr: `line 6: cannot unmarshal`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := &Step{}
			dec := yaml.NewDecoder(strings.NewReader(tc.in))
			err := dec.Decode(got)
			if diff := testutil.DiffErrString(err, tc.wantUnmarshalErr); diff != "" {
				t.Fatal(diff)
			}
			if err != nil {
				return
			}

			err = got.Validate()
			if diff := testutil.DiffErrString(err, tc.wantValidateErr); diff != "" {
				t.Fatal(diff)
			}
			if err != nil {
				return
			}

			opt := cmpopts.IgnoreTypes(&model.ConfigPos{}, model.ConfigPos{}) // don't force test authors to assert the line and column numbers
			if diff := cmp.Diff(got, tc.want, opt); diff != "" {
				t.Errorf("unmarshaling didn't yield expected struct. Diff (-got +want): %s", diff)
			}
		})
	}
}
