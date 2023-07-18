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
	"fmt"
	"path/filepath"
	"testing"

	"github.com/abcxyz/abc/templates/model"
	abctestutil "github.com/abcxyz/abc/testutil"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
)

func TestActionInclude(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                 string
		include              *model.Include
		templateContents     map[string]abctestutil.ModeAndContents
		destDirContents      map[string]abctestutil.ModeAndContents
		inputs               map[string]string
		flagSpec             string
		wantScratchContents  map[string]abctestutil.ModeAndContents
		wantIncludedFromDest []string
		statErr              error
		wantErr              string
	}{
		{
			name: "simple_success",
			include: &model.Include{
				Paths: modelStrings([]string{"myfile.txt"}),
			},
			templateContents: map[string]abctestutil.ModeAndContents{
				"myfile.txt": {Mode: 0o600, Contents: "my file contents"},
			},
			wantScratchContents: map[string]abctestutil.ModeAndContents{
				"myfile.txt": {Mode: 0o600, Contents: "my file contents"},
			},
		},
		{
			name: "absolute_path_treated_as_relative",
			include: &model.Include{
				Paths: modelStrings([]string{"/myfile.txt"}),
			},
			templateContents: map[string]abctestutil.ModeAndContents{
				"myfile.txt": {Mode: 0o600, Contents: "my file contents"},
			},
			wantScratchContents: map[string]abctestutil.ModeAndContents{
				"myfile.txt": {Mode: 0o600, Contents: "my file contents"},
			},
		},
		{
			name: "reject_dot_dot",
			include: &model.Include{
				Paths: modelStrings([]string{"../file.txt"}),
			},
			wantErr: `path "../file.txt" must not contain ".."`,
		},
		{
			name: "templated_filename_success",
			include: &model.Include{
				Paths: modelStrings([]string{"{{.my_dir}}/{{.my_file}}"}),
			},
			templateContents: map[string]abctestutil.ModeAndContents{
				"foo/bar.txt": {Mode: 0o600, Contents: "file contents"},
			},
			inputs: map[string]string{
				"my_dir":  "foo",
				"my_file": "bar.txt",
			},
			wantScratchContents: map[string]abctestutil.ModeAndContents{
				"foo/bar.txt": {Mode: 0o600, Contents: "file contents"},
			},
		},
		{
			name: "including_multiple_times_should_succeed",
			include: &model.Include{
				Paths: modelStrings([]string{"myfile.txt", "myfile.txt"}),
			},
			templateContents: map[string]abctestutil.ModeAndContents{
				"myfile.txt": {Mode: 0o600, Contents: "my file contents"},
			},
			wantScratchContents: map[string]abctestutil.ModeAndContents{
				"myfile.txt": {Mode: 0o600, Contents: "my file contents"},
			},
		},
		{
			name: "templated_filename_nonexistent_input_var_should_fail",
			include: &model.Include{
				Paths: modelStrings([]string{"{{.filename}}"}),
			},
			templateContents: map[string]abctestutil.ModeAndContents{
				"myfile.txt": {Mode: 0o600, Contents: "file contents"},
			},
			inputs:  map[string]string{},
			wantErr: `nonexistent input variable name "filename"`,
		},
		{
			name: "nonexistent_source_should_fail",
			include: &model.Include{
				Paths: modelStrings([]string{"nonexistent"}),
			},
			templateContents: map[string]abctestutil.ModeAndContents{
				"myfile.txt": {Mode: 0o600, Contents: "file contents"},
			},
			wantErr: `include path doesn't exist: "nonexistent"`,
		},
		{
			// Note: we don't exhaustively test every possible FS error here. That's
			// already done in the tests for the underlying copyRecursive function.
			name: "filesystem_error_should_be_returned",
			include: &model.Include{
				Paths: modelStrings([]string{"myfile.txt"}),
			},
			templateContents: map[string]abctestutil.ModeAndContents{
				"myfile.txt": {Mode: 0o600, Contents: "my file contents"},
			},
			statErr: fmt.Errorf("fake error"),
			wantErr: "fake error",
		},
		{
			name: "strip_prefix_from_file",
			include: &model.Include{
				Paths:       modelStrings([]string{"a/deep/subdir/hello.txt"}),
				StripPrefix: model.String{Val: "a/deep/subdir"},
			},
			templateContents: map[string]abctestutil.ModeAndContents{
				"a/deep/subdir/hello.txt": {Mode: 0o600, Contents: "my file contents"},
			},
			wantScratchContents: map[string]abctestutil.ModeAndContents{
				"hello.txt": {Mode: 0o600, Contents: "my file contents"},
			},
		},
		{
			name: "strip_prefix_from_dir",
			include: &model.Include{
				Paths:       modelStrings([]string{"a/deep/subdir/hello.txt"}),
				StripPrefix: model.String{Val: "a/deep"},
			},
			templateContents: map[string]abctestutil.ModeAndContents{
				"a/deep/subdir/hello.txt": {Mode: 0o600, Contents: "my file contents"},
			},
			wantScratchContents: map[string]abctestutil.ModeAndContents{
				"subdir/hello.txt": {Mode: 0o600, Contents: "my file contents"},
			},
		},
		{
			name: "strip_and_add_prefix_together_with_templates",
			include: &model.Include{
				Paths:       modelStrings([]string{"a/deep/subdir/hello.txt"}),
				StripPrefix: model.String{Val: "{{.ay}}/"},
				AddPrefix:   model.String{Val: "{{.bee}}/"},
			},
			inputs: map[string]string{
				"ay":  "a",
				"bee": "b",
			},
			templateContents: map[string]abctestutil.ModeAndContents{
				"a/deep/subdir/hello.txt": {Mode: 0o600, Contents: "my file contents"},
			},
			wantScratchContents: map[string]abctestutil.ModeAndContents{
				"b/deep/subdir/hello.txt": {Mode: 0o600, Contents: "my file contents"},
			},
		},
		{
			name: "as_with_single_path",
			include: &model.Include{
				Paths: modelStrings([]string{"dir1/file1.txt"}),
				As:    modelStrings([]string{"dir2/file2.txt"}),
			},
			templateContents: map[string]abctestutil.ModeAndContents{
				"dir1/file1.txt": {Mode: 0o600, Contents: "my file contents"},
			},
			wantScratchContents: map[string]abctestutil.ModeAndContents{
				"dir2/file2.txt": {Mode: 0o600, Contents: "my file contents"},
			},
		},
		{
			name: "as_with_multiple_paths_and_templates",
			include: &model.Include{
				Paths: modelStrings([]string{"file{{.one}}.txt", "file{{.two}}.txt"}),
				As:    modelStrings([]string{"file{{.three}}.txt", "file{{.four}}.txt"}),
			},
			inputs: map[string]string{
				"one":   "1",
				"two":   "2",
				"three": "3",
				"four":  "4",
			},
			templateContents: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "my file contents"},
				"file2.txt": {Mode: 0o600, Contents: "my file contents"},
			},
			wantScratchContents: map[string]abctestutil.ModeAndContents{
				"file3.txt": {Mode: 0o600, Contents: "my file contents"},
				"file4.txt": {Mode: 0o600, Contents: "my file contents"},
			},
		},
		{
			name: "strip_prefix_doesnt_find_prefix",
			include: &model.Include{
				Paths:       modelStrings([]string{"a/b/c"}),
				StripPrefix: model.String{Val: "x/"},
			},
			templateContents: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "my file contents"},
			},
			wantErr: "wasn't a prefix of the actual path",
		},
		{
			name: "spec_yaml_should_be_skipped",
			include: &model.Include{
				Paths: modelStrings([]string{"."}),
			},
			flagSpec: "spec.yaml",
			templateContents: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "my file contents"},
				"spec.yaml": {Mode: 0o600, Contents: "spec contents"},
			},
			wantScratchContents: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "my file contents"},
			},
		},
		{
			name: "spec_yaml_in_subdir_should_not_be_skipped",
			include: &model.Include{
				Paths: modelStrings([]string{"."}),
			},
			flagSpec: "spec.yaml",
			templateContents: map[string]abctestutil.ModeAndContents{
				"file1.txt":        {Mode: 0o600, Contents: "my file contents"},
				"subdir/spec.yaml": {Mode: 0o600, Contents: "spec contents"},
			},
			wantScratchContents: map[string]abctestutil.ModeAndContents{
				"file1.txt":        {Mode: 0o600, Contents: "my file contents"},
				"subdir/spec.yaml": {Mode: 0o600, Contents: "spec contents"},
			},
		},
		{
			name: "include_dot_from_destination",
			include: &model.Include{
				Paths: modelStrings([]string{"."}),
				From:  model.String{Val: "destination"},
			},
			templateContents: map[string]abctestutil.ModeAndContents{
				"spec.yaml": {Mode: 0o600, Contents: "spec contents"},
			},
			destDirContents: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "file1 contents"},
			},
			wantScratchContents: map[string]abctestutil.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "file1 contents"},
			},
			wantIncludedFromDest: []string{"file1.txt"},
		},
		{
			name: "include_subdir_from_destination",
			include: &model.Include{
				Paths: modelStrings([]string{"subdir"}),
				From:  model.String{Val: "destination"},
			},
			templateContents: map[string]abctestutil.ModeAndContents{
				"spec.yaml": {Mode: 0o600, Contents: "spec contents"},
			},
			destDirContents: map[string]abctestutil.ModeAndContents{
				"file1.txt":        {Mode: 0o600, Contents: "file1 contents"},
				"subdir/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
			wantScratchContents: map[string]abctestutil.ModeAndContents{
				"subdir/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
			wantIncludedFromDest: []string{"subdir/file2.txt"},
		},
		{
			name: "include_individual_files_from_destination",
			include: &model.Include{
				Paths: modelStrings([]string{"file1.txt", "subdir/file2.txt"}),
				From:  model.String{Val: "destination"},
			},
			templateContents: map[string]abctestutil.ModeAndContents{
				"spec.yaml": {Mode: 0o600, Contents: "spec contents"},
			},
			destDirContents: map[string]abctestutil.ModeAndContents{
				"file1.txt":        {Mode: 0o600, Contents: "file1 contents"},
				"subdir/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
			wantScratchContents: map[string]abctestutil.ModeAndContents{
				"file1.txt":        {Mode: 0o600, Contents: "file1 contents"},
				"subdir/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
			wantIncludedFromDest: []string{"file1.txt", "subdir/file2.txt"},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))

			// Convert to OS-specific paths
			abctestutil.KeysToPlatformPaths(
				tc.templateContents,
				tc.wantScratchContents,
			)
			abctestutil.ToPlatformPaths(tc.wantIncludedFromDest)

			tempDir := t.TempDir()
			templateDir := filepath.Join(tempDir, templateDirNamePart)
			scratchDir := filepath.Join(tempDir, scratchDirNamePart)
			destDir := filepath.Join(tempDir, "dest")

			abctestutil.WriteAll(t, templateDir, tc.templateContents)
			// For testing "include from destination"
			abctestutil.WriteAll(t, destDir, tc.destDirContents)

			sp := &stepParams{
				flags: &RenderFlags{
					Spec: tc.flagSpec,
					Dest: destDir,
				},
				fs: &errorFS{
					renderFS: &realFS{},
					statErr:  tc.statErr,
				},
				scratchDir:  scratchDir,
				templateDir: templateDir,
				inputs:      tc.inputs,
			}
			err := actionInclude(ctx, tc.include, sp)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			gotTemplateContents := abctestutil.LoadDirContents(t, filepath.Join(tempDir, templateDirNamePart))
			if diff := cmp.Diff(gotTemplateContents, tc.templateContents, cmpFileMode); diff != "" {
				t.Errorf("template directory should not have been touched (-got,+want): %s", diff)
			}

			gotScratchContents := abctestutil.LoadDirContents(t, filepath.Join(tempDir, scratchDirNamePart))
			if diff := cmp.Diff(gotScratchContents, tc.wantScratchContents, cmpFileMode); diff != "" {
				t.Errorf("scratch directory contents were not as expected (-got,+want): %s", diff)
			}

			if diff := cmp.Diff(sp.includedFromDest, tc.wantIncludedFromDest); diff != "" {
				t.Errorf("includedFromDest was not as expected (-got,+want): %s", diff)
			}
		})
	}
}
