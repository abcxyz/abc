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

package utils

import (
	"context"
	"fmt"
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
			scope := NewScope(tc.vars)

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

			compileEvalForTest(t, tc.expr, tc.want, tc.wantErr)
		})
	}
}

func TestGCPMatchesServiceAccount(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		param   string
		want    bool
		wantErr string
	}{
		{
			name:  "simple-success",
			param: `"platform-ops@abcxyz-my-project.iam.gserviceaccount.com"`,
			want:  true,
		},
		{
			name:  "must-not-begin-with-digit",
			param: `"9platform-ops@abcxyz-my-project.iam.gserviceaccount.com"`,
			want:  false,
		},
		{
			name:  "must-not-begin-with-dash",
			param: `"-platform-ops@abcxyz-my-project.iam.gserviceaccount.com"`,
			want:  false,
		},
		{
			name:  "longest-valid",
			param: `"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa@abcxyz-my-project.iam.gserviceaccount.com"`,
			want:  true,
		},
		{
			name:  "too-long",
			param: `"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa@abcxyz-my-project.iam.gserviceaccount.com"`,
			want:  false,
		},
		{
			name:  "too-short",
			param: `"abcde@abcxyz-my-project.iam.gserviceaccount.com"`,
			want:  false,
		},
		{
			name:  "shortest-valid",
			param: `"abcdef@abcxyz-my-project.iam.gserviceaccount.com"`,
			want:  true,
		},
		{
			name:  "default-compute-service-account",
			param: `"824005440568-compute@developer.gserviceaccount.com"`,
			want:  true,
		},
		{
			name:  "wrong-domain",
			param: `"abcdef@abcxyz-my-project.iam.fake.biz"`,
			want:  false,
		},
		{
			name:    "type-error-number-as-service-account",
			param:   `42`,
			wantErr: `found no matching overload for 'gcp_matches_service_account' applied to '(int)'`,
		},
		{
			name:  "random-string",
			param: `"alligator"`,
			want:  false,
		},
		{
			name:  "random-email-address",
			param: `"example@example.com"`,
			want:  false,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			expr := fmt.Sprintf("gcp_matches_service_account(%v)", tc.param)
			compileEvalForTest(t, expr, tc.want, tc.wantErr)
		})
	}
}

func TestGCPMatchesServiceAccountID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		param   string
		want    bool
		wantErr string
	}{
		{
			name:  "shortest-valid",
			param: `"abcdef"`,
			want:  true,
		},
		{
			name:  "too-short",
			param: `"abcde"`,
			want:  false,
		},
		{
			name:  "longest-valid",
			param: `"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`,
			want:  true,
		},
		{
			name:  "too-long",
			param: `"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`,
			want:  false,
		},
		{
			name:  "must-not-begin-with-digit",
			param: `"9platform-ops"`,
			want:  false,
		},
		{
			name:  "must-not-begin-with-dash",
			param: `"-platform-ops"`,
			want:  false,
		},
		{
			name:    "type-error-number",
			param:   `42`,
			wantErr: `found no matching overload for 'gcp_matches_service_account_id' applied to '(int)'`,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			expr := fmt.Sprintf("gcp_matches_service_account_id(%v)", tc.param)
			compileEvalForTest(t, expr, tc.want, tc.wantErr)
		})
	}
}

func TestGCPMatchesProjectID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		param   string
		want    bool
		wantErr string
	}{
		{
			name:  "simple-success",
			param: `"my-project-id"`,
			want:  true,
		},
		{
			name:  "with-domain",
			param: `"example.com:my-project-123"`,
			want:  true,
		},
		{
			name:  "must-not-end-with-dash",
			param: `"my-project-id-"`,
			want:  false,
		},
		{
			name:    "type-error-number",
			param:   `42`,
			wantErr: `found no matching overload for 'gcp_matches_project_id' applied to '(int)'`,
		},
	}
	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			expr := fmt.Sprintf("gcp_matches_project_id(%v)", tc.param)
			compileEvalForTest(t, expr, tc.want, tc.wantErr)
		})
	}
}

func TestGCPMatchesProjectNumber(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		param   string
		want    bool
		wantErr string
	}{
		{
			name:  "simple-success",
			param: `"123123123123123"`,
			want:  true,
		},
		{
			name:  "no-letters-allowed",
			param: `"a123123123123123"`,
			want:  false,
		},
		{
			name:  "empty-string",
			param: `""`,
			want:  false,
		},
		{
			name:  "too-small",
			param: `"1"`,
			want:  false,
		},
		{
			name:    "type-error",
			param:   `false`,
			wantErr: `found no matching overload for 'gcp_matches_project_number' applied to '(bool)'`,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			expr := fmt.Sprintf("gcp_matches_project_number(%v)", tc.param)
			compileEvalForTest(t, expr, tc.want, tc.wantErr)
		})
	}
}

func compileEvalForTest(t *testing.T, expr string, want any, wantErr string) {
	t.Helper()

	ctx := context.Background()

	prog, err := celCompile(ctx, NewScope(nil), model.String{Val: expr})
	if diff := testutil.DiffErrString(err, wantErr); diff != "" {
		t.Fatal(diff)
	}
	if err != nil {
		return
	}

	celOut, _, err := prog.Eval(cel.NoVars())
	if diff := testutil.DiffErrString(err, wantErr); diff != "" {
		t.Fatal(diff)
	}
	if err != nil {
		return
	}

	celOutTyped, err := celOut.ConvertToNative(reflect.TypeOf(want))
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(celOutTyped, want); diff != "" {
		t.Errorf("output was not as expected (-got,+want): %s", diff)
	}
}
