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
	"testing"

	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
)

func TestActionGoTemplate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		inputs       map[string]string
		initContents map[string]string
		gt           *model.GoTemplate
		want         map[string]string
		wantErr      string
	}{
		{
			name: "simple_success",
			inputs: map[string]string{
				"person": "Alice",
			},
			initContents: map[string]string{
				"a.txt": "Hello, {{.person}}!",
			},
			gt: &model.GoTemplate{
				Paths: modelStrings([]string{"."}),
			},
			want: map[string]string{
				"a.txt": "Hello, Alice!",
			},
		},
		{
			name:   "no_template_expressions",
			inputs: map[string]string{},
			initContents: map[string]string{
				"a.txt": "Hello, world!",
			},
			gt: &model.GoTemplate{
				Paths: modelStrings([]string{"."}),
			},
			want: map[string]string{
				"a.txt": "Hello, world!",
			},
		},
		{
			name: "multiple_template_expressions",
			inputs: map[string]string{
				"greeting": "Hello",
				"person":   "Alice",
			},
			initContents: map[string]string{
				"a.txt": "{{.greeting}}, {{.person}}!",
			},
			gt: &model.GoTemplate{
				Paths: modelStrings([]string{"."}),
			},
			want: map[string]string{
				"a.txt": "Hello, Alice!",
			},
		},
		{
			name: "missing_var",
			inputs: map[string]string{
				"something_else": "foo",
			},
			initContents: map[string]string{
				"a.txt": "Hello, {{.person}}!",
			},
			gt: &model.GoTemplate{
				Paths: modelStrings([]string{"."}),
			},
			want: map[string]string{
				"a.txt": "Hello, {{.person}}!",
			},
			wantErr: `when processing template file "a.txt": failed executing file as Go template: template.Execute() failed: the template referenced a nonexistent input variable name "person"; available variable names are [something_else]`,
		},
		{
			name:   "malformed_template",
			inputs: map[string]string{},
			initContents: map[string]string{
				"a.txt": "Hello, {{",
			},
			gt: &model.GoTemplate{
				Paths: modelStrings([]string{"."}),
			},
			want: map[string]string{
				"a.txt": "Hello, {{",
			},
			wantErr: `when processing template file "a.txt": failed executing file as Go template: error compiling as go-template: template: :1: unclosed action`, //
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			scratchDir := t.TempDir()
			if err := writeAllDefaultMode(scratchDir, tc.initContents); err != nil {
				t.Fatal(err)
			}

			ctx := context.Background()
			sp := &stepParams{
				fs:         &realFS{},
				inputs:     tc.inputs,
				scratchDir: scratchDir,
			}
			err := actionGoTemplate(ctx, tc.gt, sp)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			got := loadDirWithoutMode(t, scratchDir)
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("output differed from expected, (-got,+want): %s", diff)
			}
		})
	}
}
