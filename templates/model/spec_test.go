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
			in: `apiVersion: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'

desc: 'A simple template that just prints and exits'
inputs:
- name: 'person_name'
  required: false  # The default is required=true
  desc: 'An optional name of a person to greet'

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
						Name:     String{Val: "person_name"},
						Desc:     String{Val: "An optional name of a person to greet"},
						Required: Bool{Val: false},
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
			name: "validation_of_children_should_occur_and_fail",
			in: `desc: 'A simple template that just prints and exits'
inputs:
- name: 'person_name'

steps:
- desc: 'Print a message'
  action: 'print'
  params:
    message: 'Hello, {{.or .person_name "World"}}'`,
			wantValidateErr: `invalid config near line 3 column 3: field "desc" is required`,
		},
		{
			name: "check_required_fields",
			in:   "inputs:",
			wantValidateErr: `invalid config near line 1 column 1: field "apiVersion" value must be one of [cli.abcxyz.dev/v1alpha1]
invalid config near line 1 column 1: field "kind" value must be one of [Template]
invalid config near line 1 column 1: field "desc" is required
invalid config near line 1 column 1: field "steps" is required`,
		},

		{
			name: "unknown_field_should_fail",
			in: `apiVersion: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'

desc: 'A simple template that just prints and exits'
inputs:
- name: 'person_name'
  required: false  # The default is required=true
  desc: 'An optional name of a person to greet'
not_a_real_field: 'oops'

steps:
- desc: 'Print a message'
  action: 'print'
  params:
    message: 'Hello'`,
			wantUnmarshalErr: `invalid config near line 9 column 1: unknown field name "not_a_real_field"`,
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

			opt := cmpopts.IgnoreTypes(&ConfigPos{}) // don't force test authors to assert the line and column numbers
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
required: false`,
			want: &Input{
				Name:     String{Val: "person_name"},
				Desc:     String{Val: "The name of a person to greet"},
				Required: Bool{Val: false},
			},
		},
		{
			name: "default_true_for_required",
			in: `name: 'person_name'
desc: "The name of a person to greet"`,
			want: &Input{
				Name:     String{Val: "person_name"},
				Desc:     String{Val: "The name of a person to greet"},
				Required: Bool{Val: true},
			},
		},
		{
			name:            "missing_required_fields_should_fail",
			in:              `desc: 'a thing'`,
			wantValidateErr: `invalid config near line 1 column 1: field "name" is required`,
		},
		{
			name: "unexpected_field_should_fail",
			in: `name: 'a'
desc: 'b'
nonexistent_field: 'oops'`,
			wantUnmarshalErr: `invalid config near line 3 column 1: unknown field name "nonexistent_field"`,
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

			if diff := cmp.Diff(got, tc.want, cmpopts.IgnoreTypes(&ConfigPos{})); diff != "" {
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
			wantUnmarshalErr: `invalid config near line 5 column 3: unknown field name "extra_field"`,
		},
		{
			name: "print_missing_message",
			in: `desc: 'Print a message'
action: 'print'
params: `,
			wantValidateErr: `invalid config near line 1 column 1: field "message" is required`,
		},
		{
			name: "include_success",
			in: `desc: 'mydesc'
action: 'include'
params:
  paths:
    - 'a/b/c'
    - 'x/y.txt'`,
			want: &Step{
				Desc:   String{Val: "mydesc"},
				Action: String{Val: "include"},
				Include: &Include{
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
  paths:`,
			wantValidateErr: `invalid config near line 4 column 3: field "paths" is required`,
		},
		{
			name: "missing_include_paths_should_fail",
			in: `desc: 'mydesc'
action: 'include'
params:`,
			wantValidateErr: `invalid config near line 1 column 1: field "paths" is required`,
		},
		{
			name: "unknown_params_should_fail",
			in: `desc: 'mydesc'
action: 'include'
params:
  nonexistent: 'foo'`,
			wantUnmarshalErr: `invalid config near line 4 column 3: unknown field name "nonexistent"`,
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
			wantValidateErr: `invalid config near line 7 column 26: subgroup name must be a letter followed by zero or more alphanumerics`,
		},
		{
			name: "regex_missing_fields_should_fail",
			in: `desc: 'mydesc'
action: 'regex_replace'
params:
  paths: ['a.txt']
  replacements:
  - subgroup_to_replace: xyz`,
			wantValidateErr: `invalid config near line 6 column 5: field "regex" is required
invalid config near line 6 column 5: field "with" is required`,
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
			wantValidateErr: `invalid config near line 7 column 26: subgroup name must be a letter followed by zero or more alphanumerics`,
		},
		{
			name: "regex_missing_fields_should_fail",
			in: `desc: 'mydesc'
action: 'regex_replace'
params:
  paths: ['a.txt']
  replacements:
  - subgroup_to_replace: xyz`,
			wantValidateErr: `invalid config near line 6 column 5: field "regex" is required
invalid config near line 6 column 5: field "with" is required`,
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
			wantValidateErr: `invalid config near line 4 column 3: field "replacements" is required`,
		},
		{
			name: "string_replace_missing_paths_field_should_fail",
			in: `desc: 'mydesc'
action: 'string_replace'
params:
  replacements:
  - to_replace: 'abc'
    with: 'def'`,
			wantValidateErr: `invalid config near line 4 column 3: field "paths" is required`,
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
			wantValidateErr: `invalid config near line 4 column 3: field "paths" is required`,
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

			opt := cmpopts.IgnoreTypes(&ConfigPos{}) // don't force test authors to assert the line and column numbers
			if diff := cmp.Diff(got, tc.want, opt); diff != "" {
				t.Errorf("unmarshaling didn't yield expected struct. Diff (-got +want): %s", diff)
			}
		})
	}
}
