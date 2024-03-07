// Copyright 2024 The Authors (see AUTHORS file)
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

package rules

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"text/tabwriter"

	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/model"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta6"
	"github.com/abcxyz/pkg/testutil"
)

func TestValidateRules(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		scope *common.Scope
		rules []*spec.Rule
		want  string
	}{
		{
			name: "rules_are_valid",
			scope: common.NewScope(map[string]string{
				"my_var": "foo",
			}, nil),
			rules: []*spec.Rule{
				{
					Rule:    model.String{Val: "size(my_var) < 5"},
					Message: model.String{Val: "Length must be less than 5"},
				},
			},
			want: "",
		},
		{
			name: "rules_are_invalid",
			scope: common.NewScope(map[string]string{
				"my_var": "foobarbaz",
			}, nil),
			rules: []*spec.Rule{
				{
					Rule:    model.String{Val: "size(my_var) < 5"},
					Message: model.String{Val: "Length must be less than 5"},
				},
			},
			want: "rules validation failed:\n\nRule:      size(my_var) < 5\nRule msg:  Length must be less than 5\n",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			got := ValidateRules(ctx, tc.scope, tc.rules)
			if diff := testutil.DiffErrString(got, tc.want); diff != "" {
				t.Errorf("unexpected result (-got, +want):\n%s", diff)
			}
		})
	}
}

func TestValidateRulesWithMessage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		scope    *common.Scope
		rules    []*spec.Rule
		callBack func(*tabwriter.Writer)
		want     string
	}{
		{
			name: "rules_valid_no_op_callback",
			scope: common.NewScope(map[string]string{
				"my_var": "foo",
			}, nil),
			rules: []*spec.Rule{
				{
					Rule:    model.String{Val: "size(my_var) < 5"},
					Message: model.String{Val: "Length must be less than 5"},
				},
			},
			callBack: func(tw *tabwriter.Writer) {},
			want:     "",
		},
		{
			name: "rules_valid_call_back_with_message",
			scope: common.NewScope(map[string]string{
				"my_var": "foo",
			}, nil),
			rules: []*spec.Rule{
				{
					Rule:    model.String{Val: "size(my_var) < 5"},
					Message: model.String{Val: "Length must be less than 5"},
				},
			},
			callBack: func(tw *tabwriter.Writer) {
				fmt.Fprintln(tw, "this will not be written since rules are valid")
			},
			want: "",
		},
		{
			name: "rules_invalid_no_op_callback",
			scope: common.NewScope(map[string]string{
				"name": "bar123",
				"age":  "1054",
			}, nil),
			rules: []*spec.Rule{
				{
					Rule:    model.String{Val: "name.matches('^[A-Za-z]+$')"},
					Message: model.String{Val: "name must only contain alphabetic characters"},
				},
				{
					Rule:    model.String{Val: "int(age) < 130"},
					Message: model.String{Val: "age must be less than 130"},
				},
			},
			callBack: func(tw *tabwriter.Writer) {},
			want: `
Rule:      name.matches('^[A-Za-z]+$')
Rule msg:  name must only contain alphabetic characters

Rule:      int(age) < 130
Rule msg:  age must be less than 130
`,
		},
		{
			name: "rules_invalid_callback_with_message",
			scope: common.NewScope(map[string]string{
				"name": "bar123",
				"age":  "1054",
			}, nil),
			rules: []*spec.Rule{
				{
					Rule:    model.String{Val: "name.matches('^[A-Za-z]+$')"},
					Message: model.String{Val: "name must only contain alphabetic characters"},
				},
				{
					Rule:    model.String{Val: "int(age) < 130"},
					Message: model.String{Val: "age must be less than 130"},
				},
			},
			callBack: func(tw *tabwriter.Writer) {
				fmt.Fprint(tw, "\nRule Violation:")
			},
			want: `
Rule Violation:
Rule:      name.matches('^[A-Za-z]+$')
Rule msg:  name must only contain alphabetic characters

Rule Violation:
Rule:      int(age) < 130
Rule msg:  age must be less than 130
`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			sb := &strings.Builder{}
			tw := tabwriter.NewWriter(sb, 8, 0, 2, ' ', 0)

			ValidateRulesWithMessage(ctx, tc.scope, tc.rules, tw, func() {
				tc.callBack(tw)
			})

			tw.Flush()
			got := sb.String()
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("unexpected result (-got, +want):\n%s", diff)
			}
		})
	}
}
