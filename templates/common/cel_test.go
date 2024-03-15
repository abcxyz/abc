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

package common

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/abcxyz/abc/templates/model"
	mdl "github.com/abcxyz/abc/templates/testutil/model"
	"github.com/abcxyz/pkg/testutil"
)

func TestCompileAndEvalCEL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      model.String
		vars    map[string]string
		want    any
		wantErr string
	}{
		{
			name: "simple_success",
			in:   model.String{Val: `["alligator","crocodile"]`},
			want: []string{"alligator", "crocodile"},
		},
		{
			name:    "bad_int_to_string",
			in:      mdl.S(`42`),
			want:    []string(nil),
			wantErr: `CEL expression result couldn't be converted to []string. The CEL engine error was: unsupported type conversion from 'int' to []string`,
		},
		{
			name:    "bad_list_of_int_to_list_of_string",
			in:      mdl.S(`[42]`),
			want:    []string(nil),
			wantErr: `CEL expression result couldn't be converted to []string. The CEL engine error was: unsupported type conversion from 'int' to string`,
		},
		{
			name:    "bad_heterogenous_list",
			in:      model.String{Val: `["alligator", 42]`},
			want:    []string(nil),
			wantErr: `CEL expression result couldn't be converted to []string. The CEL engine error was: unsupported type conversion from 'int' to string`,
		},
		{
			name: "string_split",
			in:   model.String{Val: `"alligator,crocodile".split(",")`},
			want: []string{"alligator", "crocodile"},
		},
		{
			name: "input_vars",
			in:   model.String{Val: `["alligator", reptile]`},
			vars: map[string]string{"reptile": "crocodile"},
			want: []string{"alligator", "crocodile"},
		},
		{
			name:    "invalid_cel_syntax",
			in:      mdl.S(`[[[[[`),
			want:    "",
			wantErr: "Syntax error: mismatched input",
		},
		{
			name: "simple_int_return",
			in:   mdl.S("42"),
			want: 42,
		},
		{
			name: "simple_uint_return",
			in:   mdl.S("42u"),
			want: uint(42),
		},
		{
			name: "simple_map_return",
			in:   model.String{Val: `{"reptile": "alligator"}`},
			want: map[string]any{"reptile": "alligator"},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			scope := NewScope(tc.vars, nil)

			// Create a new "any" variable whose type is pointer-to-the-type-of-tc.want.
			gotPtr := reflect.New(reflect.ValueOf(tc.want).Type()).Interface()
			err := CelCompileAndEval(ctx, scope, tc.in, gotPtr)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}
			if err != nil {
				return // If compilation failed, don't try to eval.
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
			name: "string_split_success",
			expr: `"foo,bar".split(",")`,
			want: []string{"foo", "bar"},
		},
		{
			name: "string_split_no_separator",
			expr: `"foo".split(",")`,
			want: []string{"foo"},
		},
		{
			name: "string_split_multiple_separators",
			expr: `"foo,bar,baz".split(",")`,
			want: []string{"foo", "bar", "baz"},
		},
		{
			name: "string_split_empty_string",
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

// TestCELMacros tests that the optional "standard macros" for CEL are enabled.
// https://github.com/google/cel-spec/blob/master/doc/langdef.md#macros
func TestCELMacros(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		expr string
		want any
	}{
		// We don't exhaustively test every macro, just pick a couple to verify.
		{
			name: "map",
			expr: `[1, 2, 3].map(n, n * n)`,
			want: []int{1, 4, 9},
		},
		{
			name: "filter",
			expr: `[1, 2, 3].filter(i, i % 2 > 0)`,
			want: []int{1, 3},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			compileEvalForTest(t, tc.expr, tc.want, "")
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
			name:  "simple_success",
			param: `"platform-ops@abcxyz-my-project.iam.gserviceaccount.com"`,
			want:  true,
		},
		{
			name:  "must_not_begin_with_digit",
			param: `"9platform-ops@abcxyz-my-project.iam.gserviceaccount.com"`,
			want:  false,
		},
		{
			name:  "must_not_begin_with_dash",
			param: `"-platform-ops@abcxyz-my-project.iam.gserviceaccount.com"`,
			want:  false,
		},
		{
			name:  "longest_valid",
			param: `"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa@abcxyz-my-project.iam.gserviceaccount.com"`,
			want:  true,
		},
		{
			name:  "too_long",
			param: `"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa@abcxyz-my-project.iam.gserviceaccount.com"`,
			want:  false,
		},
		{
			name:  "too_short",
			param: `"abcde@abcxyz-my-project.iam.gserviceaccount.com"`,
			want:  false,
		},
		{
			name:  "shortest_valid",
			param: `"abcdef@abcxyz-my-project.iam.gserviceaccount.com"`,
			want:  true,
		},
		{
			name:  "default_compute_service_account",
			param: `"824005440568-compute@developer.gserviceaccount.com"`,
			want:  true,
		},
		{
			name:  "wrong_domain",
			param: `"abcdef@abcxyz-my-project.iam.fake.biz"`,
			want:  false,
		},
		{
			name:    "type_error_number_as_service_account",
			param:   `42`,
			wantErr: `found no matching overload for 'gcp_matches_service_account' applied to '(int)'`,
		},
		{
			name:  "random_string",
			param: `"alligator"`,
			want:  false,
		},
		{
			name:  "random_email_address",
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
			name:  "shortest_valid",
			param: `"abcdef"`,
			want:  true,
		},
		{
			name:  "too_short",
			param: `"abcde"`,
			want:  false,
		},
		{
			name:  "longest_valid",
			param: `"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`,
			want:  true,
		},
		{
			name:  "too_long",
			param: `"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`,
			want:  false,
		},
		{
			name:  "must_not_begin_with_digit",
			param: `"9platform-ops"`,
			want:  false,
		},
		{
			name:  "must_not_begin_with_dash",
			param: `"-platform-ops"`,
			want:  false,
		},
		{
			name:    "type_error_number",
			param:   `42`,
			wantErr: `found no matching overload for 'gcp_matches_service_account_id' applied to '(int)'`,
		},
		{
			name:  "full_service_account_rejected",
			param: `"platform-ops@abcxyz-my-project.iam.gserviceaccount.com"`,
			want:  false,
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
			name:  "simple_success",
			param: `"my-project-id"`,
			want:  true,
		},
		{
			name:  "with_domain",
			param: `"example.com:my-project-123"`,
			want:  true,
		},
		{
			name:  "must_not_end_with_dash",
			param: `"my-project-id-"`,
			want:  false,
		},
		{
			name:  "reject_whitespace",
			param: `"my project id"`,
			want:  false,
		},
		{
			name:    "type_error_number",
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
			name:  "simple_success",
			param: `"123123123123123"`,
			want:  true,
		},
		{
			name:  "no_letters_allowed",
			param: `"a123123123123123"`,
			want:  false,
		},
		{
			name:  "empty_string",
			param: `""`,
			want:  false,
		},
		{
			name:  "too_small",
			param: `"1"`,
			want:  false,
		},
		{
			name:    "type_error",
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

func TestMatchesCapitalizedBool(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		param   string
		want    bool
		wantErr string
	}{
		{
			name:  "capitalized_true",
			param: `"True"`,
			want:  true,
		},
		{
			name:  "capitalized_false",
			param: `"False"`,
			want:  true,
		},
		{
			name:  "uncapitalized_true",
			param: `"true"`,
			want:  false,
		},
		{
			name:  "uncapitalized_false",
			param: `"false"`,
			want:  false,
		},
		{
			name:  "random_string",
			param: `"abcabc"`,
			want:  false,
		},
		{
			name:  "empty_string",
			param: `""`,
			want:  false,
		},
		{
			name:    "type_error",
			param:   `42`,
			wantErr: "found no matching overload for 'matches_capitalized_bool' applied to '(int)'",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			expr := fmt.Sprintf("matches_capitalized_bool(%v)", tc.param)
			compileEvalForTest(t, expr, tc.want, tc.wantErr)
		})
	}
}

func TestMatchesUncapitalizedBool(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		param   string
		want    bool
		wantErr string
	}{
		{
			name:  "capitalized_true",
			param: `"True"`,
			want:  false,
		},
		{
			name:  "capitalized_false",
			param: `"False"`,
			want:  false,
		},
		{
			name:  "uncapitalized_true",
			param: `"true"`,
			want:  true,
		},
		{
			name:  "uncapitalized_false",
			param: `"false"`,
			want:  true,
		},
		{
			name:  "random_string",
			param: `"abcabc"`,
			want:  false,
		},
		{
			name:  "empty_string",
			param: `""`,
			want:  false,
		},
		{
			name:    "type_error",
			param:   `42`,
			wantErr: "found no matching overload for 'matches_uncapitalized_bool' applied to '(int)'",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			expr := fmt.Sprintf("matches_uncapitalized_bool(%v)", tc.param)
			compileEvalForTest(t, expr, tc.want, tc.wantErr)
		})
	}
}

func compileEvalForTest(t *testing.T, expr string, want any, wantErr string) {
	t.Helper()

	ctx := context.Background()

	prog, err := celCompile(ctx, NewScope(nil, nil), expr)
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
