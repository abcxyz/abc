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
	"context"
	"reflect"
	"testing"

	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/cel-go/cel"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestCompileAndEvalCEL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		in             model.String
		vars           map[string]string
		want           any
		wantCompileErr string
		wantEvalErr    string
	}{
		{
			name: "simple-success",
			in:   model.String{Val: `["alligator","crocodile"]`},
			want: []string{"alligator", "crocodile"},
		},
		{
			name:        "bad-int-to-string",
			in:          model.String{Val: `42`},
			want:        []string(nil),
			wantEvalErr: `CEL expression result couldn't be converted to []string. The CEL engine error was: unsupported type conversion from 'int' to []string`,
		},
		{
			name:        "bad-list-of-int-to-list-of-string",
			in:          model.String{Val: `[42]`},
			want:        []string(nil),
			wantEvalErr: `CEL expression result couldn't be converted to []string. The CEL engine error was: unsupported type conversion from 'int' to string`,
		},
		{
			name:        "bad-heterogenous list",
			in:          model.String{Val: `["alligator", 42]`},
			want:        []string(nil),
			wantEvalErr: `CEL expression result couldn't be converted to []string. The CEL engine error was: unsupported type conversion from 'int' to string`,
		},
		{
			name: "string-split",
			in:   model.String{Val: `"alligator,crocodile".split(",")`},
			want: []string{"alligator", "crocodile"},
		},
		{
			name: "input-vars",
			in:   model.String{Val: `["alligator", reptile]`},
			vars: map[string]string{"reptile": "crocodile"},
			want: []string{"alligator", "crocodile"},
		},
		{
			name:           "invalid-cel-syntax",
			in:             model.String{Val: `[[[[[`},
			want:           "",
			wantCompileErr: "Syntax error: mismatched input",
		},
		{
			name: "yaml-pos-passed-through-on-error",
			in: model.String{
				Val: `[[[[[`,
				Pos: &model.ConfigPos{Line: 9876},
			},
			want:           "",
			wantCompileErr: "9876",
		},
		{
			name: "simple-int-return",
			in:   model.String{Val: "42"},
			want: 42,
		},
		{
			name: "simple-uint-return",
			in:   model.String{Val: "42u"},
			want: uint(42),
		},
		{
			name: "simple-map-return",
			in:   model.String{Val: `{"reptile": "alligator"}`},
			want: map[string]any{"reptile": "alligator"},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			scope := newScope(tc.vars)

			prog, err := celCompile(ctx, scope, tc.in)
			if diff := testutil.DiffErrString(err, tc.wantCompileErr); diff != "" {
				t.Fatal(diff)
			}
			if err != nil {
				return // If compilation failed, don't try to eval.
			}

			// Create a new "any" variable whose type is pointer-to-the-type-of-tc.want.
			gotPtr := reflect.New(reflect.ValueOf(tc.want).Type()).Interface()

			err = celEval(ctx, scope, tc.in.Pos, prog, gotPtr)
			if diff := testutil.DiffErrString(err, tc.wantEvalErr); diff != "" {
				t.Fatal(diff)
			}

			// Dereference the pointer hidden inside the gotPtr "any" variable.
			got := reflect.ValueOf(gotPtr).Elem().Interface()

			if diff := cmp.Diff(got, tc.want, cmpopts.EquateEmpty()); diff != "" {
				t.Fatalf("output %q of type %T was not as expected (-got,+want): %s", got, got, diff)
			}
		})
	}
}

// Tests for all of our custom functions that we add to CEL.
func TestCELFuncs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		expr    string
		want    any
		wantErr string
	}{
		{
			name: "string-split-success",
			expr: `"foo,bar".split(",")`,
			want: []string{"foo", "bar"},
		},
		{
			name: "string-split-no-separator",
			expr: `"foo".split(",")`,
			want: []string{"foo"},
		},
		{
			name: "string-split-multiple-separators",
			expr: `"foo,bar,baz".split(",")`,
			want: []string{"foo", "bar", "baz"},
		},
		{
			name: "string-split-empty-string",
			expr: `"".split(",")`,
			want: []string{""},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			prog, err := celCompile(ctx, newScope(nil), model.String{Val: tc.expr})
			if err != nil {
				t.Fatal(err)
			}

			celOut, _, err := prog.Eval(cel.NoVars())
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}

			celOutTyped, err := celOut.ConvertToNative(reflect.TypeOf(tc.want))
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(celOutTyped, tc.want); diff != "" {
				t.Errorf("output was not as expected (-got,+want): %s", diff)
			}
		})
	}
}
