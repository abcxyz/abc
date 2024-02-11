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
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/abc/templates/common"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta4"
	abctestutil "github.com/abcxyz/abc/templates/testutil"
	"github.com/abcxyz/pkg/testutil"
)

func TestActionGoTemplate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		inputs       map[string]string
		initContents map[string]string
		gt           *spec.GoTemplate
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
			gt: &spec.GoTemplate{
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
			gt: &spec.GoTemplate{
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
			gt: &spec.GoTemplate{
				Paths: modelStrings([]string{"."}),
			},
			want: map[string]string{
				"a.txt": "Hello, Alice!",
			},
		},
		{
			name: "path_expression",
			inputs: map[string]string{
				"greeting": "Hello",
				"person":   "Alice",
			},
			initContents: map[string]string{
				"a_Alice.txt": "{{.greeting}}, {{.person}}!",
			},
			gt: &spec.GoTemplate{
				Paths: modelStrings([]string{"a_{{.person}}.txt"}),
			},
			want: map[string]string{
				"a_Alice.txt": "Hello, Alice!",
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
			gt: &spec.GoTemplate{
				Paths: modelStrings([]string{"."}),
			},
			want: map[string]string{
				"a.txt": "Hello, {{.person}}!",
			},
			wantErr: `when processing template file "a.txt": failed executing file as Go template: template.Execute() failed: the template referenced a nonexistent variable name "person"; available variable names are [something_else]`,
		},
		{
			name:   "malformed_template",
			inputs: map[string]string{},
			initContents: map[string]string{
				"a.txt": "Hello, {{",
			},
			gt: &spec.GoTemplate{
				Paths: modelStrings([]string{"."}),
			},
			want: map[string]string{
				"a.txt": "Hello, {{",
			},
			wantErr: `when processing template file "a.txt": failed executing file as Go template: error compiling as go-template: template: :1: unclosed action`, //
		},
		{
			name: "has_functions",
			inputs: map[string]string{
				"list":   "one,two,three",
				"trim":   "  padded  ",
				"prefix": "prefixtest",
				"suffix": "testsuffix",
			},
			initContents: map[string]string{
				"replace_all.txt":  `{{ replace "my-test-project" "-" "_" -1 }}`,
				"replace_some.txt": `{{ replace "my-test-project" "-" "_" 1 }}`,
				"list.txt":         `{{ range split .list "," }}{{.}} {{end}}`,
				"trim.txt":         `{{ trimSpace .trim }}`,
				"prefix.txt":       `{{ trimPrefix .prefix "prefix" }}`,
				"suffix.txt":       `{{ trimSuffix .suffix "suffix" }}`,
			},
			gt: &spec.GoTemplate{
				Paths: modelStrings([]string{"."}),
			},
			want: map[string]string{
				"replace_all.txt":  `my_test_project`,
				"replace_some.txt": `my_test-project`,
				"list.txt":         `one two three `,
				"trim.txt":         `padded`,
				"prefix.txt":       `test`,
				"suffix.txt":       `test`,
			},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			scratchDir := t.TempDir()
			abctestutil.WriteAllDefaultMode(t, scratchDir, tc.initContents)

			ctx := context.Background()
			sp := &stepParams{
				scope:      common.NewScope(tc.inputs),
				scratchDir: scratchDir,
				rp: &Params{
					FS: &common.RealFS{},
				},
			}
			err := actionGoTemplate(ctx, tc.gt, sp)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			got := abctestutil.LoadDirWithoutMode(t, scratchDir)
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("output differed from expected, (-got,+want): %s", diff)
			}
		})
	}
}
