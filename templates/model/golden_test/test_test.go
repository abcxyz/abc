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

package golden_test

import (
	"strings"
	"testing"

	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
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
			in: `api_version: 'cli.abcxyz.dev/v1alpha1'
inputs:
- name: 'person_name'
  value: 'iron_man'
- name: 'dog_name'
  value: 'iron_dog'`,
			want: &Test{
				APIVersion: model.String{Val: "cli.abcxyz.dev/v1alpha1"},
				Inputs: []*InputValue{
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
			name: "missing_field_should_fail",
			in: `api_version: 'cli.abcxyz.dev/v1alpha1'
inputs:
- name: 'person_name'`,
			wantErr: `at line 3 column 3: field "value" is required`,
		},
		{
			name: "unknown_field_should_fail",
			in: `api_version: 'cli.abcxyz.dev/v1alpha1'
inputs:
- name: 'person_name'
  value: 'iron_man'
  pet: 'iron_dog'`,
			wantErr: `error parsing test YAML file: at line 5 column 3: unknown field name "pet"; valid choices are [name value]`,
		},
		{
			name: "missing_api_version_should_fail",
			in: `inputs:
- name: 'person_name'
  value: 'iron_man'`,
			wantErr: `at line 1 column 1: field "api_version" value must be one of [cli.abcxyz.dev/v1alpha1]`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := DecodeTest(strings.NewReader(tc.in))
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
