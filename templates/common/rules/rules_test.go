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
	"testing"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/model"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta4"
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
			}),
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
			}),
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
