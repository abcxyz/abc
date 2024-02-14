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
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	abctestutil "github.com/abcxyz/abc/templates/testutil"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
)

func TestNewTestCommand(t *testing.T) {
	t.Parallel()

	specYaml := `apiVersion: 'cli.abcxyz.dev/v1beta4'
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

	specYamlNoDefault := `apiVersion: 'cli.abcxyz.dev/v1beta4'
kind: 'Template'

desc: 'An example template that demonstrates the "print" action'

inputs:
  - name: 'name'
    desc: 'the name of the person to greet'

steps:
  - desc: 'Print a personalized message'
    action: 'print'
    params:
      message: 'Hello, {{.name}}!'
`

	testYaml := `api_version: cli.abcxyz.dev/v1beta5
kind: GoldenTest
inputs:
    - name: name
      value: Bob
builtin_vars:
    - name: _git_tag
      value: my-cool-tag
`

	cases := []struct {
		name               string
		newTestName        string
		flagInputs         map[string]string
		flagBuiltinVars    map[string]string
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
			flagBuiltinVars: map[string]string{
				"_git_tag": "my-cool-tag",
			},
			templateContents: map[string]string{
				"spec.yaml": specYaml,
			},
			expectedContents: map[string]string{
				"test.yaml": testYaml,
			},
		},
		{
			name:        "simple_test_succeeds_with_no_default_spec",
			newTestName: "new-test",
			flagInputs: map[string]string{
				"name": "Bob",
			},
			flagBuiltinVars: map[string]string{
				"_git_tag": "my-cool-tag",
			},
			templateContents: map[string]string{
				"spec.yaml": specYamlNoDefault,
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
			name:        "unknown_builtin_vars",
			newTestName: "new-test",
			flagInputs: map[string]string{
				"name": "Bob",
			},
			flagBuiltinVars: map[string]string{
				"unknown_builtin": "unknown",
			},
			templateContents: map[string]string{
				"spec.yaml": specYaml,
			},
			wantErr: "these builtin override var names are unknown and therefore invalid",
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
				"name": "John",
			},
			flagForceOverwrite: true,
			templateContents: map[string]string{
				"spec.yaml":                          specYaml,
				"testdata/golden/new-test/test.yaml": testYaml,
			},
			expectedContents: map[string]string{
				"test.yaml": `api_version: cli.abcxyz.dev/v1beta5
kind: GoldenTest
inputs:
    - name: name
      value: John
`,
			},
		},
		{
			name:        "force_overwrite_success_with_no_exist_test_yaml",
			newTestName: "new-test",
			flagInputs: map[string]string{
				"name": "Bob",
			},
			flagBuiltinVars: map[string]string{
				"_git_tag": "my-cool-tag",
			},
			flagForceOverwrite: true,
			templateContents: map[string]string{
				"spec.yaml": specYaml,
			},
			expectedContents: map[string]string{
				"test.yaml": testYaml,
			},
		},
		{
			name:        "template_with_no_inputs",
			newTestName: "new-test",
			templateContents: map[string]string{
				"spec.yaml": `apiVersion: 'cli.abcxyz.dev/v1beta4'
kind: 'Template'
desc: 'A template with no inputs'
steps:
  - desc: 'Print a message'
    action: 'print'
    params:
      message: 'Hello!'
`,
			},
			expectedContents: map[string]string{
				"test.yaml": `api_version: cli.abcxyz.dev/v1beta5
kind: GoldenTest
`,
			},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()

			abctestutil.WriteAllDefaultMode(t, tempDir, tc.templateContents)

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))

			var args []string
			for k, v := range tc.flagInputs {
				args = append(args, fmt.Sprintf("--input=%s=%s", k, v))
			}
			for k, v := range tc.flagBuiltinVars {
				args = append(args, fmt.Sprintf("--builtin-var=%s=%s", k, v))
			}
			if tc.flagForceOverwrite {
				args = append(args, "--force-overwrite")
			}
			args = append(args, tc.newTestName)
			args = append(args, tempDir)

			r := &NewTestCommand{}
			if err := r.Run(ctx, args); err != nil {
				if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
					t.Fatal(diff)
				}
				return
			}

			gotContents := abctestutil.LoadDirWithoutMode(t, filepath.Join(tempDir, "testdata/golden/", tc.newTestName))
			if diff := cmp.Diff(gotContents, tc.expectedContents); diff != "" {
				t.Errorf("dest directory contents were not as expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestNewTestFlags_Parse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    []string
		want    NewTestFlags
		wantErr string
	}{
		{
			name: "all_flags_present",
			args: []string{
				"--input", "x=y",
				"--builtin-var", "_git_tag=my-cool-tag",
				"--force-overwrite",
				"--prompt",
				"new-test",
				"/a/b/c",
			},
			want: NewTestFlags{
				NewTestName:    "new-test",
				Location:       "/a/b/c",
				Inputs:         map[string]string{"x": "y"},
				BuiltinVars:    map[string]string{"_git_tag": "my-cool-tag"},
				ForceOverwrite: true,
				Prompt:         true,
			},
		},
		{
			name: "default_location",
			args: []string{
				"--input", "x=y",
				"--builtin-var", "_git_tag=my-cool-tag",
				"--force-overwrite",
				"--prompt",
				"new-test",
			},
			want: NewTestFlags{
				NewTestName:    "new-test",
				Location:       ".",
				Inputs:         map[string]string{"x": "y"},
				BuiltinVars:    map[string]string{"_git_tag": "my-cool-tag"},
				ForceOverwrite: true,
				Prompt:         true,
			},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var cmd NewTestCommand
			cmd.SetLookupEnv(cli.MapLookuper(nil))

			err := cmd.Flags().Parse(tc.args)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}
			if diff := cmp.Diff(cmd.flags, tc.want); diff != "" {
				t.Errorf("got %#v, want %#v, diff (-got, +want): %v", cmd.flags, tc.want, diff)
			}
		})
	}
}

func TestNewTestPrompt(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		newTestName      string
		flagBuiltinVars  map[string]string
		flagPrompt       bool
		dialog           []abctestutil.DialogStep
		templateContents map[string]string
		expectedContents map[string]string
		wantErr          string
	}{
		{
			name:        "prompt_success",
			newTestName: "new-test",
			flagPrompt:  true,
			flagBuiltinVars: map[string]string{
				"_git_tag": "my-cool-tag",
			},
			templateContents: map[string]string{
				"spec.yaml": `apiVersion: 'cli.abcxyz.dev/v1beta4'
kind: 'Template'

desc: 'An example template that demonstrates the "print" action'

inputs:
  - name: 'name'
    desc: 'the name of the person to greet'

steps:
  - desc: 'Print a personalized message'
    action: 'print'
    params:
      message: 'Hello, {{.name}}!'`,
			},
			dialog: []abctestutil.DialogStep{
				{
					WaitForPrompt: `
Input name:   name
Description:  the name of the person to greet

Enter value: `,
					ThenRespond: "John\n",
				},
			},
			expectedContents: map[string]string{
				"test.yaml": `api_version: cli.abcxyz.dev/v1beta5
kind: GoldenTest
inputs:
    - name: name
      value: John
builtin_vars:
    - name: _git_tag
      value: my-cool-tag
`,
			},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()

			abctestutil.WriteAllDefaultMode(t, tempDir, tc.templateContents)

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))

			var args []string
			for k, v := range tc.flagBuiltinVars {
				args = append(args, fmt.Sprintf("--builtin-var=%s=%s", k, v))
			}
			if tc.flagPrompt {
				args = append(args, "--prompt")
			}
			args = append(args, tc.newTestName)
			args = append(args, tempDir)

			r := &NewTestCommand{}
			r.skipPromptTTYCheck = true
			stdinReader, stdinWriter := io.Pipe()
			stdoutReader, stdoutWriter := io.Pipe()
			_, stderrWriter := io.Pipe()

			r.SetStdin(stdinReader)
			r.SetStdout(stdoutWriter)
			r.SetStderr(stderrWriter)

			errCh := make(chan error)
			go func() {
				defer close(errCh)
				err := r.Run(ctx, args)
				errCh <- err
			}()

			for _, ds := range tc.dialog {
				abctestutil.ReadWithTimeout(t, stdoutReader, ds.WaitForPrompt)
				abctestutil.WriteWithTimeout(t, stdinWriter, ds.ThenRespond)
			}

			select {
			case err := <-errCh:
				if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
					t.Fatal(diff)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for background goroutine to finish")
			}
			gotContents := abctestutil.LoadDirWithoutMode(t, filepath.Join(tempDir, "testdata/golden/", tc.newTestName))
			if diff := cmp.Diff(gotContents, tc.expectedContents); diff != "" {
				t.Errorf("dest directory contents were not as expected (-got,+want): %s", diff)
			}
		})
	}
}
