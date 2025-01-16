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
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
)

func TestActionPrint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		in             string
		inputs         map[string]string
		extraPrintVars map[string]string
		want           string
		wantErr        string
	}{
		{
			name: "simple_success",
			in:   "hello 🐕",
			want: "hello 🐕\n",
		},
		{
			name: "simple_templating",
			in:   "hello {{.name}}",
			inputs: map[string]string{
				"name": "🐕",
			},
			want: "hello 🐕\n",
		},
		{
			name:    "template_missing_input",
			in:      "hello {{.name}}",
			inputs:  map[string]string{},
			wantErr: `template referenced a nonexistent variable name "name"`,
		},
		{
			name: "flags_in_message",
			in:   "{{._flag_dest}} {{._flag_source}}",
			extraPrintVars: map[string]string{
				"_flag_source": "mysource",
				"_flag_dest":   "mydest",
			},
			want: "mydest mysource\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))
			var outBuf bytes.Buffer

			params := Params{
				Stdout: &outBuf,
			}

			sp := &stepParams{
				rp:             &params,
				scope:          common.NewScope(tc.inputs, nil),
				extraPrintVars: tc.extraPrintVars,
			}
			pr := &spec.Print{
				Message: mdl.S(tc.in),
			}
			err := actionPrint(ctx, pr, sp)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if diff := cmp.Diff(outBuf.String(), tc.want); diff != "" {
				t.Errorf("got different output than wanted (-got,+want): %s", diff)
			}
		})
	}
}
