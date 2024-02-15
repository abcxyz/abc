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

// Package goldentest implements golden test related subcommands.
package goldentest

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/abc/templates/model/goldentest/features"
	goldentest "github.com/abcxyz/abc/templates/model/goldentest/v1beta4"
	abctestutil "github.com/abcxyz/abc/templates/testutil"
	"github.com/abcxyz/pkg/testutil"
)

func TestParseTestCases(t *testing.T) {
	t.Parallel()

	invalidYaml := "bad yaml"
	validTestCase := &goldentest.Test{}

	cases := []struct {
		name         string
		testNames    []string
		filesContent map[string]string
		want         []*TestCase
		wantErr      string
	}{
		{
			name:      "specified_test_name_succeed",
			testNames: []string{"test_case_1"},
			filesContent: map[string]string{
				"testdata/golden/test_case_1/test.yaml": `api_version: 'cli.abcxyz.dev/v1beta5'
kind: 'GoldenTest'`,
			},
			want: []*TestCase{
				{
					TestName:   "test_case_1",
					TestConfig: validTestCase,
				},
			},
		},
		{
			name:      "specified_multiple_test_names_succeed",
			testNames: []string{"test_case_1", "test_case_2"},
			filesContent: map[string]string{
				"testdata/golden/test_case_1/test.yaml": `api_version: 'cli.abcxyz.dev/v1beta5'
kind: 'GoldenTest'`,
				"testdata/golden/test_case_2/test.yaml": `api_version: 'cli.abcxyz.dev/v1beta5'
kind: 'GoldenTest'`,
				"testdata/golden/test_case_3/test.yaml": `api_version: 'cli.abcxyz.dev/v1beta5'
kind: 'GoldenTest'`,
			},
			want: []*TestCase{
				{
					TestName:   "test_case_1",
					TestConfig: validTestCase,
				},
				{
					TestName:   "test_case_2",
					TestConfig: validTestCase,
				},
			},
		},
		{
			name: "all_tests_succeed",
			filesContent: map[string]string{
				"testdata/golden/test_case_1/test.yaml": `api_version: 'cli.abcxyz.dev/v1beta5'
kind: 'GoldenTest'`,
				"testdata/golden/test_case_2/test.yaml": `api_version: 'cli.abcxyz.dev/v1beta5'
kind: 'GoldenTest'`,
			},
			want: []*TestCase{
				{
					TestName:   "test_case_1",
					TestConfig: validTestCase,
				},
				{
					TestName:   "test_case_2",
					TestConfig: validTestCase,
				},
			},
		},
		{
			name: "golden_test_dir_not_exist",
			filesContent: map[string]string{
				"myfile": invalidYaml,
			},
			want:    nil,
			wantErr: "error reading golden test directory",
		},
		{
			name: "unexpected_file_in_golden_test_dir",
			filesContent: map[string]string{
				"testdata/golden/hello.txt": invalidYaml,
			},
			want:    nil,
			wantErr: "unexpected file entry under golden test directory",
		},
		{
			name: "test_does_not_have_config",
			filesContent: map[string]string{
				"testdata/golden/test_case_1/hello.txt": invalidYaml,
			},
			want:    nil,
			wantErr: "error opening test config",
		},
		{
			name: "test_bad_config",
			filesContent: map[string]string{
				"testdata/golden/test_case_1/test.yaml": invalidYaml,
			},
			want:    nil,
			wantErr: "error reading golden test config file",
		},
		{
			name:      "specified_test_name_not_found",
			testNames: []string{"test_case_2"},
			filesContent: map[string]string{
				"testdata/golden/test_case_1/test.yaml": `api_version: 'cli.abcxyz.dev/v1beta5'
kind: 'GoldenTest'`,
			},
			want:    nil,
			wantErr: "error opening test config",
		},
		{
			name:      "builtin_overrides_rejected_on_old_api_version",
			testNames: []string{"test_case_1"},
			filesContent: map[string]string{
				"testdata/golden/test_case_1/test.yaml": `api_version: 'cli.abcxyz.dev/v1beta2'
kind: 'GoldenTest'
builtin_vars:
  - name: '_git_tag'
    value: 'my-cool-tag'`,
			},
			wantErr: "does not parse and validate successfully under that version",
		},
		{
			name:      "builtin_overrides_accepted_on_api_version_at_least_v1beta3",
			testNames: []string{"test_case_1"},
			filesContent: map[string]string{
				"testdata/golden/test_case_1/test.yaml": `api_version: 'cli.abcxyz.dev/v1beta3'
kind: 'GoldenTest'
builtin_vars:
  - name: '_git_tag'
    value: 'my-cool-tag'`,
			},
			want: []*TestCase{
				{
					TestName: "test_case_1",
					TestConfig: &goldentest.Test{
						BuiltinVars: []*goldentest.VarValue{
							{
								Name:  model.String{Val: "_git_tag"},
								Value: model.String{Val: "my-cool-tag"},
							},
						},
						Features: features.Features{
							SkipStdout:     true,
							SkipABCRenamed: true,
						},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()

			abctestutil.WriteAllDefaultMode(t, tempDir, tc.filesContent)

			ctx := context.Background()
			got, err := parseTestCases(ctx, tempDir, tc.testNames)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}
			if err != nil {
				return
			}

			opt := cmpopts.IgnoreTypes(&model.ConfigPos{}, model.ConfigPos{})
			if diff := cmp.Diff(got, tc.want, opt, cmpopts.EquateEmpty()); diff != "" {
				t.Fatalf("Output test cases wasn't as expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestRenderTestCase(t *testing.T) {
	t.Parallel()

	specYaml := `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'

desc: 'A simple template'

inputs:
  - name: 'input_a'
    desc: 'input of A'
    default: 'default'

  - name: 'input_b'
    desc: 'input of B'
    default: 'default'

steps:
  - desc: 'Include some files and directories'
    action: 'include'
    params:
      paths: ['.']
`

	cases := []struct {
		name                  string
		testCase              *TestCase
		filesContent          map[string]string
		expectedGoldenContent map[string]string
		wantErr               string
	}{
		{
			name: "simple_test_succeeds",
			testCase: &TestCase{
				TestName:   "test",
				TestConfig: &goldentest.Test{},
			},
			filesContent: map[string]string{
				"spec.yaml":                      specYaml,
				"a.txt":                          "file A content",
				"b.txt":                          "file B content",
				"testdata/golden/test/test.yaml": "yaml",
			},
			expectedGoldenContent: map[string]string{
				"test.yaml":  "yaml",
				"data/a.txt": "file A content",
				"data/b.txt": "file B content",
			},
		},
		{
			name: "test_with_inputs_succeeds",
			testCase: &TestCase{
				TestName: "test",
				TestConfig: &goldentest.Test{
					Inputs: []*goldentest.VarValue{
						{
							Name:  model.String{Val: "input_a"},
							Value: model.String{Val: "a"},
						},
						{
							Name:  model.String{Val: "input_b"},
							Value: model.String{Val: "b"},
						},
					},
				},
			},
			filesContent: map[string]string{
				"spec.yaml":                      specYaml,
				"a.txt":                          "file A content",
				"b.txt":                          "file B content",
				"testdata/golden/test/test.yaml": "yaml",
			},
			expectedGoldenContent: map[string]string{
				"test.yaml":  "yaml",
				"data/a.txt": "file A content",
				"data/b.txt": "file B content",
			},
		},
		{
			name: "empty_template_output_is_valid_with_stdout",
			testCase: &TestCase{
				TestName:   "test",
				TestConfig: &goldentest.Test{},
			},
			filesContent: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'

desc: 'A template that outputs no files and do print'
steps:
  - desc: 'Print a message'
    action: 'print'
    params:
        message: 'Hello'`,
			},
			expectedGoldenContent: map[string]string{
				"data/.abc/stdout": "Hello\n",
			},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()

			abctestutil.WriteAllDefaultMode(t, tempDir, tc.filesContent)

			ctx := context.Background()
			err := renderTestCase(ctx, tempDir, tempDir, tc.testCase)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}
			if err != nil {
				return
			}

			gotDestContents := abctestutil.LoadDirWithoutMode(t, filepath.Join(tempDir, "testdata/golden/test"))
			if diff := cmp.Diff(gotDestContents, tc.expectedGoldenContent); diff != "" {
				t.Errorf("dest directory contents were not as expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestBuiltIns(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		testCase     *TestCase
		filesContent map[string]string
		want         map[string]string
		wantErr      string
	}{
		{
			name: "builtins_are_present_on_spec_api_version_v1beta3",
			testCase: &TestCase{
				TestName: "test",
				TestConfig: &goldentest.Test{
					BuiltinVars: []*goldentest.VarValue{
						{
							Name:  model.String{Val: "_git_tag"},
							Value: model.String{Val: "my-cool-tag"},
						},
					},
				},
			},
			filesContent: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta3'
kind: 'Template'

desc: 'A simple template'

steps:
- desc: 'Include some files and directories'
  action: 'include'
  params:
    paths: ['.']
- desc: 'Replace git tag placeholder'
  action: 'go_template'
  params:
    paths: ['my_file.txt']`,
				"my_file.txt": "{{._git_tag}}",
			},
			want: map[string]string{
				"data/my_file.txt": "my-cool-tag",
			},
		},
		{
			name: "builtins_are_only_in_scope_if_overridden",
			testCase: &TestCase{
				TestName:   "test",
				TestConfig: &goldentest.Test{},
			},
			filesContent: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta3'
kind: 'Template'

desc: 'A simple template'

steps:
- desc: 'Include some files and directories'
  action: 'include'
  params:
    paths: ['.']
- desc: 'Replace git tag placeholder'
  action: 'go_template'
  params:
    paths: ['my_file.txt']`,
				"my_file.txt": "{{._git_tag}}",
			},
			wantErr: `nonexistent variable name "_git_tag"`,
		},
		{
			name: "builtins_are_rejected_on_old_spec_api_version_v1beta2",
			testCase: &TestCase{
				TestName: "test",
				TestConfig: &goldentest.Test{
					BuiltinVars: []*goldentest.VarValue{
						{
							Name:  model.String{Val: "_git_tag"},
							Value: model.String{Val: "my-cool-tag"},
						},
					},
				},
			},
			filesContent: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta2'
kind: 'Template'

desc: 'A simple template'

steps:
- desc: 'Include some files and directories'
  action: 'include'
  params:
    paths: ['.']
- desc: 'Replace git tag placeholder'
  action: 'go_template'
  params:
    paths: ['my_file.txt']`,
				"my_file.txt": "{{._git_tag}}",
			},
			wantErr: "these builtin override var names are unknown and therefore invalid: [_git_tag]",
		},
		{
			name: "invalid_builtin_name_rejected",
			testCase: &TestCase{
				TestName: "test",
				TestConfig: &goldentest.Test{
					BuiltinVars: []*goldentest.VarValue{
						{
							Name:  model.String{Val: "_bad_var_name_should_fail"},
							Value: model.String{Val: "foo"},
						},
					},
				},
			},
			filesContent: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'

desc: 'A simple template'

steps:
- desc: 'Include some files and directories'
  action: 'include'
  params:
    paths: ['.']`,
			},
			wantErr: "these builtin override var names are unknown and therefore invalid: [_bad_var_name_should_fail]",
		},
		{
			name: "dest_and_src_are_overrideable_for_print",
			testCase: &TestCase{
				TestName: "test",
				TestConfig: &goldentest.Test{
					BuiltinVars: []*goldentest.VarValue{
						{
							Name:  model.String{Val: "_flag_dest"},
							Value: model.String{Val: "my-dest"},
						},
						{
							Name:  model.String{Val: "_flag_source"},
							Value: model.String{Val: "my-source"},
						},
					},
				},
			},
			filesContent: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta3'
kind: 'Template'

desc: 'A simple template'

steps:
- desc: 'Include some files and directories'
  action: 'print'
  params:
    message: '{{._flag_dest}} {{._flag_source}}'`,
			},
			want: map[string]string{
				"data/.abc/stdout": "my-dest my-source\n",
			},
		},

		{
			name: "dest_and_src_are_not_in_scope_outside_of_print_action",
			testCase: &TestCase{
				TestName: "test",
				TestConfig: &goldentest.Test{
					BuiltinVars: []*goldentest.VarValue{
						{
							Name:  model.String{Val: "_flag_dest"},
							Value: model.String{Val: "my-dest"},
						},
						{
							Name:  model.String{Val: "_flag_source"},
							Value: model.String{Val: "my-source"},
						},
					},
				},
			},
			filesContent: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta3'
kind: 'Template'

desc: 'A simple template'

steps:
- desc: 'Include some files and directories'
  action: 'include'
  params:
    paths: ['{{._flag_dest}}', '{{._flag_source}}']`,
			},
			wantErr: `the template referenced a nonexistent variable name "_flag_dest"`,
		},
		{
			name: "custom_error_message_for_builtin_needing_override",
			testCase: &TestCase{
				TestName:   "test",
				TestConfig: &goldentest.Test{},
			},
			filesContent: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta3'
kind: 'Template'
desc: 'A simple template'
steps:
- desc: 'Reference an undefined builtin'
  action: 'print'
  params:
    message: '{{._git_tag}}'`,
			},
			wantErr: `you may need to provide a value for "_git_tag" in the builtin_vars section of test.yaml`,
		},
		{
			name: "custom_error_message_for_builtin_needing_override_cel",
			testCase: &TestCase{
				TestName:   "test",
				TestConfig: &goldentest.Test{},
			},
			filesContent: map[string]string{
				"spec.yaml": `api_version: 'cli.abcxyz.dev/v1beta3'
kind: 'Template'
desc: 'A simple template'
steps:
- desc: 'Reference an undefined builtin'
  action: 'print'
  if: '_git_tag == ""'  # Should fail
  params:
    message: 'Hello'`,
			},
			wantErr: `you may need to provide a value for "_git_tag" in the builtin_vars section of test.yaml`,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()

			abctestutil.WriteAllDefaultMode(t, tempDir, tc.filesContent)

			ctx := context.Background()
			err := renderTestCase(ctx, tempDir, tempDir, tc.testCase)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}
			if err != nil {
				return
			}

			gotDestContents := abctestutil.LoadDirWithoutMode(t, filepath.Join(tempDir, "testdata/golden/test"))
			if diff := cmp.Diff(gotDestContents, tc.want); diff != "" {
				t.Errorf("dest directory contents were not as expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestRenameGitDirsAndFiles(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		filesContent map[string]string
		want         map[string]string
		wantErr      string
	}{
		{
			name: "simple_success",
			filesContent: map[string]string{
				".gitfoo/file1.txt": "foo file1",
				".git/config":       "gitconfig contents",
				".git/ref":          "gitref contents",
				".gitignore":        "gitignore contents",
				"file1.txt":         "file1",
			},
			want: map[string]string{
				".gitfoo.abc_renamed/file1.txt": "foo file1",
				".git.abc_renamed/config":       "gitconfig contents",
				".git.abc_renamed/ref":          "gitref contents",
				".gitignore.abc_renamed":        "gitignore contents",
				"file1.txt":                     "file1",
			},
		},
		{
			name: "non_root_gitignore_success",
			filesContent: map[string]string{
				"subfolder1/.gitignore": "subfolder1 gitignore contents",
				"subfolder2/.gitignore": "subfolder2 gitignore contents",
				"file1.txt":             "file1",
			},
			want: map[string]string{
				"subfolder1/.gitignore.abc_renamed": "subfolder1 gitignore contents",
				"subfolder2/.gitignore.abc_renamed": "subfolder2 gitignore contents",
				"file1.txt":                         "file1",
			},
		},
		{
			name: "nested_git_success",
			filesContent: map[string]string{
				".git/config":           "gitconfig contents",
				".git/.gitignore":       "git gitignore contents",
				"subfolder1/.gitignore": "subfolder1 gitignore contents",
				"file1.txt":             "file1",
			},
			want: map[string]string{
				".git.abc_renamed/config":                 "gitconfig contents",
				".git.abc_renamed/.gitignore.abc_renamed": "git gitignore contents",
				"subfolder1/.gitignore.abc_renamed":       "subfolder1 gitignore contents",
				"file1.txt":                               "file1",
			},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()

			abctestutil.WriteAllDefaultMode(t, tempDir, tc.filesContent)

			err := renameGitDirsAndFiles(tempDir)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}

			gotDestContents := abctestutil.LoadDirWithoutMode(t, tempDir)
			if diff := cmp.Diff(gotDestContents, tc.want); diff != "" {
				t.Errorf("dest directory contents were not as expected (-got,+want): %s", diff)
			}
		})
	}
}
