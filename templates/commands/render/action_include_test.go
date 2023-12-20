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
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/model"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta2"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
)

func TestActionInclude(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                 string
		include              *spec.Include
		templateContents     map[string]common.ModeAndContents
		destDirContents      map[string]common.ModeAndContents
		inputs               map[string]string
		wantScratchContents  map[string]common.ModeAndContents
		wantIncludedFromDest []string
		statErr              error
		wantErr              string
	}{
		{
			name: "simple_success",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"myfile.txt"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"myfile.txt": {Mode: 0o600, Contents: "my file contents"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"myfile.txt": {Mode: 0o600, Contents: "my file contents"},
			},
		},
		{
			name: "absolute_path_treated_as_relative",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"/myfile.txt"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"myfile.txt": {Mode: 0o600, Contents: "my file contents"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"myfile.txt": {Mode: 0o600, Contents: "my file contents"},
			},
		},
		{
			name: "reject_dot_dot",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"../file.txt"}),
					},
				},
			},
			wantErr: fmt.Sprintf(`path %q must not contain ".."`, filepath.FromSlash("../file.txt")),
		},
		{
			name: "reject_dot_dot_glob",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"../*.txt"}),
					},
				},
			},
			wantErr: fmt.Sprintf(`path %q must not contain ".."`, filepath.FromSlash("../*.txt")),
		},
		{
			name: "templated_filename_success",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"{{.my_dir}}/{{.my_file}}"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"foo/bar.txt": {Mode: 0o600, Contents: "file contents"},
			},
			inputs: map[string]string{
				"my_dir":  "foo",
				"my_file": "bar.txt",
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"foo/bar.txt": {Mode: 0o600, Contents: "file contents"},
			},
		},
		{
			name: "including_multiple_times_should_succeed",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"myfile.txt", "myfile.txt"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"myfile.txt": {Mode: 0o600, Contents: "my file contents"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"myfile.txt": {Mode: 0o600, Contents: "my file contents"},
			},
		},
		{
			name: "including_multiple_times_should_succeed",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"foo/myfile.txt", "foo/", "foo/myfile.txt"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"foo/myfile.txt": {Mode: 0o600, Contents: "my file contents"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"foo/myfile.txt": {Mode: 0o600, Contents: "my file contents"},
			},
		},
		{
			name: "templated_filename_nonexistent_input_var_should_fail",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"{{.filename}}"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"myfile.txt": {Mode: 0o600, Contents: "file contents"},
			},
			inputs:  map[string]string{},
			wantErr: `nonexistent input variable name "filename"`,
		},
		{
			name: "nonexistent_source_should_fail",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"nonexistent"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"myfile.txt": {Mode: 0o600, Contents: "my file contents"},
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
						Paths: modelStrings([]string{"myfile.txt"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"myfile.txt": {Mode: 0o600, Contents: "my file contents"},
			},
			statErr: fmt.Errorf("fake error"),
			wantErr: "fake error",
		},
		{
			name: "simple_glob_path",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"*.txt"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "file1 contents"},
				"file2.txt": {Mode: 0o600, Contents: "file2 contents"},
				"file3.txt": {Mode: 0o600, Contents: "file3 contents"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "file1 contents"},
				"file2.txt": {Mode: 0o600, Contents: "file2 contents"},
				"file3.txt": {Mode: 0o600, Contents: "file3 contents"},
			},
		},
		{
			name: "glob_dir",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"dir*"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"dir1/file1.txt": {Mode: 0o600, Contents: "file1 contents"},
				"dir2/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"dir1/file1.txt": {Mode: 0o600, Contents: "file1 contents"},
				"dir2/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
		},
		{
			name: "glob_dir_and_files",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"dir*"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"directive.txt":  {Mode: 0o600, Contents: "directive file contents"},
				"director.txt":   {Mode: 0o600, Contents: "director file contents"},
				"dir1/file1.txt": {Mode: 0o600, Contents: "file1 contents"},
				"dir2/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"directive.txt":  {Mode: 0o600, Contents: "directive file contents"},
				"director.txt":   {Mode: 0o600, Contents: "director file contents"},
				"dir1/file1.txt": {Mode: 0o600, Contents: "file1 contents"},
				"dir2/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
		},
		{
			name: "glob_in_subdir",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"dir/*.txt"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"dont_include.txt":  {Mode: 0o600, Contents: "dont_include contents"},
				"dont/include2.txt": {Mode: 0o600, Contents: "dont_include2 contents"},
				"dir/file1.txt":     {Mode: 0o600, Contents: "file1 contents"},
				"dir/file2.txt":     {Mode: 0o600, Contents: "file2 contents"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"dir/file1.txt": {Mode: 0o600, Contents: "file1 contents"},
				"dir/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
		},
		{
			name: "go_template_to_glob",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"{{.filename}}.*"}),
					},
				},
			},
			inputs: map[string]string{
				"filename": "file",
			},
			templateContents: map[string]common.ModeAndContents{
				"file.txt":  {Mode: 0o600, Contents: "txt file contents"},
				"file.md":   {Mode: 0o600, Contents: "md file contents"},
				"file.json": {Mode: 0o600, Contents: "json file contents"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"file.txt":  {Mode: 0o600, Contents: "txt file contents"},
				"file.md":   {Mode: 0o600, Contents: "md file contents"},
				"file.json": {Mode: 0o600, Contents: "json file contents"},
			},
		},
		{
			name: "as_with_single_path",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"dir1/file1.txt"}),
						As:    modelStrings([]string{"dir2/file2.txt"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"dir1/file1.txt": {Mode: 0o600, Contents: "my file contents"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"dir2/file2.txt": {Mode: 0o600, Contents: "my file contents"},
			},
		},
		{
			name: "as_with_single_glob_path",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"*.txt"}),
						As:    modelStrings([]string{"dir"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "file1 contents"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"dir/file1.txt": {Mode: 0o600, Contents: "file1 contents"},
			},
		},
		{
			name: "as_with_multiple_glob_paths",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"*.txt"}),
						As:    modelStrings([]string{"dir"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "file1 contents"},
				"file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"dir/file1.txt": {Mode: 0o600, Contents: "file1 contents"},
				"dir/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
		},
		{
			name: "multiple_as_with_glob_paths",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"*.txt", "*.md"}),
						As:    modelStrings([]string{"txtdir", "mddir"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "file1 contents"},
				"file2.txt": {Mode: 0o600, Contents: "file2 contents"},
				"file3.md":  {Mode: 0o600, Contents: "file3 contents"},
				"file4.md":  {Mode: 0o600, Contents: "file4 contents"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"txtdir/file1.txt": {Mode: 0o600, Contents: "file1 contents"},
				"txtdir/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
				"mddir/file3.md":   {Mode: 0o600, Contents: "file3 contents"},
				"mddir/file4.md":   {Mode: 0o600, Contents: "file4 contents"},
			},
		},
		{
			name: "as_with_glob_dir",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"dir*"}),
						As:    modelStrings([]string{"topdir"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"dir1/file1.txt": {Mode: 0o600, Contents: "file1 contents"},
				"dir2/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"topdir/dir1/file1.txt": {Mode: 0o600, Contents: "file1 contents"},
				"topdir/dir2/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
		},
		{
			name: "as_with_multiple_paths_and_templates",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"file{{.one}}.txt", "file{{.two}}.txt"}),
						As:    modelStrings([]string{"file{{.three}}.txt", "file{{.four}}.txt"}),
					},
				},
			},
			inputs: map[string]string{
				"one":   "1",
				"two":   "2",
				"three": "3",
				"four":  "4",
			},
			templateContents: map[string]common.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "my file contents"},
				"file2.txt": {Mode: 0o600, Contents: "my file contents"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"file3.txt": {Mode: 0o600, Contents: "my file contents"},
				"file4.txt": {Mode: 0o600, Contents: "my file contents"},
			},
		},
		{
			name: "spec_yaml_should_be_skipped",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"."}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"file1.txt":                 {Mode: 0o600, Contents: "my file contents"},
				"spec.yaml":                 {Mode: 0o600, Contents: "spec contents"},
				"testdata/golden/test.yaml": {Mode: 0o600, Contents: "some yaml"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "my file contents"},
			},
		},
		{
			name: "spec_yaml_in_subdir_should_not_be_skipped",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"."}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"file1.txt":        {Mode: 0o600, Contents: "my file contents"},
				"subdir/spec.yaml": {Mode: 0o600, Contents: "spec contents"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"file1.txt":        {Mode: 0o600, Contents: "my file contents"},
				"subdir/spec.yaml": {Mode: 0o600, Contents: "spec contents"},
			},
		},
		{
			name: "golden_test_in_subdir_should_not_be_skipped",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"."}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"file1.txt":                        {Mode: 0o600, Contents: "my file contents"},
				"spec.yaml":                        {Mode: 0o600, Contents: "spec contents"},
				"subdir/testdata/golden/test.yaml": {Mode: 0o600, Contents: "some yaml"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"file1.txt":                        {Mode: 0o600, Contents: "my file contents"},
				"subdir/testdata/golden/test.yaml": {Mode: 0o600, Contents: "some yaml"},
			},
		},
		{
			name: "skip_file",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"."}),
						Skip:  modelStrings([]string{"file2.txt"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"file1.txt":                 {Mode: 0o600, Contents: "file 1 contents"},
				"file2.txt":                 {Mode: 0o600, Contents: "file 2 contents"},
				"subfolder/file3.txt":       {Mode: 0o600, Contents: "file 3 contents"},
				"spec.yaml":                 {Mode: 0o600, Contents: "spec contents"},
				"testdata/golden/test.yaml": {Mode: 0o600, Contents: "some yaml"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"file1.txt":           {Mode: 0o600, Contents: "file 1 contents"},
				"subfolder/file3.txt": {Mode: 0o600, Contents: "file 3 contents"},
			},
		},
		{
			name: "skip_multiple_files",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"."}),
						Skip:  modelStrings([]string{"file1.txt", "file2.txt", "file4.txt"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"file1.txt":                 {Mode: 0o600, Contents: "file 1 contents"},
				"file2.txt":                 {Mode: 0o600, Contents: "file 2 contents"},
				"file3.txt":                 {Mode: 0o600, Contents: "file 3 contents"},
				"file4.txt":                 {Mode: 0o600, Contents: "file 4 contents"},
				"spec.yaml":                 {Mode: 0o600, Contents: "spec contents"},
				"testdata/golden/test.yaml": {Mode: 0o600, Contents: "some yaml"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"file3.txt": {Mode: 0o600, Contents: "file 3 contents"},
			},
		},
		{
			name: "skip_multiple_files_globbing",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"*.txt"}),
						Skip:  modelStrings([]string{"file1.txt", "file2.txt", "file4.txt"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"file1.txt":                 {Mode: 0o600, Contents: "file 1 contents"},
				"file2.txt":                 {Mode: 0o600, Contents: "file 2 contents"},
				"file3.txt":                 {Mode: 0o600, Contents: "file 3 contents"},
				"file4.txt":                 {Mode: 0o600, Contents: "file 4 contents"},
				"spec.yaml":                 {Mode: 0o600, Contents: "spec contents"},
				"testdata/golden/test.yaml": {Mode: 0o600, Contents: "some yaml"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"file3.txt": {Mode: 0o600, Contents: "file 3 contents"},
			},
		},
		{
			name: "skip_file_in_subfolder",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"subfolder"}),
						Skip:  modelStrings([]string{"subfolder/file2.txt"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"file1.txt":                 {Mode: 0o600, Contents: "file 1 contents"},
				"subfolder/file2.txt":       {Mode: 0o600, Contents: "file 2 contents"},
				"subfolder/file3.txt":       {Mode: 0o600, Contents: "file 3 contents"},
				"spec.yaml":                 {Mode: 0o600, Contents: "spec contents"},
				"testdata/golden/test.yaml": {Mode: 0o600, Contents: "some yaml"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"subfolder/file3.txt": {Mode: 0o600, Contents: "file 3 contents"},
			},
		},
		{
			name: "skip_glob_in_directory",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"subfolder"}),
						Skip:  modelStrings([]string{"subfolder/*.txt"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"subfolder/skip1.txt":       {Mode: 0o600, Contents: "skip 1 contents"},
				"subfolder/skip2.txt":       {Mode: 0o600, Contents: "skip 2 contents"},
				"subfolder/include.md":      {Mode: 0o600, Contents: "include contents"},
				"spec.yaml":                 {Mode: 0o600, Contents: "spec contents"},
				"testdata/golden/test.yaml": {Mode: 0o600, Contents: "some yaml"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"subfolder/include.md": {Mode: 0o600, Contents: "include contents"},
			},
		},
		{
			name: "skip_directory",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"folder1"}),
						Skip:  modelStrings([]string{"folder1/folder2"}),
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"folder1/file1.txt":         {Mode: 0o600, Contents: "file 1 contents"},
				"folder1/folder2/file2.txt": {Mode: 0o600, Contents: "file 2 contents"},
				"folder1/folder3/file3.txt": {Mode: 0o600, Contents: "file 2 contents"},
				"spec.yaml":                 {Mode: 0o600, Contents: "spec contents"},
				"testdata/golden/test.yaml": {Mode: 0o600, Contents: "some yaml"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"folder1/file1.txt":         {Mode: 0o600, Contents: "file 1 contents"},
				"folder1/folder3/file3.txt": {Mode: 0o600, Contents: "file 2 contents"},
			},
		},
		{
			name: "include_dot_from_destination",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"."}),
						From:  model.String{Val: "destination"},
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"spec.yaml":                 {Mode: 0o600, Contents: "spec contents"},
				"testdata/golden/test.yaml": {Mode: 0o600, Contents: "some yaml"},
			},
			destDirContents: map[string]common.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "file1 contents"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "file1 contents"},
			},
			wantIncludedFromDest: []string{"file1.txt"},
		},
		{
			name: "include_subdir_from_destination",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"subdir"}),
						From:  model.String{Val: "destination"},
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"spec.yaml":                 {Mode: 0o600, Contents: "spec contents"},
				"testdata/golden/test.yaml": {Mode: 0o600, Contents: "some yaml"},
			},
			destDirContents: map[string]common.ModeAndContents{
				"file1.txt":        {Mode: 0o600, Contents: "file1 contents"},
				"subdir/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"subdir/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
			wantIncludedFromDest: []string{"subdir/file2.txt"},
		},
		{
			name: "include_glob_from_destination",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"*.txt"}),
						From:  model.String{Val: "destination"},
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"spec.yaml":                 {Mode: 0o600, Contents: "spec contents"},
				"testdata/golden/test.yaml": {Mode: 0o600, Contents: "some yaml"},
			},
			destDirContents: map[string]common.ModeAndContents{
				"file1.txt":        {Mode: 0o600, Contents: "file1 contents"},
				"file2.txt":        {Mode: 0o600, Contents: "file1 contents"},
				"subdir/file3.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
				"file1.txt": {Mode: 0o600, Contents: "file1 contents"},
				"file2.txt": {Mode: 0o600, Contents: "file1 contents"},
			},
			wantIncludedFromDest: []string{
				"file1.txt",
				"file2.txt",
			},
		},
		{
			name: "include_individual_files_from_destination",
			include: &spec.Include{
				Paths: []*spec.IncludePath{
					{
						Paths: modelStrings([]string{"file1.txt", "subdir/file2.txt"}),
						From:  model.String{Val: "destination"},
					},
				},
			},
			templateContents: map[string]common.ModeAndContents{
				"spec.yaml":                 {Mode: 0o600, Contents: "spec contents"},
				"testdata/golden/test.yaml": {Mode: 0o600, Contents: "some yaml"},
			},
			destDirContents: map[string]common.ModeAndContents{
				"file1.txt":        {Mode: 0o600, Contents: "file1 contents"},
				"subdir/file2.txt": {Mode: 0o600, Contents: "file2 contents"},
			},
			wantScratchContents: map[string]common.ModeAndContents{
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
			toPlatformPaths(tc.wantIncludedFromDest)

			tempDir := t.TempDir()
			templateDir := filepath.Join(tempDir, templateDirNamePart)
			scratchDir := filepath.Join(tempDir, scratchDirNamePart)
			destDir := filepath.Join(tempDir, "dest")

			common.WriteAll(t, templateDir, tc.templateContents)

			// For testing "include from destination"
			common.WriteAll(t, destDir, tc.destDirContents)

			sp := &stepParams{
				flags: &RenderFlags{
					Dest: destDir,
				},
				fs: &common.ErrorFS{
					FS:      &common.RealFS{},
					StatErr: tc.statErr,
				},
				scratchDir:  scratchDir,
				templateDir: templateDir,
				scope:       common.NewScope(tc.inputs),
			}

			err := actionInclude(ctx, tc.include, sp)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			gotTemplateContents := common.LoadDirContents(t, filepath.Join(tempDir, templateDirNamePart))
			if diff := cmp.Diff(gotTemplateContents, tc.templateContents, common.CmpFileMode); diff != "" {
				t.Errorf("template directory should not have been touched (-got,+want): %s", diff)
			}

			gotScratchContents := common.LoadDirContents(t, filepath.Join(tempDir, scratchDirNamePart))
			if diff := cmp.Diff(gotScratchContents, tc.wantScratchContents, common.CmpFileMode); diff != "" {
				t.Errorf("scratch directory contents were not as expected (-got,+want): %s", diff)
			}

			if diff := cmp.Diff(sp.includedFromDest, tc.wantIncludedFromDest); diff != "" {
				t.Errorf("includedFromDest was not as expected (-got,+want): %s", diff)
			}
		})
	}
}
