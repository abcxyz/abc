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

package goldentest

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gopkg.in/yaml.v3"

	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/pkg/testutil"
)

func TestTestUnmarshal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		want    *Test
		wantErr string
	}{
		{
			name: "simple_test_should_succeed",
			in: `inputs:
- name: 'person_name'
  value: 'iron_man'
- name: 'dog_name'
  value: 'iron_dog'`,
			want: &Test{
				Inputs: []*VarValue{
					{
						Name:  model.String{Val: "person_name"},
						Value: model.String{Val: "iron_man"},
					},
					{
						Name:  model.String{Val: "dog_name"},
						Value: model.String{Val: "iron_dog"},
					},
				},
			},
		},
		{
			name: "no_inputs_should_succeed",
			in:   "",
			want: &Test{},
		},
		{
			name: "empty_string_value",
			in: `inputs:
- name: 'person_name'
  value: ''`,
  			want: &Test{
				Inputs: []*VarValue{
					{
						Name:  model.String{Val: "person_name"},
						Value: model.String{Val: ""},
					},
				},
			},
		},
		{
			name: "unknown_field_should_fail",
			in: `inputs:
- name: 'person_name'
  value: 'iron_man'
  pet: 'iron_dog'`,
			wantErr: `at line 4 column 3: unknown field name "pet"; valid choices are [name value]`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := &Test{}
			err := yaml.Unmarshal([]byte(tc.in), got)
			if err == nil {
				err = got.Validate()
			}
			if err != nil {
				if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
					t.Fatal(diff)
				}
				return
			}

			opt := cmpopts.IgnoreTypes(&model.ConfigPos{}, model.ConfigPos{})
			if diff := cmp.Diff(got, tc.want, opt); diff != "" {
				t.Fatalf("unmarshaling didn't yield expected struct. Diff (-got +want): %s", diff)
			}
		})
	}
}
