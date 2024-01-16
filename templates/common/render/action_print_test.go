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
	"github.com/abcxyz/abc/templates/model"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta3"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
)

func TestActionPrint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		inputs  map[string]string
		params  *Params
		want    string
		wantErr string
	}{
		{
			name:   "simple_success",
			in:     "hello 🐕",
			params: &Params{},
			want:   "hello 🐕\n",
		},
		{
			name:   "simple_templating",
			in:     "hello {{.name}}",
			params: &Params{},
			inputs: map[string]string{
				"name": "🐕",
			},
			want: "hello 🐕\n",
		},
		{
			name:    "template_missing_input",
			in:      "hello {{.name}}",
			params:  &Params{},
			inputs:  map[string]string{},
			wantErr: `template referenced a nonexistent variable name "name"`,
		},
		{
			name: "flags_in_message",
			in:   "{{._flag_dest}} {{._flag_source}}",
			params: &Params{
				Source:  "mysource",
				DestDir: "mydest",
			},
			want: "mydest mysource\n",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))
			var outBuf bytes.Buffer

			params := *tc.params
			params.Stdout = &outBuf

			sp := &stepParams{
				rp:    &params,
				scope: common.NewScope(tc.inputs),
			}
			pr := &spec.Print{
				Message: model.String{
					Val: tc.in,
					Pos: &model.ConfigPos{},
				},
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
