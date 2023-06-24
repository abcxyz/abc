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

package flags

import (
	"testing"

	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type testCommand struct {
	cli.BaseCommand
	AutomationFlags
}

func (t *testCommand) Flags() *cli.FlagSet {
	set := cli.NewFlagSet()
	t.AutomationFlags.AddAutomationFlags(set)
	return set
}

func TestAutomationFlags(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    []string
		want    *testCommand
		wantErr string
	}{
		{
			name: "all_flags_present",
			args: []string{
				"--no-prompt",
			},
			want: &testCommand{
				AutomationFlags: AutomationFlags{
					FlagNoPrompt: true,
				},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := &testCommand{}
			err := r.Flags().Parse(tc.args)
			if err != nil || tc.wantErr != "" {
				if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
					t.Fatal(diff)
				}
				return
			}
			opts := []cmp.Option{
				cmp.AllowUnexported(testCommand{}),
				cmpopts.IgnoreFields(testCommand{}, "BaseCommand"),
			}
			if diff := cmp.Diff(r, tc.want, opts...); diff != "" {
				t.Errorf("got %#v, want %#v, diff (-got, +want): %v", r, tc.want, diff)
			}
		})
	}
}
