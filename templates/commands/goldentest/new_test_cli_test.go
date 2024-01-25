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

// Package goldentest implements golden test related subcommands.
package goldentest

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
)

func TestNewTestCommand(t *testing.T) {
	t.Parallel()

	specYaml := `apiVersion: 'cli.abcxyz.dev/v1beta3'
kind: 'Template'

desc: 'An example template that demonstrates the "print" action'

inputs:
  - name: 'name'
    desc: 'the name of the person to greet'
    default: 'Alice'

steps:
  - desc: 'Print a personalized message'
    action: 'print'
    params:
      message: 'Hello, {{.name}}!'
`

	testYaml := `api_version: cli.abcxyz.dev/v1beta3
kind: GoldenTest
inputs:
    - name: name
      value: Bob
`

	cases := []struct {
		name               string
		newTestName        string
		flagInputs         map[string]string
		flagForceOverwrite bool
		templateContents   map[string]string
		expectedContents   map[string]string
		wantErr            string
	}{
		{
			name:        "simple_test_succeeds",
			newTestName: "new-test",
			flagInputs: map[string]string{
				"name": "Bob",
			},
			templateContents: map[string]string{
				"spec.yaml": specYaml,
			},
			expectedContents: map[string]string{
				"test.yaml": testYaml,
			},
		},
		{
			name:        "unknown_inputs",
			newTestName: "new-test",
			flagInputs: map[string]string{
				"unknown_input": "unknown",
			},
			templateContents: map[string]string{
				"spec.yaml": specYaml,
			},
			wantErr: "unknown input(s)",
		},
		{
			name:        "test_yaml_already_exists",
			newTestName: "new-test",
			flagInputs: map[string]string{
				"name": "Bob",
			},
			templateContents: map[string]string{
				"spec.yaml":                          specYaml,
				"testdata/golden/new-test/test.yaml": testYaml,
			},
			wantErr: "can't open file",
		},
		{
			name:        "force_overwrite_success",
			newTestName: "new-test",
			flagInputs: map[string]string{
				"name": "Alice",
			},
			flagForceOverwrite: true,
			templateContents: map[string]string{
				"spec.yaml":                          specYaml,
				"testdata/golden/new-test/test.yaml": testYaml,
			},
			expectedContents: map[string]string{
				"test.yaml": `api_version: cli.abcxyz.dev/v1beta3
kind: GoldenTest
inputs:
    - name: name
      value: Alice
`,
			},
		},
		{
			name:        "force_overwrite_success_with_no_exist_test_yaml",
			newTestName: "new-test",
			flagInputs: map[string]string{
				"name": "Bob",
			},
			flagForceOverwrite: true,
			templateContents: map[string]string{
				"spec.yaml": specYaml,
			},
			expectedContents: map[string]string{
				"test.yaml": testYaml,
			},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()

			common.WriteAllDefaultMode(t, tempDir, tc.templateContents)

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))

			var args []string
			args = append(args, fmt.Sprintf("--location=%s", tempDir))
			for k, v := range tc.flagInputs {
				args = append(args, fmt.Sprintf("--input=%s=%s", k, v))
			}
			if tc.flagForceOverwrite {
				args = append(args, "--force-overwrite")
			}
			args = append(args, tc.newTestName)

			r := &NewTestCommand{}
			if err := r.Run(ctx, args); err != nil {
				if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
					t.Fatal(diff)
				}
				return
			}

			gotContents := common.LoadDirWithoutMode(t, filepath.Join(tempDir, "testdata/golden/", tc.newTestName))
			if diff := cmp.Diff(gotContents, tc.expectedContents); diff != "" {
				t.Errorf("dest directory contents were not as expected (-got,+want): %s", diff)
			}
		})
	}
}
