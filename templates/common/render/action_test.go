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
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/errs"
	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
)

func TestWalkAndModify(t *testing.T) {
	t.Parallel()

	fooToBarVisitor := func(buf []byte) ([]byte, error) {
		return bytes.ReplaceAll(buf, []byte("foo"), []byte("bar")), nil
	}

	fooToFooFooVisitor := func(buf []byte) ([]byte, error) {
		return bytes.ReplaceAll(buf, []byte("foo"), []byte("foofoo")), nil
	}

	cases := []struct {
		name            string
		visitor         walkAndModifyVisitor
		relPaths        []string
		initialContents map[string]string
		want            map[string]string
		wantErr         string

		// fakeable errors
		readFileErr  error
		writeFileErr error
	}{
		{
			name:            "simple_single_file_replacement_should_work",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"my_file.txt"},
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc bar def"},
		},
		{
			name:            "repeated_file_only_visited_once",
			visitor:         fooToFooFooVisitor,
			relPaths:        []string{"my_file.txt", "my_file.txt"},
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc foofoo def"},
		},
		{
			name:            "repeated_file_directory_only_visited_once",
			visitor:         fooToFooFooVisitor,
			relPaths:        []string{"my_file.txt", ".", "./my_file.txt", "./", "/"},
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc foofoo def"},
		},
		{
			name:            "multiple_replacements_should_work",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"my_file.txt"},
			initialContents: map[string]string{"my_file.txt": "foo foo"}, //nolint:dupword
			want:            map[string]string{"my_file.txt": "bar bar"}, //nolint:dupword
		},
		{
			name:     "multiple_replacements_multiple_paths_should_work",
			visitor:  fooToBarVisitor,
			relPaths: []string{"my_file.txt", "b/"},
			initialContents: map[string]string{
				"my_file.txt":   "foo foo", //nolint:dupword
				"b/my_file.txt": "foo foo", //nolint:dupword
			},
			want: map[string]string{"my_file.txt": "bar bar", "b/my_file.txt": "bar bar"}, //nolint:dupword
		},
		{
			name:     "dot_dir_should_work",
			visitor:  fooToBarVisitor,
			relPaths: []string{"."},
			initialContents: map[string]string{
				"my_file.txt":       "abc foo def",
				"my_other_file.txt": "abc foo fed",
			},
			want: map[string]string{
				"my_file.txt":       "abc bar def",
				"my_other_file.txt": "abc bar fed",
			},
		},
		{
			name:            "empty_path_means_root_should_work",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"."},
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc bar def"},
		},
		{
			name:            "dot_dir_with_trailing_slash_should_work",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"./"},
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc bar def"},
		},
		{
			name:            "single_subdir_should_work",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"./dir"},
			initialContents: map[string]string{"dir/my_file.txt": "abc foo def"},
			want:            map[string]string{"dir/my_file.txt": "abc bar def"},
		},
		{
			name:            "named_file_in_subdir_should_work",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"dir/my_file.txt"},
			initialContents: map[string]string{"dir/my_file.txt": "abc foo def"},
			want:            map[string]string{"dir/my_file.txt": "abc bar def"},
		},
		{
			name:            "deeply_nested_dirs_should_work",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"dir1"},
			initialContents: map[string]string{"dir1/dir2/dir3/dir4/my_file.txt": "abc foo def"},
			want:            map[string]string{"dir1/dir2/dir3/dir4/my_file.txt": "abc bar def"},
		},
		{
			name:     "one_included_dir_one_excluded",
			visitor:  fooToBarVisitor,
			relPaths: []string{"dir1"},
			initialContents: map[string]string{
				"dir1/should_change.txt":     "abc foo def",
				"dir2/should_not_change.txt": "ghi foo jkl",
			},
			want: map[string]string{
				"dir1/should_change.txt":     "abc bar def",
				"dir2/should_not_change.txt": "ghi foo jkl",
			},
		},
		{
			name:            "nonexistent_path_should_fail",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"nonexistent"},
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc foo def"},
			wantErr:         `glob "nonexistent" did not match any files`,
		},
		{
			name:            "absolute_path_should_become_relative",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"/my_file.txt"},
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc bar def"},
		},
		{
			name:            "empty_file_should_be_ignored",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"."},
			initialContents: map[string]string{"my_file.txt": ""},
			want:            map[string]string{"my_file.txt": ""},
		},
		{
			name:            "writefile_should_not_be_called_if_contents_unchanged",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"."},
			initialContents: map[string]string{"my_file.txt": "abc"},
			want:            map[string]string{"my_file.txt": "abc"},
			writeFileErr:    fmt.Errorf("WriteFile should not have been called"),
		},
		{
			name:            "readfile_error_should_be_returned",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"."},
			initialContents: map[string]string{"my_file.txt": "foo"},
			want:            map[string]string{"my_file.txt": "foo"},
			readFileErr:     fmt.Errorf("fake error for testing"),
			wantErr:         "fake error for testing",
		},
		{
			name:            "writefile_error_should_be_returned",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"."},
			initialContents: map[string]string{"my_file.txt": "foo"},
			want:            map[string]string{"my_file.txt": "foo"},
			writeFileErr:    fmt.Errorf("fake error for testing"),
			wantErr:         "fake error for testing",
		},
		{
			name:            "simple_glob_path_should_work",
			visitor:         fooToBarVisitor,
			relPaths:        []string{"*.txt"},
			initialContents: map[string]string{"my_file.txt": "abc foo def"},
			want:            map[string]string{"my_file.txt": "abc bar def"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			scratchDir := t.TempDir()
			common.WriteAllDefaultMode(t, scratchDir, tc.initialContents)

			sp := &stepParams{
				scope:      common.NewScope(nil),
				scratchDir: scratchDir,
				rp: &Params{
					FS: &common.ErrorFS{
						FS:           &common.RealFS{},
						ReadFileErr:  tc.readFileErr,
						WriteFileErr: tc.writeFileErr,
					},
				},
			}

			relPathsPositions := make([]model.String, 0, len(tc.relPaths))

			for _, p := range tc.relPaths {
				relPathsPositions = append(relPathsPositions, model.String{Val: p})
			}

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))
			err := walkAndModify(ctx, sp, relPathsPositions, tc.visitor)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			got := common.LoadDirWithoutMode(t, scratchDir)
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("scratch directory contents were not as expected (-got,+want): %v", diff)
			}
		})
	}
}

func TestParseAndExecuteGoTmpl(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		pos               *model.ConfigPos
		tmpl              string
		inputs            map[string]string
		want              string
		wantUnknownKeyErr bool
		wantErr           string
	}{
		{
			name: "simple_success",
			pos: &model.ConfigPos{
				Line: 1,
			},
			tmpl: "{{.greeting}}, {{.greeted_entity}}!",
			inputs: map[string]string{
				"greeting":       "Hello",
				"greeted_entity": "world",
			},
			want: "Hello, world!",
		},
		{
			name: "missing_input",
			pos: &model.ConfigPos{
				Line: 1,
			},
			tmpl: "{{.my_input}}!",
			inputs: map[string]string{
				"something_else": "🥲",
			},
			wantUnknownKeyErr: true,
			wantErr:           `at line 1 column 0: template.Execute() failed: the template referenced a nonexistent variable name "my_input"; available variable names are [something_else]`,
		},
		{
			name: "unclosed_braces",
			tmpl: "Hello {{",
			inputs: map[string]string{
				"something_else": "🥲",
			},
			wantErr: `unclosed action`,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseAndExecuteGoTmpl(tc.pos, tc.tmpl, common.NewScope(tc.inputs))
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}
			if tc.wantUnknownKeyErr {
				as := &errs.UnknownVarError{}
				if ok := errors.As(err, &as); !ok {
					t.Errorf("errors.As(%T)=false, wanted true, for error %v", &as, err)
				}
			}

			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("template output was not as expected, (-got,+want): %s", diff)
			}
		})
	}
}

// These are basic tests to ensure the template functions are mounted. More
// exhaustive tests are at template_funcs_test.go.
func TestTemplateFuncs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		tmpl    string
		inputs  map[string]string
		want    string
		wantErr string
	}{
		{
			name: "contains_true",
			tmpl: `{{ contains "food" "foo" }}`,
			want: "true",
		},
		{
			name: "contains_false",
			tmpl: `{{ contains "food" "bar" }}`,
			want: "false",
		},
		{
			name: "replace",
			tmpl: `{{ replace "food" "foo" "bar" 1 }}`,
			want: "bard",
		},
		{
			name: "replaceAll",
			tmpl: `{{ replaceAll "food food food" "foo" "bar" }}`,
			want: "bard bard bard", //nolint:dupword // expected
		},
		{
			name: "sortStrings",
			tmpl: `{{ split "zebra,car,foo" "," | sortStrings }}`,
			want: "[car foo zebra]",
		},
		{
			name: "split",
			tmpl: `{{ split "a,b,c" "," }}`,
			want: "[a b c]",
		},
		{
			name: "toLower",
			tmpl: `{{ toLower "AbCD" }}`,
			want: "abcd",
		},
		{
			name: "toUpper",
			tmpl: `{{ toUpper "AbCD" }}`,
			want: "ABCD",
		},
		{
			name: "trimPrefix",
			tmpl: `{{ trimPrefix "foobarbaz" "foo" }}`,
			want: "barbaz",
		},
		{
			name: "trimSuffix",
			tmpl: `{{ trimSuffix "foobarbaz" "baz" }}`,
			want: "foobar",
		},
		{
			name: "toSnakeCase",
			tmpl: `{{ toSnakeCase "foo-bar-baz" }}`,
			want: "foo_bar_baz",
		},
		{
			name: "toLowerSnakeCase",
			tmpl: `{{ toLowerSnakeCase "foo-bar-baz" }}`,
			want: "foo_bar_baz",
		},
		{
			name: "toUpperSnakeCase",
			tmpl: `{{ toUpperSnakeCase "foo-bar-baz" }}`,
			want: "FOO_BAR_BAZ",
		},
		{
			name: "toHyphenCase",
			tmpl: `{{ toHyphenCase "foo_bar_baz" }}`,
			want: "foo-bar-baz",
		},
		{
			name: "toLowerHyphenCase",
			tmpl: `{{ toLowerHyphenCase "foo_bar_baz" }}`,
			want: "foo-bar-baz",
		},
		{
			name: "toUpperHyphenCase",
			tmpl: `{{ toUpperHyphenCase "foo-bar-baz" }}`,
			want: "FOO-BAR-BAZ",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pos := &model.ConfigPos{
				Line: 1,
			}

			got, err := parseAndExecuteGoTmpl(pos, tc.tmpl, common.NewScope(map[string]string{}))
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("template output was not as expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestProcessPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		paths     []model.String
		scope     *common.Scope
		wantPaths []model.String
		wantErr   string
	}{
		{
			name:      "verify_paths_unchanged",
			paths:     modelStrings([]string{"file1.txt", "file2.txt", "subfolder1", "subfolder2/file3.txt"}),
			scope:     common.NewScope(map[string]string{}),
			wantPaths: modelStrings([]string{"file1.txt", "file2.txt", "subfolder1", filepath.FromSlash("subfolder2/file3.txt")}),
		},
		{
			name:  "go_template_in_path",
			paths: modelStrings([]string{"{{.replace_name}}.txt"}),
			scope: common.NewScope(map[string]string{
				"replace_name": "file1",
			}),
			wantPaths: modelStrings([]string{"file1.txt"}),
		},
		{
			name:    "fail_dot_dot_relative_path",
			paths:   modelStrings([]string{"../foo.txt"}),
			scope:   common.NewScope(map[string]string{}),
			wantErr: fmt.Sprintf(`path %q must not contain ".."`, filepath.FromSlash("../foo.txt")),
		},
		{
			name: "no_escaping_glob_paths",
			paths: modelStrings([]string{
				`file\1.txt`,
			}),
			scope:   common.NewScope(map[string]string{}),
			wantErr: fmt.Sprintf(`backslashes in glob paths are not permitted: %q`, `file\1.txt`),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pathsCopy := make([]model.String, 0, len(tc.paths))

			for _, p := range tc.paths {
				pathsCopy = append(pathsCopy, model.String{
					Val: p.Val,
					Pos: p.Pos,
				})
			}
			gotPaths, err := processPaths(tc.paths, tc.scope)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}
			if diff := cmp.Diff(tc.paths, pathsCopy); diff != "" {
				t.Errorf("input paths for action should not have been changed (-got,+want): %s", diff)
			}
			if diff := cmp.Diff(gotPaths, tc.wantPaths); diff != "" {
				t.Errorf("resulting paths should match expected paths from input (-got,+want): %s", diff)
			}
		})
	}
}

func TestProcessGlobs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		dirContents    map[string]common.ModeAndContents
		paths          []model.String
		wantPaths      []model.String
		wantGlobErr    string
		wantNonGlobErr string
	}{
		{
			name: "non_glob_paths",
			dirContents: map[string]common.ModeAndContents{
				"file1.txt":            {Mode: 0o600, Contents: "file1 contents"},
				"file2.txt":            {Mode: 0o600, Contents: "file2 contents"},
				"subfolder1/file3.txt": {Mode: 0o600, Contents: "file3 contents"},
				"subfolder2/file4.txt": {Mode: 0o600, Contents: "file4 contents"},
				"subfolder2/file5.txt": {Mode: 0o600, Contents: "file5 contents"},
			},
			paths: modelStrings([]string{
				"file1.txt",
				"file2.txt",
				"subfolder1",
				"subfolder2/file4.txt",
			}),
			wantPaths: modelStrings([]string{
				"file1.txt",
				"file2.txt",
				"subfolder1",
				filepath.FromSlash("subfolder2/file4.txt"),
			}),
		},
		{
			name: "star_glob_paths",
			dirContents: map[string]common.ModeAndContents{
				"file1.txt":            {Mode: 0o600, Contents: "file1 contents"},
				"file2.txt":            {Mode: 0o600, Contents: "file2 contents"},
				"subfolder1/file3.txt": {Mode: 0o600, Contents: "file3 contents"},
				"subfolder2/file4.txt": {Mode: 0o600, Contents: "file4 contents"},
				"subfolder2/file5.txt": {Mode: 0o600, Contents: "file5 contents"},
			},
			paths: modelStrings([]string{
				"*.txt",
				"subfolder2/*.txt",
			}),
			wantPaths: modelStrings([]string{
				"file1.txt",
				"file2.txt",
				filepath.FromSlash("subfolder2/file4.txt"),
				filepath.FromSlash("subfolder2/file5.txt"),
			}),
		},
		{
			name: "glob_star_in_middle",
			dirContents: map[string]common.ModeAndContents{
				"file1.txt":            {Mode: 0o600, Contents: "file1 contents"},
				"file2.txt":            {Mode: 0o600, Contents: "file2 contents"},
				"subfolder1/file3.txt": {Mode: 0o600, Contents: "file3 contents"},
				"subfolder2/file4.txt": {Mode: 0o600, Contents: "file4 contents"},
				"subfolder2/file5.txt": {Mode: 0o600, Contents: "file5 contents"},
			},
			paths: modelStrings([]string{
				"f*e1.txt",
				"f*e2.txt",
				"sub*er2",
			}),
			wantPaths: modelStrings([]string{
				"file1.txt",
				"file2.txt",
				"subfolder2",
			}),
		},
		{
			name: "glob_star_all_paths",
			dirContents: map[string]common.ModeAndContents{
				"file1.txt":            {Mode: 0o600, Contents: "file1 contents"},
				"file2.txt":            {Mode: 0o600, Contents: "file2 contents"},
				"subfolder1/file3.txt": {Mode: 0o600, Contents: "file3 contents"},
				"subfolder2/file4.txt": {Mode: 0o600, Contents: "file4 contents"},
				"subfolder2/file5.txt": {Mode: 0o600, Contents: "file5 contents"},
			},
			paths: modelStrings([]string{"*"}),
			wantPaths: modelStrings([]string{
				"file1.txt",
				"file2.txt",
				"subfolder1",
				"subfolder2",
			}),
		},
		{
			name: "glob_star_matches_hidden_files",
			dirContents: map[string]common.ModeAndContents{
				".gitignore": {Mode: 0o600, Contents: ".gitignore contents"},
				".something": {Mode: 0o600, Contents: ".something contents"},
			},
			paths: modelStrings([]string{"*"}),
			wantPaths: modelStrings([]string{
				".gitignore",
				".something",
			}),
		},
		{
			name: "question_glob_paths",
			dirContents: map[string]common.ModeAndContents{
				"file1.txt":            {Mode: 0o600, Contents: "file1 contents"},
				"file2.txt":            {Mode: 0o600, Contents: "file2 contents"},
				"subfolder1/file3.txt": {Mode: 0o600, Contents: "file3 contents"},
				"subfolder2/file4.txt": {Mode: 0o600, Contents: "file4 contents"},
				"subfolder2/file5.txt": {Mode: 0o600, Contents: "file4 contents"},
			},
			paths: modelStrings([]string{
				"file?.txt",
				"subfolder2/file?.txt",
			}),
			wantPaths: modelStrings([]string{
				"file1.txt",
				"file2.txt",
				filepath.FromSlash("subfolder2/file4.txt"),
				filepath.FromSlash("subfolder2/file5.txt"),
			}),
		},
		{
			name: "no_glob_matches",
			paths: modelStrings([]string{
				"file_not_found.txt",
			}),
			wantGlobErr:    fmt.Sprintf(`glob %q did not match any files`, "file_not_found.txt"),
			wantNonGlobErr: fmt.Sprintf(`include path doesn't exist: %q`, "file_not_found.txt"),
		},
		{
			name: "character_range_paths",
			dirContents: map[string]common.ModeAndContents{
				"abc.txt": {Mode: 0o600, Contents: "bcd contents"},
				"xyz.txt": {Mode: 0o600, Contents: "xyz contents"},
			},
			paths: modelStrings([]string{
				"[a-c][a-c][a-c].txt",
			}),
			wantPaths: modelStrings([]string{
				"abc.txt",
			}),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// pre-populate dir contents
			tempDir := t.TempDir()
			common.WriteAll(t, tempDir, tc.dirContents)
			ctx := context.Background()

			gotPaths, err := processGlobs(ctx, tc.paths, tempDir, false) // with globbing enabled
			if diff := testutil.DiffErrString(err, tc.wantGlobErr); diff != "" {
				t.Error(diff)
			}
			if err != nil {
				return // err was expected as part of the test
			}
			if diff := testutil.DiffErrString(err, tc.wantNonGlobErr); diff != "" {
				t.Error(diff)
			}
			if err != nil {
				return // err was expected as part of the test
			}

			relGotPaths := make([]model.String, 0, len(gotPaths))
			for _, p := range gotPaths {
				relPath, err := filepath.Rel(tempDir, p.Val)
				if err != nil {
					t.Fatal(err)
				}
				relGotPaths = append(relGotPaths, model.String{
					Val: relPath,
					Pos: p.Pos,
				})
			}
			if diff := cmp.Diff(relGotPaths, tc.wantPaths); diff != "" {
				t.Errorf("resulting paths should match expected glob paths from input (-got,+want): %s", diff)
			}
		})
	}
}

func modelStrings(ss []string) []model.String {
	out := make([]model.String, len(ss))
	for i, s := range ss {
		out[i] = model.String{
			Pos: &model.ConfigPos{}, // for the purposes of testing, "location unknown" is fine.
			Val: s,
		}
	}
	return out
}
