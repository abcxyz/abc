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
			in: `apiVersion: 'abcxyz.dev/cli/v1alpha1'
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
				APIVersion: String{Val: "abcxyz.dev/cli/v1alpha1"},
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
			wantValidateErr: `invalid config near line 1 column 1: field "apiVersion" is required
invalid config near line 1 column 1: field "kind" is required
invalid config near line 1 column 1: field "desc" is required
invalid config near line 1 column 1: field "steps" is required`,
		},

		{
			name: "unknown_field_should_fail",
			in: `apiVersion: 'abcxyz.dev/cli/v1alpha1'
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
			dec := NewDecoder(strings.NewReader(tc.in))
			if err := dec.Decode(got); tc.wantUnmarshalErr != "" || err != nil {
				if diff := testutil.DiffErrString(err, tc.wantUnmarshalErr); diff != "" {
					t.Fatal(diff)
				}
				return
			}

			if err := got.Validate(); tc.wantValidateErr != "" || err != nil {
				if diff := testutil.DiffErrString(err, tc.wantValidateErr); diff != "" {
					t.Fatal(diff)
				}
				return
			}

			opt := cmpopts.IgnoreTypes(ConfigPos{}) // don't force test authors to assert the line and column numbers
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
			dec := NewDecoder(strings.NewReader(tc.in))
			if err := dec.Decode(got); tc.wantUnmarshalErr != "" || err != nil {
				if diff := testutil.DiffErrString(err, tc.wantUnmarshalErr); diff != "" {
					t.Fatal(diff)
				}
				return
			}

			if err := got.Validate(); tc.wantValidateErr != "" || err != nil {
				if diff := testutil.DiffErrString(err, tc.wantValidateErr); diff != "" {
					t.Fatal(diff)
				}
				return
			}

			if diff := cmp.Diff(got, tc.want, cmpopts.IgnoreTypes(ConfigPos{})); diff != "" {
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
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := &Step{}
			dec := NewDecoder(strings.NewReader(tc.in))
			if err := dec.Decode(got); tc.wantUnmarshalErr != "" || err != nil {
				if diff := testutil.DiffErrString(err, tc.wantUnmarshalErr); diff != "" {
					t.Fatal(diff)
				}
				return
			}

			if err := got.Validate(); tc.wantValidateErr != "" || err != nil {
				if diff := testutil.DiffErrString(err, tc.wantValidateErr); diff != "" {
					t.Fatal(diff)
				}
				return
			}

			opt := cmpopts.IgnoreTypes(ConfigPos{}) // don't force test authors to assert the line and column numbers
			if diff := cmp.Diff(got, tc.want, opt); diff != "" {
				t.Errorf("unmarshaling didn't yield expected struct. Diff (-got +want): %s", diff)
			}
		})
	}
}
