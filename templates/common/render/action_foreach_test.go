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
	"bytes"
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/abc/templates/common"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta6"
	mdl "github.com/abcxyz/abc/templates/testutil/model"
	"github.com/abcxyz/pkg/testutil"
)

func TestActionForEach(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		in         *spec.ForEach
		inputs     map[string]string
		wantStdout string
		wantErr    string
	}{
		{
			name: "values_literal",
			inputs: map[string]string{
				"from": "Alice",
			},
			in: &spec.ForEach{
				Iterator: &spec.ForEachIterator{
					Key:    mdl.S("greeting_target"),
					Values: mdl.Strings("Bob", "Charlie"),
				},
				Steps: []*spec.Step{
					{
						Print: &spec.Print{
							Message: mdl.S("Hello {{.greeting_target}} from {{.from}}"),
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
			in: &spec.ForEach{
				Iterator: &spec.ForEachIterator{
					Key:    mdl.S("greeting_target"),
					Values: mdl.Strings("{{.first_recipient}}", "{{.second_recipient}}"),
				},
				Steps: []*spec.Step{
					{
						Print: &spec.Print{
							Message: mdl.S("Hello {{.greeting_target}} from {{.from}}"),
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
			in: &spec.ForEach{
				Iterator: &spec.ForEachIterator{
					Key:    mdl.S("greeter"),
					Values: mdl.Strings("{{.first_greeter}}", "{{.second_greeter}}"),
				},
				Steps: []*spec.Step{
					{
						Action: mdl.S("for_each"),
						ForEach: &spec.ForEach{
							Iterator: &spec.ForEachIterator{
								Key:    mdl.S("greeting_target"),
								Values: mdl.Strings("{{.first_recipient}}", "{{.second_recipient}}"),
							},
							Steps: []*spec.Step{
								{
									Print: &spec.Print{
										Message: mdl.S("Hello {{.greeting_target}} from {{.greeter}}"),
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
			name: "single_value_literal",
			inputs: map[string]string{
				"color": "Blue",
			},
			in: &spec.ForEach{
				Iterator: &spec.ForEachIterator{
					Key:    mdl.S("color"),
					Values: mdl.Strings("Red"),
				},
				Steps: []*spec.Step{
					{
						Print: &spec.Print{
							Message: mdl.S("{{.color}}"),
						},
					},
				},
			},
			wantStdout: "Red\n",
		},
		{
			name:   "errors_are_propagated",
			inputs: map[string]string{},
			in: &spec.ForEach{
				Iterator: &spec.ForEachIterator{
					Key:    mdl.S("x"),
					Values: mdl.Strings("Alice"),
				},
				Steps: []*spec.Step{
					{
						Print: &spec.Print{
							Message: mdl.S("{{.nonexistent}}"),
						},
					},
				},
			},
			wantErr: `nonexistent variable name "nonexistent"`,
		},
		{
			name: "cel_values_from",
			inputs: map[string]string{
				"environments": "production,dev",
			},
			in: &spec.ForEach{
				Iterator: &spec.ForEachIterator{
					Key:        mdl.S("env"),
					ValuesFrom: mdl.SP(`environments.split(",")`),
				},
				Steps: []*spec.Step{
					{
						Print: &spec.Print{
							Message: mdl.S("{{.env}}"),
						},
					},
				},
			},
			wantStdout: "production\ndev\n",
		},
		{
			name: "cel_values_empty_no_actions",
			in: &spec.ForEach{
				Iterator: &spec.ForEachIterator{
					Key:        mdl.S("env"),
					ValuesFrom: mdl.SP(`[]`),
				},
				Steps: []*spec.Step{
					{
						Print: &spec.Print{
							Message: mdl.S("{{.env}}"),
						},
					},
				},
			},
			wantStdout: "",
		},
		{
			name: "cel_values_literal",
			in: &spec.ForEach{
				Iterator: &spec.ForEachIterator{
					Key:        mdl.S("env"),
					ValuesFrom: mdl.SP(`["production", "dev"]`),
				},
				Steps: []*spec.Step{
					{
						Print: &spec.Print{
							Message: mdl.S("{{.env}}"),
						},
					},
				},
			},
			wantStdout: "production\ndev\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			buf := &bytes.Buffer{}
			sp := &stepParams{
				scope: common.NewScope(tc.inputs, nil),
				rp: &Params{
					Stdout: buf,
				},
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
