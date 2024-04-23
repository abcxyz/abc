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

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/pkg/testutil"
)

func TestRootCmd(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		args       []string
		wantStdout string
		wantStderr string
		wantErr    string
	}{
		{
			name:       "render_prints_to_stdout",
			args:       []string{"render", "--input=person_name=Bob", "../../examples/templates/render/print"},
			wantStdout: "Hello, Bob!\n",
		},
		{
			name:       "old_templates_subcommand_render_prints_to_stdout",
			args:       []string{"templates", "render", "--input=person_name=Bob", "../../examples/templates/render/print"},
			wantStdout: "Hello, Bob!\n",
		},
		{
			name:    "error_return",
			args:    []string{"render", "nonexistent/dir"},
			wantErr: "isn't a valid template name",
		},
		{
			name:       "help_text",
			args:       []string{"-h"},
			wantStderr: "Usage: abc",
		},
		{
			name:    "nonexistent_subcommand",
			args:    []string{"nonexistent"},
			wantErr: `unknown command "nonexistent": run "abc -help" for a list of commands`,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			rc := rootCmd()
			_, stdout, stderr := rc.Pipe()
			err := rc.Run(ctx, tc.args)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}
			if !strings.Contains(stdout.String(), tc.wantStdout) {
				t.Errorf("stdout was not as expected (-got,+want):\n%s", cmp.Diff(stdout.String(), tc.wantStdout))
			}
			if !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Errorf("stderr was not as expected (-got,+want):\n%s", cmp.Diff(stderr.String(), tc.wantStderr))
			}
		})
	}
}
