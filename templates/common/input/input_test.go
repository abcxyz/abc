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

package input

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	spec "github.com/abcxyz/abc/templates/model/spec/v1beta6"
	mdl "github.com/abcxyz/abc/templates/testutil/model"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/testutil"
)

func TestPromptForInputs_CanceledContext(t *testing.T) {
	t.Parallel()

	cmd := &cli.BaseCommand{}

	stdinReader, _ := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	_, stderrWriter := io.Pipe()

	cmd.SetStdin(stdinReader)
	cmd.SetStdout(stdoutWriter)
	cmd.SetStderr(stderrWriter)

	ctx, cancel := context.WithCancel(t.Context())

	errCh := make(chan error)
	go func() {
		defer close(errCh)
		spec := &spec.Spec{
			Inputs: []*spec.Input{
				{
					Name: mdl.S("my_input"),
				},
			},
		}
		errCh <- promptForInputs(ctx, cmd, spec, map[string]string{})
	}()

	go func() {
		for {
			// Read and discard prompt printed to the user.
			if _, err := stdoutReader.Read(make([]byte, 1024)); err != nil {
				return
			}
		}
	}()

	cancel()
	var err error
	select {
	case err = <-errCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the background goroutine to finish")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got an error %v, want context.Canceled", err)
	}

	stdoutWriter.Close() // terminate the background goroutine blocking on stdoutReader.Read()
}

func TestValidateInputs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		inputModels []*spec.Input
		inputVals   map[string]string
		want        string
	}{
		{
			name: "no_validation_rule",
			inputModels: []*spec.Input{
				{
					Name: mdl.S("my_input"),
				},
			},
			inputVals: map[string]string{
				"my_input": "foo",
			},
		},
		{
			name: "single_passing_validation_rule",
			inputModels: []*spec.Input{
				{
					Name: mdl.S("my_input"),
					Rules: []*spec.Rule{
						{
							Rule:    mdl.S(`size(my_input) < 5`),
							Message: mdl.S("Length must be less than 5"),
						},
					},
				},
			},
			inputVals: map[string]string{
				"my_input": "foo",
			},
		},
		{
			name: "single_failing_validation_rule",
			inputModels: []*spec.Input{
				{
					Name: mdl.S("my_input"),
					Rules: []*spec.Rule{
						{
							Rule:    mdl.S(`size(my_input) < 3`),
							Message: mdl.S("Length must be less than 3"),
						},
					},
				},
			},
			inputVals: map[string]string{
				"my_input": "foo",
			},
			want: `input validation failed:

Input name:   my_input
Input value:  foo
Rule:         size(my_input) < 3
Rule msg:     Length must be less than 3`,
		},
		{
			name: "multiple_passing_validation_rules",
			inputModels: []*spec.Input{
				{
					Name: mdl.S("my_input"),
					Rules: []*spec.Rule{
						{
							Rule:    mdl.S(`size(my_input) < 5`),
							Message: mdl.S("Length must be less than 5"),
						},
						{
							Rule:    mdl.S(`my_input.startsWith("fo")`),
							Message: mdl.S(`Must start with "fo"`),
						},
						{
							Rule:    mdl.S(`my_input.contains("oo")`),
							Message: mdl.S(`Must contain "oo"`),
						},
					},
				},
			},
			inputVals: map[string]string{
				"my_input": "foo",
			},
		},
		{
			name: "multiple_passing_validation_rules_one_failing",
			inputModels: []*spec.Input{
				{
					Name: mdl.S("my_input"),
					Rules: []*spec.Rule{
						{
							Rule:    mdl.S(`size(my_input) < 3`),
							Message: mdl.S("Length must be less than 3"),
						},
						{
							Rule:    mdl.S(`my_input.startsWith("fo")`),
							Message: mdl.S(`Must start with "fo"`),
						},
						{
							Rule:    mdl.S(`my_input.contains("oo")`),
							Message: mdl.S(`Must contain "oo"`),
						},
					},
				},
			},
			inputVals: map[string]string{
				"my_input": "foo",
			},
			want: `input validation failed:

Input name:   my_input
Input value:  foo
Rule:         size(my_input) < 3
Rule msg:     Length must be less than 3`,
		},
		{
			name: "multiple_failing_validation_rules",
			inputModels: []*spec.Input{
				{
					Name: mdl.S("my_input"),
					Rules: []*spec.Rule{
						{
							Rule:    mdl.S(`size(my_input) < 3`),
							Message: mdl.S("Length must be less than 3"),
						},
						{
							Rule:    mdl.S(`my_input.startsWith("ham")`),
							Message: mdl.S(`Must start with "ham"`),
						},
						{
							Rule:    mdl.S(`my_input.contains("shoe")`),
							Message: mdl.S(`Must contain "shoe"`),
						},
					},
				},
			},
			inputVals: map[string]string{
				"my_input": "foo",
			},
			want: `input validation failed:

Input name:   my_input
Input value:  foo
Rule:         size(my_input) < 3
Rule msg:     Length must be less than 3

Input name:   my_input
Input value:  foo
Rule:         my_input.startsWith("ham")
Rule msg:     Must start with "ham"

Input name:   my_input
Input value:  foo
Rule:         my_input.contains("shoe")
Rule msg:     Must contain "shoe"`,
		},
		{
			name: "cel_syntax_error",
			inputModels: []*spec.Input{
				{
					Name: mdl.S("my_input"),
					Rules: []*spec.Rule{
						{
							Rule: mdl.S(`(`),
						},
					},
				},
			},
			inputVals: map[string]string{
				"my_input": "foo",
			},
			want: `input validation failed:

Input name:   my_input
Input value:  foo
Rule:         (
CEL error:    failed compiling CEL expression: ERROR: <input>:1:2: Syntax error:`, // remainder of error omitted
		},
		{
			name: "cel_type_conversion_error",
			inputModels: []*spec.Input{
				{
					Name: mdl.S("my_input"),
					Rules: []*spec.Rule{
						{
							Rule: mdl.S(`bool(42)`),
						},
					},
				},
			},
			inputVals: map[string]string{
				"my_input": "foo",
			},
			want: `input validation failed:

Input name:   my_input
Input value:  foo
Rule:         bool(42)
CEL error:    failed compiling CEL expression: ERROR: <input>:1:5: found no matching overload for 'bool'`, // remainder of error omitted
		},
		{
			name: "cel_output_type_conversion_error",
			inputModels: []*spec.Input{
				{
					Name: mdl.S("my_input"),
					Rules: []*spec.Rule{
						{
							Rule: mdl.S(`42`),
						},
					},
				},
			},
			inputVals: map[string]string{
				"my_input": "foo",
			},
			want: `input validation failed:

Input name:   my_input
Input value:  foo
Rule:         42
CEL error:    CEL expression result couldn't be converted to bool. The CEL engine error was: unsupported type conversion from 'int' to bool`, // remainder of error omitted
		},
		{
			name: "multi_input_validation",
			inputModels: []*spec.Input{
				{
					Name: mdl.S("my_input"),
					Rules: []*spec.Rule{
						{
							Rule: mdl.S(`my_input + my_other_input == "sharknado"`),
						},
					},
				},
				{
					Name: mdl.S("my_other_input"),
					Rules: []*spec.Rule{
						{
							Rule: mdl.S(`"tor" + my_other_input + my_input == "tornadoshark"`),
						},
					},
				},
			},
			inputVals: map[string]string{
				"my_input":       "shark",
				"my_other_input": "nado",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			err := validateInputs(ctx, tc.inputModels, tc.inputVals)
			if diff := testutil.DiffErrString(err, tc.want); diff != "" {
				t.Error(diff)
			}
		})
	}
}
