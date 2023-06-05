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
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
)

func TestActionInclude(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                string
		paths               []string
		templateContents    map[string]modeAndContents
		inputs              map[string]string
		wantScratchContents map[string]modeAndContents
		statErr             error
		wantErr             string
	}{
		{
			name:  "simple_success",
			paths: []string{"myfile.txt"},
			templateContents: map[string]modeAndContents{
				"myfile.txt": {0o600, "my file contents"},
			},
			wantScratchContents: map[string]modeAndContents{
				"myfile.txt": {0o600, "my file contents"},
			},
		},
		{
			name:    "reject_dot_dot",
			paths:   []string{"../file.txt"},
			wantErr: `path must not contain ".."`,
		},
		{
			name:  "templated_filename_success",
			paths: []string{"{{.my_dir}}/{{.my_file}}"},
			templateContents: map[string]modeAndContents{
				"foo/bar.txt": {0o600, "file contents"},
			},
			inputs: map[string]string{
				"my_dir":  "foo",
				"my_file": "bar.txt",
			},
			wantScratchContents: map[string]modeAndContents{
				"foo/bar.txt": {0o600, "file contents"},
			},
		},
		{
			name:  "templated_filename_nonexistent_input_var_should_fail",
			paths: []string{"{{.filename}}"},
			templateContents: map[string]modeAndContents{
				"myfile.txt": {0o600, "file contents"},
			},
			inputs:  map[string]string{},
			wantErr: `no entry for key "filename"`,
		},
		{
			name:  "nonexistent_source_should_fail",
			paths: []string{"nonexistent"},
			templateContents: map[string]modeAndContents{
				"myfile.txt": {0o600, "file contents"},
			},
			wantErr: `include path doesn't exist: "nonexistent"`,
		},
		{
			// Note: we don't exhaustively test every possible FS error here. That's
			// already done in the tests for the underlying copyRecursive function.
			name:  "filesystem_error_should_be_returned",
			paths: []string{"myfile.txt"},
			templateContents: map[string]modeAndContents{
				"myfile.txt": {0o600, "my file contents"},
			},
			statErr: fmt.Errorf("fake error"),
			wantErr: "fake error",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))

			tempDir := t.TempDir()
			templateDir := filepath.Join(tempDir, templateDirNamePart)
			scratchDir := filepath.Join(tempDir, scratchDirNamePart)

			if err := writeAll(templateDir, tc.templateContents); err != nil {
				t.Fatal(err)
			}

			include := &model.Include{
				Pos:   &model.ConfigPos{},
				Paths: modelStrings(tc.paths),
			}
			sp := &stepParams{
				fs: &errorFS{
					renderFS: &realFS{},
					statErr:  tc.statErr,
				},
				scratchDir:  scratchDir,
				templateDir: templateDir,
				inputs:      tc.inputs,
			}
			err := actionInclude(ctx, include, sp)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			gotTemplateContents := loadDirContents(t, filepath.Join(tempDir, templateDirNamePart))
			if diff := cmp.Diff(gotTemplateContents, tc.templateContents); diff != "" {
				t.Errorf("template directory should not have been touched (-got,+want): %s", diff)
			}

			gotScratchContents := loadDirContents(t, filepath.Join(tempDir, scratchDirNamePart))
			if diff := cmp.Diff(gotScratchContents, tc.wantScratchContents); diff != "" {
				t.Errorf("scratch directory contents were not as expected (-got,+want): %s", diff)
			}
		})
	}
}
