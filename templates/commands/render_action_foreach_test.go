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
	"bytes"
	"context"
	"testing"

	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
)

func TestActionForEach(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		in         *model.ForEach
		inputs     map[string]string
		wantStdout string
		wantErr    string
	}{
		{
			name: "values_literal",
			inputs: map[string]string{
				"from": "Alice",
			},
			in: &model.ForEach{
				Iterator: &model.ForEachIterator{
					Key: model.String{Val: "greeting_target"},
					Values: []model.String{
						{Val: "Bob"},
						{Val: "Charlie"},
					},
				},
				Steps: []*model.Step{
					{
						Print: &model.Print{
							Message: model.String{Val: "Hello {{.greeting_target}} from {{.from}}"},
						},
					},
				},
			},
			wantStdout: "Hello Bob from Alice\nHello Charlie from Alice\n",
		},
		{
			name: "templated_values",
			inputs: map[string]string{
				"from":             "Alice",
				"first_recipient":  "Bob",
				"second_recipient": "Charlie",
			},
			in: &model.ForEach{
				Iterator: &model.ForEachIterator{
					Key: model.String{Val: "greeting_target"},
					Values: []model.String{
						{Val: "{{.first_recipient}}"},
						{Val: "{{.second_recipient}}"},
					},
				},
				Steps: []*model.Step{
					{
						Print: &model.Print{
							Message: model.String{Val: "Hello {{.greeting_target}} from {{.from}}"},
						},
					},
				},
			},
			wantStdout: "Hello Bob from Alice\nHello Charlie from Alice\n",
		},
		{
			name: "nested",
			inputs: map[string]string{
				"first_greeter":    "Alice",
				"second_greeter":   "Zendaya",
				"first_recipient":  "Bob",
				"second_recipient": "Charlie",
			},
			in: &model.ForEach{
				Iterator: &model.ForEachIterator{
					Key: model.String{Val: "greeter"},
					Values: []model.String{
						{Val: "{{.first_greeter}}"},
						{Val: "{{.second_greeter}}"},
					},
				},
				Steps: []*model.Step{
					{
						Action: model.String{Val: "for_each"},
						ForEach: &model.ForEach{
							Iterator: &model.ForEachIterator{
								Key: model.String{Val: "greeting_target"},
								Values: []model.String{
									{Val: "{{.first_recipient}}"},
									{Val: "{{.second_recipient}}"},
								},
							},
							Steps: []*model.Step{
								{
									Print: &model.Print{
										Message: model.String{Val: "Hello {{.greeting_target}} from {{.greeter}}"},
									},
								},
							},
						},
					},
				},
			},
			wantStdout: "Hello Bob from Alice\nHello Charlie from Alice\nHello Bob from Zendaya\nHello Charlie from Zendaya\n",
		},
		{
			name: "values_literal",
			inputs: map[string]string{
				"color": "Blue",
			},
			in: &model.ForEach{
				Iterator: &model.ForEachIterator{
					Key: model.String{Val: "color"},
					Values: []model.String{
						{Val: "Red"},
					},
				},
				Steps: []*model.Step{
					{
						Print: &model.Print{
							Message: model.String{Val: "{{.color}}"},
						},
					},
				},
			},
			wantStdout: "Red\n",
		},
		{
			name:   "errors_are_propagated",
			inputs: map[string]string{},
			in: &model.ForEach{
				Iterator: &model.ForEachIterator{
					Key: model.String{Val: "x"},
					Values: []model.String{
						{Val: "Alice"},
					},
				},
				Steps: []*model.Step{
					{
						Print: &model.Print{
							Message: model.String{Val: "{{.nonexistent}}"},
						},
					},
				},
			},
			wantErr: `nonexistent input variable name "nonexistent"`,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			buf := &bytes.Buffer{}
			sp := &stepParams{
				scope:  newScope(tc.inputs),
				stdout: buf,
				flags:  &RenderFlags{},
			}
			err := actionForEach(ctx, tc.in, sp)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			got := buf.String()
			if diff := cmp.Diff(got, tc.wantStdout); diff != "" {
				t.Errorf("stdout was not as expected (-got,+want): %s", diff)
			}
		})
	}
}
