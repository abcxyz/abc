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
	"strings"
	"testing"

	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestSpecUnmarshal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		in               string
		want             *Spec
		wantUnmarshalErr string
		wantValidateErr  string
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
				APIVersion: String{Val: "cli.abcxyz.dev/v1alpha1"},
				Kind:       String{Val: "Template"},

				Desc: String{Val: "A simple template that just prints and exits"},
				Inputs: []*Input{
					{
						Name:    String{Val: "person_name"},
						Desc:    String{Val: "An optional name of a person to greet"},
						Default: &String{Val: "default value"},
					},
				},
				Steps: []*Step{
					{
						Desc:   String{Val: "Print a message"},
						Action: String{Val: "print"},
						Print: &Print{
							Message: String{Val: `Hello, {{.or .person_name "World"}}`},
						},
					},
				},
			},
		},
		{
			name: "apiVersion_camel_case",
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
				APIVersion: String{Val: "cli.abcxyz.dev/v1alpha1"},
				Kind:       String{Val: "Template"},

				Desc: String{Val: "A simple template that just prints and exits"},
				Inputs: []*Input{
					{
						Name:    String{Val: "person_name"},
						Desc:    String{Val: "An optional name of a person to greet"},
						Default: &String{Val: "default value"},
					},
				},
				Steps: []*Step{
					{
						Desc:   String{Val: "Print a message"},
						Action: String{Val: "print"},
						Print: &Print{
							Message: String{Val: `Hello, {{.or .person_name "World"}}`},
						},
					},
				},
			},
		},
		{
			name: "api_version_both_forms",
			in: `api_version: 'cli.abcxyz.dev/v1alpha1'
apiVersion: 'cli.abcxyz.dev/v1alpha1'
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
			wantUnmarshalErr: "must not set both apiVersion and api_version, please use api_version only",
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
			wantValidateErr: `at line 3 column 3: field "desc" is required`,
		},
		{
			name: "check_required_fields",
			in:   "inputs:",
			wantValidateErr: `at line 1 column 1: field "api_version" value must be one of [cli.abcxyz.dev/v1alpha1]
at line 1 column 1: field "kind" value must be one of [Template]
at line 1 column 1: field "desc" is required
at line 1 column 1: field "steps" is required`,
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := &Spec{}
			dec := newDecoder(strings.NewReader(tc.in))
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

			opt := cmpopts.IgnoreTypes(&ConfigPos{}, ConfigPos{}) // don't force test authors to assert the line and column numbers
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
				Name:    String{Val: "person_name"},
				Desc:    String{Val: "The name of a person to greet"},
				Default: &String{Val: "default"},
			},
		},
		{
			name: "missing_default_is_nil",
			in: `name: 'person_name'
desc: "The name of a person to greet"`,
			want: &Input{
				Name:    String{Val: "person_name"},
				Desc:    String{Val: "The name of a person to greet"},
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
			name: "validation-rule",
			in: `desc: 'foo'
name: 'a'
rules:
  - rule: 'size(a) > 5'
    message: 'my message'`,
			want: &Input{
				Name: String{Val: "a"},
				Desc: String{Val: "foo"},
				Rules: []*InputRule{
					{
						Rule:    String{Val: "size(a) > 5"},
						Message: String{Val: "my message"},
					},
				},
			},
		},
		{
			name: "validation-rule-without-message",
			in: `desc: 'foo'
name: 'a'
rules:
  - rule: 'size(a) > 5'`,
			want: &Input{
				Name: String{Val: "a"},
				Desc: String{Val: "foo"},
				Rules: []*InputRule{
					{
						Rule: String{Val: "size(a) > 5"},
					},
				},
			},
		},
		{
			name: "multiple-validation-rules",
			in: `desc: 'foo'
name: 'a'
rules:
  - rule: 'size(a) > 5'
    message: 'my message'
  - rule: 'size(a) < 100'
    message: 'my other message'`,
			want: &Input{
				Name: String{Val: "a"},
				Desc: String{Val: "foo"},
				Rules: []*InputRule{
					{
						Rule:    String{Val: "size(a) > 5"},
						Message: String{Val: "my message"},
					},
					{
						Rule:    String{Val: "size(a) < 100"},
						Message: String{Val: "my other message"},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := &Input{}
			dec := newDecoder(strings.NewReader(tc.in))
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

			if diff := cmp.Diff(got, tc.want, cmpopts.IgnoreTypes(&ConfigPos{}, ConfigPos{})); diff != "" {
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
				Desc:   String{Val: "mydesc"},
				Action: String{Val: "append"},
				Append: &Append{
					Paths: []String{
						{Val: "a.txt"},
						{Val: "b.txt"},
					},
					With:              String{Val: "jkl"},
					SkipEnsureNewline: Bool{Val: true},
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
				Desc:   String{Val: "Print a message"},
				Action: String{Val: "print"},
				Print: &Print{
					Message: String{Val: "Hello"},
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
				Desc:   String{Val: "mydesc"},
				Action: String{Val: "include"},
				Include: &Include{
					Paths: []*IncludePath{
						{
							From: String{Val: "destination"},
							Paths: []String{
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
				Desc:   String{Val: "mydesc"},
				Action: String{Val: "include"},
				Include: &Include{
					Paths: []*IncludePath{
						{
							From: String{Val: "destination"},
							Paths: []String{
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
			name: "include_with_prefixes",
			in: `desc: 'mydesc'
action: 'include'
params:
  paths:
    - paths: ['a/b/c', 'x/y.txt']
      strip_prefix: 'a/b'
      add_prefix: 'c/d'`,
			want: &Step{
				Desc:   String{Val: "mydesc"},
				Action: String{Val: "include"},
				Include: &Include{
					Paths: []*IncludePath{
						{
							Paths: []String{
								{
									Val: "a/b/c",
								},
								{
									Val: "x/y.txt",
								},
							},
							StripPrefix: String{Val: "a/b"},
							AddPrefix:   String{Val: "c/d"},
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
				Desc:   String{Val: "mydesc"},
				Action: String{Val: "include"},
				Include: &Include{
					Paths: []*IncludePath{
						{
							Paths: []String{
								{
									Val: "a/b/c",
								},
								{
									Val: "d/e/f",
								},
							},
							As: []String{
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
				Desc:   String{Val: "mydesc"},
				Action: String{Val: "include"},
				Include: &Include{
					Paths: []*IncludePath{
						{
							Paths: []String{
								{
									Val: ".",
								},
							},
							Skip: []String{
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
				Desc:   String{Val: "mydesc"},
				Action: String{Val: "include"},
				Include: &Include{
					Paths: []*IncludePath{
						{
							Paths: []String{
								{
									Val: ".",
								},
							},
							From: String{
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
				Desc:   String{Val: "mydesc"},
				Action: String{Val: "include"},
				Include: &Include{
					Paths: []*IncludePath{
						{
							Paths: []String{
								{
									Val: ".",
								},
							},
							From: String{
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
				Desc:   String{Val: "mydesc"},
				Action: String{Val: "include"},
				Include: &Include{
					Paths: []*IncludePath{
						{
							Paths: []String{
								{
									Val: "a/b/c",
								},
								{
									Val: "d/e/f",
								},
							},
							As: []String{
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
				Desc:   String{Val: "mydesc"},
				Action: String{Val: "regex_replace"},
				RegexReplace: &RegexReplace{
					Paths: []String{
						{Val: "a.txt"},
						{Val: "b.txt"},
					},
					Replacements: []*RegexReplaceEntry{
						{
							Regex:             String{Val: "my_(?P<groupname>regex)"},
							SubgroupToReplace: String{Val: "groupname"},
							With:              String{Val: "some_template"},
						},
						{
							Regex: String{Val: "my_other_regex"},
							With:  String{Val: "whatever"},
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
				Desc:   String{Val: "mydesc"},
				Action: String{Val: "regex_name_lookup"},
				RegexNameLookup: &RegexNameLookup{
					Paths: []String{
						{Val: "a.txt"},
						{Val: "b.txt"},
					},
					Replacements: []*RegexNameLookupEntry{
						{Regex: String{Val: "(?P<mygroup>myregex"}},
						{Regex: String{Val: "(?P<myothergroup>myotherregex"}},
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
				Desc:   String{Val: "mydesc"},
				Action: String{Val: "string_replace"},
				StringReplace: &StringReplace{
					Paths: []String{
						{Val: "a.txt"},
						{Val: "b.txt"},
					},
					Replacements: []*StringReplacement{
						{
							ToReplace: String{Val: "abc"},
							With:      String{Val: "def"},
						},
						{
							ToReplace: String{Val: "ghi"},
							With:      String{Val: "jkl"},
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
				Desc:   String{Val: "mydesc"},
				Action: String{Val: "go_template"},
				GoTemplate: &GoTemplate{
					Paths: []String{
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
				Desc:   String{Val: "mydesc"},
				Action: String{Val: "for_each"},
				ForEach: &ForEach{
					Iterator: &ForEachIterator{
						Key: String{Val: "environment"},
						Values: []String{
							{Val: "dev"},
							{Val: "prod"},
						},
					},
					Steps: []*Step{
						{
							Desc:   String{Val: "print some stuff"},
							Action: String{Val: "print"},
							Print: &Print{
								Message: String{Val: `Hello, {{.name}}`},
							},
						},
						{
							Desc:   String{Val: "another action"},
							Action: String{Val: "print"},
							Print: &Print{
								Message: String{Val: "yet another message"},
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
				Desc:   String{Val: "mydesc"},
				Action: String{Val: "for_each"},
				ForEach: &ForEach{
					Iterator: &ForEachIterator{
						Key:        String{Val: "environment"},
						ValuesFrom: &String{Val: "my_cel_expression"},
					},
					Steps: []*Step{
						{
							Desc:   String{Val: "print some stuff"},
							Action: String{Val: "print"},
							Print: &Print{
								Message: String{Val: `Hello, {{.name}}`},
							},
						},
						{
							Desc:   String{Val: "another action"},
							Action: String{Val: "print"},
							Print: &Print{
								Message: String{Val: "yet another message"},
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := &Step{}
			dec := newDecoder(strings.NewReader(tc.in))
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

			opt := cmpopts.IgnoreTypes(&ConfigPos{}, ConfigPos{}) // don't force test authors to assert the line and column numbers
			if diff := cmp.Diff(got, tc.want, opt); diff != "" {
				t.Errorf("unmarshaling didn't yield expected struct. Diff (-got +want): %s", diff)
			}
		})
	}
}
