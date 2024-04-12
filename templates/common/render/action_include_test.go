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
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/tempdir"
	"github.com/abcxyz/abc/templates/model"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta6"
	abctestutil "github.com/abcxyz/abc/templates/testutil"
	mdl "github.com/abcxyz/abc/templates/testutil/model"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
)

func TestActionInclude(t *testing.T) {
	t.Parallel()

	const destDirBaseName = "dest"

	cases := []struct {
		name                 string
		include              *spec.Include
		templateContents     map[string]string
		destDirContents      map[string]string
		inputs               map[string]string
		ignorePatterns       []model.String
		wantScratchContents  map[string]string
		wantIncludedFromDest map[string]string
		statErr              error
		wantErr              string
	}{
		{
			name: "simple_success",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("myfile.txt"),
					},
				},
			},
			templateContents: map[string]string{
				"myfile.txt": "my file contents",
			},
			wantScratchContents: map[string]string{
				"myfile.txt": "my file contents",
			},
		},
		{
			name: "absolute_path_treated_as_relative",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("/myfile.txt"),
					},
				},
			},
			templateContents: map[string]string{
				"myfile.txt": "my file contents",
			},
			wantScratchContents: map[string]string{
				"myfile.txt": "my file contents",
			},
		},
		{
			name: "reject_dot_dot",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("../file.txt"),
					},
				},
			},
			wantErr: `path "../file.txt" must not contain ".."`,
		},
		{
			name: "reject_dot_dot_glob",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("../*.txt"),
					},
				},
			},
			wantErr: `path "../*.txt" must not contain ".."`,
		},
		{
			name: "templated_filename_success",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("{{.my_dir}}/{{.my_file}}"),
					},
				},
			},
			templateContents: map[string]string{
				"foo/bar.txt": "file contents",
			},
			inputs: map[string]string{
				"my_dir":  "foo",
				"my_file": "bar.txt",
			},
			wantScratchContents: map[string]string{
				"foo/bar.txt": "file contents",
			},
		},
		{
			name: "including_multiple_times_should_succeed",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("myfile.txt", "myfile.txt"),
					},
				},
			},
			templateContents: map[string]string{
				"myfile.txt": "my file contents",
			},
			wantScratchContents: map[string]string{
				"myfile.txt": "my file contents",
			},
		},
		{
			name: "including_multiple_times_should_succeed",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("foo/myfile.txt", "foo/", "foo/myfile.txt"),
					},
				},
			},
			templateContents: map[string]string{
				"foo/myfile.txt": "my file contents",
			},
			wantScratchContents: map[string]string{
				"foo/myfile.txt": "my file contents",
			},
		},
		{
			name: "templated_filename_nonexistent_input_var_should_fail",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("{{.filename}}"),
					},
				},
			},
			templateContents: map[string]string{
				"myfile.txt": "file contents",
			},
			inputs:  map[string]string{},
			wantErr: `nonexistent variable name "filename"`,
		},
		{
			name: "nonexistent_source_should_fail",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("nonexistent"),
					},
				},
			},
			templateContents: map[string]string{
				"myfile.txt": "my file contents",
			},
			wantErr: `glob "nonexistent" did not match any files`,
		},
		{
			// Note: we don't exhaustively test every possible FS error here. That's
			// already done in the tests for the underlying copyRecursive function.
			name: "filesystem_error_should_be_returned",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("myfile.txt"),
					},
				},
			},
			templateContents: map[string]string{
				"myfile.txt": "my file contents",
			},
			statErr: fmt.Errorf("fake error"),
			wantErr: "fake error",
		},
		{
			name: "simple_glob_path",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("*.txt"),
					},
				},
			},
			templateContents: map[string]string{
				"file1.txt": "file1 contents",
				"file2.txt": "file2 contents",
				"file3.txt": "file3 contents",
			},
			wantScratchContents: map[string]string{
				"file1.txt": "file1 contents",
				"file2.txt": "file2 contents",
				"file3.txt": "file3 contents",
			},
		},
		{
			name: "glob_dir",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("dir*"),
					},
				},
			},
			templateContents: map[string]string{
				"dir1/file1.txt": "file1 contents",
				"dir2/file2.txt": "file2 contents",
			},
			wantScratchContents: map[string]string{
				"dir1/file1.txt": "file1 contents",
				"dir2/file2.txt": "file2 contents",
			},
		},
		{
			name: "glob_dir_and_files",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("dir*"),
					},
				},
			},
			templateContents: map[string]string{
				"directive.txt":  "directive file contents",
				"director.txt":   "director file contents",
				"dir1/file1.txt": "file1 contents",
				"dir2/file2.txt": "file2 contents",
			},
			wantScratchContents: map[string]string{
				"directive.txt":  "directive file contents",
				"director.txt":   "director file contents",
				"dir1/file1.txt": "file1 contents",
				"dir2/file2.txt": "file2 contents",
			},
		},
		{
			name: "glob_in_subdir",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("dir/*.txt"),
					},
				},
			},
			templateContents: map[string]string{
				"dont_include.txt":  "dont_include contents",
				"dont/include2.txt": "dont_include2 contents",
				"dir/file1.txt":     "file1 contents",
				"dir/file2.txt":     "file2 contents",
			},
			wantScratchContents: map[string]string{
				"dir/file1.txt": "file1 contents",
				"dir/file2.txt": "file2 contents",
			},
		},
		{
			name: "go_template_to_glob",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("{{.filename}}.*"),
					},
				},
			},
			inputs: map[string]string{
				"filename": "file",
			},
			templateContents: map[string]string{
				"file.txt":  "txt file contents",
				"file.md":   "md file contents",
				"file.json": "json file contents",
			},
			wantScratchContents: map[string]string{
				"file.txt":  "txt file contents",
				"file.md":   "md file contents",
				"file.json": "json file contents",
			},
		},
		{
			name: "as_with_single_path",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("dir1/file1.txt"),
						As:    mdl.Strings("dir2/file2.txt"),
					},
				},
			},
			templateContents: map[string]string{
				"dir1/file1.txt": "my file contents",
			},
			wantScratchContents: map[string]string{
				"dir2/file2.txt": "my file contents",
			},
		},
		{
			name: "as_with_single_glob_path",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("*.txt"),
						As:    mdl.Strings("dir"),
					},
				},
			},
			templateContents: map[string]string{
				"file1.txt": "file1 contents",
			},
			wantScratchContents: map[string]string{
				"dir/file1.txt": "file1 contents",
			},
		},
		{
			name: "as_with_multiple_glob_paths",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("*.txt"),
						As:    mdl.Strings("dir"),
					},
				},
			},
			templateContents: map[string]string{
				"file1.txt": "file1 contents",
				"file2.txt": "file2 contents",
			},
			wantScratchContents: map[string]string{
				"dir/file1.txt": "file1 contents",
				"dir/file2.txt": "file2 contents",
			},
		},
		{
			name: "multiple_as_with_glob_paths",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("*.txt", "*.md"),
						As:    mdl.Strings("txtdir", "mddir"),
					},
				},
			},
			templateContents: map[string]string{
				"file1.txt": "file1 contents",
				"file2.txt": "file2 contents",
				"file3.md":  "file3 contents",
				"file4.md":  "file4 contents",
			},
			wantScratchContents: map[string]string{
				"txtdir/file1.txt": "file1 contents",
				"txtdir/file2.txt": "file2 contents",
				"mddir/file3.md":   "file3 contents",
				"mddir/file4.md":   "file4 contents",
			},
		},
		{
			name: "as_with_glob_dir",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("dir*"),
						As:    mdl.Strings("topdir"),
					},
				},
			},
			templateContents: map[string]string{
				"dir1/file1.txt": "file1 contents",
				"dir2/file2.txt": "file2 contents",
			},
			wantScratchContents: map[string]string{
				"topdir/dir1/file1.txt": "file1 contents",
				"topdir/dir2/file2.txt": "file2 contents",
			},
		},
		{
			name: "as_with_multiple_paths_and_templates",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("file{{.one}}.txt", "file{{.two}}.txt"),
						As:    mdl.Strings("file{{.three}}.txt", "file{{.four}}.txt"),
					},
				},
			},
			inputs: map[string]string{
				"one":   "1",
				"two":   "2",
				"three": "3",
				"four":  "4",
			},
			templateContents: map[string]string{
				"file1.txt": "my file contents",
				"file2.txt": "my file contents",
			},
			wantScratchContents: map[string]string{
				"file3.txt": "my file contents",
				"file4.txt": "my file contents",
			},
		},
		{
			name: "spec_yaml_should_be_skipped",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("."),
					},
				},
			},
			templateContents: map[string]string{
				"file1.txt":                 "my file contents",
				"spec.yaml":                 "spec contents",
				"testdata/golden/test.yaml": "some yaml",
			},
			wantScratchContents: map[string]string{
				"file1.txt": "my file contents",
			},
		},
		{
			name: "spec_yaml_in_subdir_should_not_be_skipped",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("."),
					},
				},
			},
			templateContents: map[string]string{
				"file1.txt":        "my file contents",
				"subdir/spec.yaml": "spec contents",
			},
			wantScratchContents: map[string]string{
				"file1.txt":        "my file contents",
				"subdir/spec.yaml": "spec contents",
			},
		},
		{
			name: "golden_test_in_subdir_should_not_be_skipped",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("."),
					},
				},
			},
			templateContents: map[string]string{
				"file1.txt":                        "my file contents",
				"spec.yaml":                        "spec contents",
				"subdir/testdata/golden/test.yaml": "some yaml",
			},
			wantScratchContents: map[string]string{
				"file1.txt":                        "my file contents",
				"subdir/testdata/golden/test.yaml": "some yaml",
			},
		},
		{
			name: "skip_file",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("."),
						Skip:  mdl.Strings("file2.txt"),
					},
				},
			},
			templateContents: map[string]string{
				"file1.txt":                 "file 1 contents",
				"file2.txt":                 "file 2 contents",
				"subfolder/file3.txt":       "file 3 contents",
				"spec.yaml":                 "spec contents",
				"testdata/golden/test.yaml": "some yaml",
			},
			wantScratchContents: map[string]string{
				"file1.txt":           "file 1 contents",
				"subfolder/file3.txt": "file 3 contents",
			},
		},
		{
			name: "skip_single_path_file",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("file1.txt"),
						Skip:  mdl.Strings("file1.txt"),
					},
				},
			},
			templateContents: map[string]string{
				"file1.txt": "file 1 contents",
			},
			wantScratchContents: nil,
		},
		{
			name: "skip_multiple_files",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("."),
						Skip:  mdl.Strings("file1.txt", "file2.txt", "file4.txt"),
					},
				},
			},
			templateContents: map[string]string{
				"file1.txt":                 "file 1 contents",
				"file2.txt":                 "file 2 contents",
				"file3.txt":                 "file 3 contents",
				"file4.txt":                 "file 4 contents",
				"spec.yaml":                 "spec contents",
				"testdata/golden/test.yaml": "some yaml",
			},
			wantScratchContents: map[string]string{
				"file3.txt": "file 3 contents",
			},
		},
		{
			name: "skip_multiple_files_globbing",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("*.txt"),
						Skip:  mdl.Strings("file1.txt", "file2.txt", "file4.txt"),
					},
				},
			},
			templateContents: map[string]string{
				"file1.txt":                 "file 1 contents",
				"file2.txt":                 "file 2 contents",
				"file3.txt":                 "file 3 contents",
				"file4.txt":                 "file 4 contents",
				"spec.yaml":                 "spec contents",
				"testdata/golden/test.yaml": "some yaml",
			},
			wantScratchContents: map[string]string{
				"file3.txt": "file 3 contents",
			},
		},
		{
			name: "skip_file_in_subfolder",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("subfolder"),
						Skip:  mdl.Strings("subfolder/file2.txt"),
					},
				},
			},
			templateContents: map[string]string{
				"file1.txt":                 "file 1 contents",
				"subfolder/file2.txt":       "file 2 contents",
				"subfolder/file3.txt":       "file 3 contents",
				"spec.yaml":                 "spec contents",
				"testdata/golden/test.yaml": "some yaml",
			},
			wantScratchContents: map[string]string{
				"subfolder/file3.txt": "file 3 contents",
			},
		},
		{
			name: "skip_glob_in_directory",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("subfolder"),
						Skip:  mdl.Strings("subfolder/*.txt"),
					},
				},
			},
			templateContents: map[string]string{
				"subfolder/skip1.txt":       "skip 1 contents",
				"subfolder/skip2.txt":       "skip 2 contents",
				"subfolder/include.md":      "include contents",
				"spec.yaml":                 "spec contents",
				"testdata/golden/test.yaml": "some yaml",
			},
			wantScratchContents: map[string]string{
				"subfolder/include.md": "include contents",
			},
		},
		{
			name: "skip_directory",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("folder1"),
						Skip:  mdl.Strings("folder1/folder2"),
					},
				},
			},
			templateContents: map[string]string{
				"folder1/file1.txt":         "file 1 contents",
				"folder1/folder2/file2.txt": "file 2 contents",
				"folder1/folder3/file3.txt": "file 2 contents",
				"spec.yaml":                 "spec contents",
				"testdata/golden/test.yaml": "some yaml",
			},
			wantScratchContents: map[string]string{
				"folder1/file1.txt":         "file 1 contents",
				"folder1/folder3/file3.txt": "file 2 contents",
			},
		},
		{
			name: "include_dot_from_destination",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("."),
						From:  mdl.S("destination"),
					},
				},
			},
			templateContents: map[string]string{
				"spec.yaml":                 "spec contents",
				"testdata/golden/test.yaml": "some yaml",
			},
			destDirContents: map[string]string{
				"file1.txt": "file1 contents",
			},
			wantScratchContents: map[string]string{
				"file1.txt": "file1 contents",
			},
			wantIncludedFromDest: map[string]string{"file1.txt": destDirBaseName},
		},
		{
			name: "include_subdir_from_destination",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("subdir"),
						From:  mdl.S("destination"),
					},
				},
			},
			templateContents: map[string]string{
				"spec.yaml":                 "spec contents",
				"testdata/golden/test.yaml": "some yaml",
			},
			destDirContents: map[string]string{
				"file1.txt":        "file1 contents",
				"subdir/file2.txt": "file2 contents",
			},
			wantScratchContents: map[string]string{
				"subdir/file2.txt": "file2 contents",
			},
			wantIncludedFromDest: map[string]string{"subdir/file2.txt": destDirBaseName},
		},
		{
			name: "include_glob_from_destination",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("*.txt"),
						From:  mdl.S("destination"),
					},
				},
			},
			templateContents: map[string]string{
				"spec.yaml":                 "spec contents",
				"testdata/golden/test.yaml": "some yaml",
			},
			destDirContents: map[string]string{
				"file1.txt":        "file1 contents",
				"file2.txt":        "file1 contents",
				"subdir/file3.txt": "file2 contents",
			},
			wantScratchContents: map[string]string{
				"file1.txt": "file1 contents",
				"file2.txt": "file1 contents",
			},
			wantIncludedFromDest: map[string]string{
				"file1.txt": destDirBaseName,
				"file2.txt": destDirBaseName,
			},
		},
		{
			name: "include_individual_files_from_destination",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("file1.txt", "subdir/file2.txt"),
						From:  mdl.S("destination"),
					},
				},
			},
			templateContents: map[string]string{
				"spec.yaml":                 "spec contents",
				"testdata/golden/test.yaml": "some yaml",
			},
			destDirContents: map[string]string{
				"file1.txt":        "file1 contents",
				"subdir/file2.txt": "file2 contents",
			},
			wantScratchContents: map[string]string{
				"file1.txt":        "file1 contents",
				"subdir/file2.txt": "file2 contents",
			},
			wantIncludedFromDest: map[string]string{
				"file1.txt":        destDirBaseName,
				"subdir/file2.txt": destDirBaseName,
			},
		},
		{
			name: "skip_paths_with_custom_ignore",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("."),
					},
					{
						Paths: mdl.Strings("."),
						From:  mdl.S("destination"),
					},
				},
			},
			ignorePatterns: mdl.Strings("folder0", "file1.txt", "folder1/folder2"),
			templateContents: map[string]string{
				"folder0/file0.txt":                 "file 0 contents",
				"folder1/folder0/file0.txt":         "file 0 contents",
				"folder1/folder1/folder0/file0.txt": "file 0 contents",
				"file1.txt":                         "file 1 contents",
				"folder1/file1.txt":                 "file 1 contents",
				"folder1/folder1/file1.txt":         "file 1 contents",
				"folder1/folder2/file2.txt":         "file 2 contents",
				"folder1/folder3/file3.txt":         "file 3 contents",
			},
			destDirContents: map[string]string{
				"folder0/file0.txt":                 "file0 contents",
				"folder1/folder0/file0.txt":         "file0 contents",
				"folder1/folder1/folder0/file0.txt": "file 0 contents",
				"file1.txt":                         "file1 contents",
				"folder1/file1.txt":                 "file1 contents",
				"folder1/folder2/file2.txt":         "file2 contents",
				"file2.txt":                         "file2 contents",
			},
			wantScratchContents: map[string]string{
				"folder1/folder3/file3.txt": "file 3 contents",
				"file2.txt":                 "file2 contents",
			},
			wantIncludedFromDest: map[string]string{"file2.txt": destDirBaseName},
		},
		{
			name: "skip_paths_with_custom_ignore_glob",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("."),
					},
					{
						Paths: mdl.Strings("."),
						From:  mdl.S("destination"),
					},
				},
			},
			ignorePatterns: mdl.Strings("*older0", "*1.txt", "folder1/*2"),
			templateContents: map[string]string{
				"folder0/file0.txt":                 "file 0 contents",
				"folder1/folder0/file0.txt":         "file 0 contents",
				"folder1/folder1/folder0/file0.txt": "file 0 contents",
				"file1.txt":                         "file 1 contents",
				"folder1/file1.txt":                 "file 1 contents",
				"folder1/folder1/file1.txt":         "file 1 contents",
				"folder1/folder2/file2.txt":         "file 2 contents",
				"folder1/folder3/file3.txt":         "file 3 contents",
			},
			destDirContents: map[string]string{
				"folder0/file0.txt":                 "file0 contents",
				"folder1/folder0/file0.txt":         "file0 contents",
				"folder1/folder1/folder0/file0.txt": "file0 contents",
				"file1.txt":                         "file1 contents",
				"folder1/file1.txt":                 "file1 contents",
				"folder1/folder1/file1.txt":         "file1 contents",
				"folder1/folder2/file2.txt":         "file2 contents",
				"file2.txt":                         "file2 contents",
			},
			wantScratchContents: map[string]string{
				"folder1/folder3/file3.txt": "file 3 contents",
				"file2.txt":                 "file2 contents",
			},
			wantIncludedFromDest: map[string]string{"file2.txt": destDirBaseName},
		},
		{
			name: "skip_paths_with_custom_ignore_leading_slash",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("."),
					},
					{
						Paths: mdl.Strings("."),
						From:  mdl.S("destination"),
					},
				},
			},
			ignorePatterns: mdl.Strings("/folder0", "/file1.txt", "/folder1/folder2"),
			templateContents: map[string]string{
				"folder0/file0.txt":                 "file 0 contents",
				"folder1/folder0/file0.txt":         "file 0 contents",
				"folder1/folder1/folder0/file0.txt": "file 0 contents",
				"file1.txt":                         "file 1 contents",
				"folder1/file1.txt":                 "file 1 contents",
				"folder1/folder1/file1.txt":         "file 1 contents",
				"folder1/folder2/file2.txt":         "file 2 contents",
				"folder1/folder3/file3.txt":         "file 3 contents",
			},
			destDirContents: map[string]string{
				"folder0/file0.txt":                      "file0 contents",
				"dest_folder1/folder0/file0.txt":         "file0 contents",
				"dest_folder1/folder1/folder0/file0.txt": "file0 contents",
				"file1.txt":                              "file1 contents",
				"dest_folder1/file1.txt":                 "file1 contents",
				"dest_folder1/folder1/file1.txt":         "file1 contents",
				"folder1/folder2/file2.txt":              "file2 contents",
				"file2.txt":                              "file2 contents",
			},
			wantScratchContents: map[string]string{
				"folder1/folder0/file0.txt":              "file 0 contents",
				"folder1/folder1/folder0/file0.txt":      "file 0 contents",
				"dest_folder1/folder0/file0.txt":         "file0 contents",
				"dest_folder1/folder1/folder0/file0.txt": "file0 contents",
				"folder1/file1.txt":                      "file 1 contents",
				"folder1/folder1/file1.txt":              "file 1 contents",
				"dest_folder1/file1.txt":                 "file1 contents",
				"dest_folder1/folder1/file1.txt":         "file1 contents",
				"folder1/folder3/file3.txt":              "file 3 contents",
				"file2.txt":                              "file2 contents",
			},
			wantIncludedFromDest: map[string]string{
				"dest_folder1/file1.txt":                 destDirBaseName,
				"dest_folder1/folder0/file0.txt":         destDirBaseName,
				"dest_folder1/folder1/file1.txt":         destDirBaseName,
				"dest_folder1/folder1/folder0/file0.txt": destDirBaseName,
				"file2.txt":                              destDirBaseName,
			},
		},
		{
			name: "skip_paths_with_default_ignore",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("."),
					},
					{
						Paths: mdl.Strings("."),
						From:  mdl.S("destination"),
					},
				},
			},
			templateContents: map[string]string{
				"folder1/file1.txt":      "file 1 contents",
				".bin/file2.txt":         "file 2 contents",
				"folder1/.bin/file3.txt": "file 3 contents",
			},
			destDirContents: map[string]string{
				"file1.txt":              "file1 contents",
				".bin/file2.txt":         "file2 contents",
				"folder1/.bin/file3.txt": "file3 contents",
			},
			wantScratchContents: map[string]string{
				"folder1/file1.txt": "file 1 contents",
				"file1.txt":         "file1 contents",
			},
			wantIncludedFromDest: map[string]string{"file1.txt": destDirBaseName},
		},
		{
			name: "include_from_dest_forgotten_on_reinclude",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: mdl.Strings("."),
						From:  mdl.S("destination"),
					},
					{
						Paths: mdl.Strings("."),
					},
				},
			},
			templateContents: map[string]string{
				"file1.txt": "file 1 template contents",
			},
			destDirContents: map[string]string{
				"file1.txt": "file 1 dest contents",
			},
			wantScratchContents: map[string]string{
				"file1.txt": "file 1 template contents",
			},
			wantIncludedFromDest: map[string]string{},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))

			tempDir := t.TempDir()
			templateDir := filepath.Join(tempDir, tempdir.TemplateDirNamePart)
			scratchDir := filepath.Join(tempDir, tempdir.ScratchDirNamePart)
			destDir := filepath.Join(tempDir, "dest")

			abctestutil.WriteAll(t, templateDir, tc.templateContents)

			// For testing "include from destination"
			abctestutil.WriteAll(t, destDir, tc.destDirContents)

			sp := &stepParams{
				ignorePatterns:   tc.ignorePatterns,
				includedFromDest: make(map[string]string),
				scope:            common.NewScope(tc.inputs, nil),
				scratchDir:       scratchDir,
				templateDir:      templateDir,
				rp: &Params{
					DestDir: destDir,

					FS: &common.ErrorFS{
						FS:      &common.RealFS{},
						StatErr: tc.statErr,
					},
				},
			}

			err := actionInclude(ctx, tc.include, sp)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			gotTemplateContents := abctestutil.LoadDir(t, filepath.Join(tempDir, tempdir.TemplateDirNamePart))
			if diff := cmp.Diff(gotTemplateContents, tc.templateContents); diff != "" {
				t.Errorf("template directory should not have been touched (-got,+want): %s", diff)
			}

			gotScratchContents := abctestutil.LoadDir(t, filepath.Join(tempDir, tempdir.ScratchDirNamePart))
			if diff := cmp.Diff(gotScratchContents, tc.wantScratchContents); diff != "" {
				t.Errorf("scratch directory contents were not as expected (-got,+want): %s", diff)
			}

			opts := []cmp.Option{
				cmpopts.EquateEmpty(),
				cmp.Comparer(func(s1, s2 string) bool {
					// We can't write our test assertions to include the temp
					// directory, so dynamically rewrite the paths in the output
					// files to remove the temp directory name.
					trim := func(s string) string {
						return strings.TrimPrefix(s, tempDir+"/")
					}
					return trim(s1) == trim(s2)
				}),
			}
			if diff := cmp.Diff(sp.includedFromDest, tc.wantIncludedFromDest, opts...); diff != "" {
				t.Errorf("includedFromDest was not as expected (-got,+want): %s", diff)
			}
		})
	}
}
