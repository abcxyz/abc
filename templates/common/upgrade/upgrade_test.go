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

package upgrade

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gopkg.in/yaml.v3"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/render"
	"github.com/abcxyz/abc/templates/common/templatesource"
	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/abc/templates/model/decode"
	"github.com/abcxyz/abc/templates/model/header"
	manifest "github.com/abcxyz/abc/templates/model/manifest/v1alpha1"
	abctestutil "github.com/abcxyz/abc/templates/testutil"
	mdl "github.com/abcxyz/abc/templates/testutil/model"
	"github.com/abcxyz/pkg/testutil"
)

func TestUpgrade(t *testing.T) {
	t.Parallel()

	// We don't use UTC time here because we want to make sure local time
	// gets converted to UTC time before saving.
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("time.LoadLocation(): %v", err)
	}
	beforeUpgradeTime := time.Date(2024, 3, 1, 4, 5, 6, 7, loc)
	afterUpgradeTime := beforeUpgradeTime.Add(time.Hour)

	// This spec file is used for some (but not all) of the tests.
	includeDotSpec := `
api_version: 'cli.abcxyz.dev/v1beta6'
kind: 'Template'

desc: 'my template'

steps:
  - desc: 'include .'
    action: 'include'
    params:
      paths: ['.']
`

	outTxtOnlyManifest := &manifest.Manifest{
		CreationTime:     beforeUpgradeTime.UTC(),
		ModificationTime: beforeUpgradeTime.UTC(),
		TemplateLocation: mdl.S("../template_dir"),
		TemplateVersion:  mdl.S(abctestutil.MinimalGitHeadSHA),
		LocationType:     mdl.S("local_git"),
		Inputs:           []*manifest.Input{},
		OutputFiles: []*manifest.OutputFile{
			{
				File: mdl.S("out.txt"),
			},
		},
	}

	cases := []struct {
		name string

		// We're doing an `abc templates render` followed by an `abc
		// templates upgrade`. Then, the files in
		// templateChangesForUpgrade are added to the template before executing
		// the upgrade operation.

		// origTemplateDirContents is used as the template for the initial
		// render operation.
		origTemplateDirContents map[string]string

		origDestContents map[string]string

		// Only one of templateUnionForUpgrade or templateReplacementForUpgrade
		// may be set.

		// templateUnionForUpgrade is a set of files that will be dropped into
		// the template directory, thus creating the new upgraded template
		// version. Is is convenient to use this instead of
		// templateReplacementForUpgrade in the case where you just want to add
		// files without removing any.
		templateUnionForUpgrade map[string]string
		// templateReplacementForUpgrade will be used as the full template
		// contents when upgrade (unlike templateUnionForUpgrade, which is a
		// delta).
		templateReplacementForUpgrade map[string]string

		wantManifestBeforeUpgrade *manifest.Manifest

		localEdits                   func(tb testing.TB, installedDir string)
		wantDestContentsAfterUpgrade map[string]string // excludes manifest contents
		wantManifestAfterUpgrade     *manifest.Manifest
		want                         *Result
		wantErr                      string
	}{
		// TODO(upgrade): tests to add:
		//  a chain of upgrades
		//  extra inputs needed:
		//    inputs from file
		//    inputs provided as flags
		//    upgraded template removes input(s)

		{
			name: "new_template_has_updated_file_without_local_edits",
			origTemplateDirContents: map[string]string{
				"out.txt":   "hello\n",
				"spec.yaml": includeDotSpec,
			},
			wantManifestBeforeUpgrade: outTxtOnlyManifest,
			templateUnionForUpgrade: map[string]string{
				"spec.yaml": includeDotSpec + `
  - desc: 'append ", world"" to the file'
    action: 'append'
    params:
      paths: ['out.txt']
      with: 'world'`,
			},
			want: &Result{
				NonConflicts: []ActionTaken{{Path: "out.txt", Action: WriteNew}},
			},
			wantDestContentsAfterUpgrade: map[string]string{
				"out.txt": "hello\nworld\n",
			},
			wantManifestAfterUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.ModificationTime = afterUpgradeTime
			}),
		},
		{
			name: "short_circuit_if_already_latest_version",
			want: &Result{AlreadyUpToDate: true},
			origTemplateDirContents: map[string]string{
				"out.txt":   "hello\n",
				"spec.yaml": includeDotSpec,
			},
			templateUnionForUpgrade:   map[string]string{},
			wantManifestBeforeUpgrade: outTxtOnlyManifest,
			wantDestContentsAfterUpgrade: map[string]string{
				"out.txt": "hello\n",
			},
			wantManifestAfterUpgrade: outTxtOnlyManifest,
		},
		{
			name: "new_template_has_file_not_in_old_template",
			origTemplateDirContents: map[string]string{
				"out.txt":   "hello\n",
				"spec.yaml": includeDotSpec,
			},
			wantManifestBeforeUpgrade: outTxtOnlyManifest,
			templateUnionForUpgrade: map[string]string{
				"another_file.txt": "I'm another file\n",
				"spec.yaml":        includeDotSpec,
			},
			want: &Result{
				NonConflicts: []ActionTaken{
					{Action: WriteNew, Path: "another_file.txt"},
					{Action: Noop, Path: "out.txt"},
				},
			},
			wantDestContentsAfterUpgrade: map[string]string{
				"out.txt":          "hello\n",
				"another_file.txt": "I'm another file\n",
			},
			wantManifestAfterUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.ModificationTime = afterUpgradeTime
				m.OutputFiles = []*manifest.OutputFile{
					{
						File: mdl.S("another_file.txt"),
					},
					{
						File: mdl.S("out.txt"),
					},
				}
			}),
		},
		{
			// This test case starts with a template outputting two files, and
			// upgrades to a template that only outputs one file. The other file
			// should be removed from the destination directory.
			name: "old_template_has_file_not_in_new_template_with_no_local_edits",
			origTemplateDirContents: map[string]string{
				"out.txt":          "hello\n",
				"another_file.txt": "I'm another file\n",
				"spec.yaml":        includeDotSpec,
			},
			wantManifestBeforeUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.OutputFiles = []*manifest.OutputFile{
					{
						File: mdl.S("another_file.txt"),
					},
					{
						File: mdl.S("out.txt"),
					},
				}
			}),
			templateReplacementForUpgrade: map[string]string{
				"out.txt":   "hello\n",
				"spec.yaml": includeDotSpec,
			},
			want: &Result{
				NonConflicts: []ActionTaken{
					{Action: DeleteAction, Path: "another_file.txt"},
					{Action: Noop, Path: "out.txt"},
				},
			},
			wantDestContentsAfterUpgrade: map[string]string{
				"out.txt": "hello\n",
			},
			wantManifestAfterUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.ModificationTime = afterUpgradeTime
			}),
		},
		{
			// This test simulates a situation where:
			//  - A template outputs two files
			//  - The user edits one of the files
			//  - We upgrade to a template that no longer outputs the file that was edited
			//  - There should be an edit/delete conflict.
			name: "new_template_removes_file_that_has_user_edits",
			origTemplateDirContents: map[string]string{
				"out.txt":          "hello\n",
				"another_file.txt": "I'm another file\n",
				"spec.yaml":        includeDotSpec,
			},
			wantManifestBeforeUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.OutputFiles = []*manifest.OutputFile{
					{
						File: mdl.S("another_file.txt"),
					},
					{
						File: mdl.S("out.txt"),
					},
				}
			}),
			localEdits: func(tb testing.TB, installedDir string) { //nolint:thelper
				abctestutil.Overwrite(tb, installedDir, "another_file.txt", "my edited contents")
			},
			templateReplacementForUpgrade: map[string]string{
				"out.txt":   "hello\n",
				"spec.yaml": includeDotSpec,
			},
			want: &Result{
				NonConflicts: []ActionTaken{
					{
						Action: Noop,
						Path:   "out.txt",
					},
				},
				Conflicts: []ActionTaken{
					{
						Action:   EditDeleteConflict,
						Path:     "another_file.txt",
						OursPath: "another_file.txt.abcmerge_template_wants_to_delete",
					},
				},
			},
			wantDestContentsAfterUpgrade: map[string]string{
				"another_file.txt.abcmerge_template_wants_to_delete": "my edited contents",
				"out.txt": "hello\n",
			},
			wantManifestAfterUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.ModificationTime = afterUpgradeTime
			}),
		},
		{
			// This test simulates a situation where:
			//  - The template outputs two files
			//  - The user deletes one of them
			//  - We upgrade to a new verson of the template that no longer outputs that file
			//  - This "delete vs delete" should not be a conflict, we just accept the absence of the file.
			name: "upgraded_template_no_longer_outputs_a_file_that_was_locally_deleted",
			origTemplateDirContents: map[string]string{
				"out.txt":          "hello\n",
				"another_file.txt": "I'm another file\n",
				"spec.yaml":        includeDotSpec,
			},
			wantManifestBeforeUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.OutputFiles = []*manifest.OutputFile{
					{
						File: mdl.S("another_file.txt"),
					},
					{
						File: mdl.S("out.txt"),
					},
				}
			}),
			localEdits: func(tb testing.TB, installedDir string) { //nolint:thelper
				filename := filepath.Join(installedDir, "another_file.txt")
				if err := os.Remove(filename); err != nil {
					t.Fatal(err)
				}
			},
			templateReplacementForUpgrade: map[string]string{
				"out.txt":   "hello\n",
				"spec.yaml": includeDotSpec,
			},
			want: &Result{
				NonConflicts: []ActionTaken{
					{Action: Noop, Path: "another_file.txt"},
					{Action: Noop, Path: "out.txt"},
				},
			},
			wantDestContentsAfterUpgrade: map[string]string{
				"out.txt": "hello\n",
			},
			wantManifestAfterUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.ModificationTime = afterUpgradeTime
			}),
		},
		{
			// This test simulates a situation where:
			//  - A template outputs a file
			//  - The user edits that file
			//  - We upgrade to a template that also changes that same file
			//  - There should be an edit/edit conflict.
			name: "upgraded_template_changes_file_that_has_user_edits",
			origTemplateDirContents: map[string]string{
				"out.txt":   "hello",
				"spec.yaml": includeDotSpec,
			},
			wantManifestBeforeUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.OutputFiles = []*manifest.OutputFile{
					{
						File: mdl.S("out.txt"),
					},
				}
			}),
			localEdits: func(tb testing.TB, installedDir string) { //nolint:thelper
				abctestutil.Overwrite(tb, installedDir, "out.txt", "my edited contents")
			},
			templateReplacementForUpgrade: map[string]string{
				"out.txt":   "goodbye",
				"spec.yaml": includeDotSpec,
			},
			want: &Result{
				Conflicts: []ActionTaken{
					{
						Action:               EditEditConflict,
						Path:                 "out.txt",
						OursPath:             "out.txt.abcmerge_locally_edited",
						IncomingTemplatePath: "out.txt.abcmerge_from_new_template",
					},
				},
			},
			wantDestContentsAfterUpgrade: map[string]string{
				"out.txt.abcmerge_locally_edited":    "my edited contents",
				"out.txt.abcmerge_from_new_template": "goodbye",
			},
			wantManifestAfterUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.ModificationTime = afterUpgradeTime
			}),
		},
		{
			// This test simulates a situation where:
			//  - A template outputs a file
			//  - The user deletes the file
			//  - The upgraded template version has new contents for that file
			//  - There should be a delete-vs-edit conflict
			name: "user_deleted_and_template_has_updated_version",
			origTemplateDirContents: map[string]string{
				"out.txt":   "hello",
				"spec.yaml": includeDotSpec,
			},
			wantManifestBeforeUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.OutputFiles = []*manifest.OutputFile{
					{
						File: mdl.S("out.txt"),
					},
				}
			}),
			localEdits: func(tb testing.TB, installedDir string) { //nolint:thelper
				filename := filepath.Join(installedDir, "out.txt")
				if err := os.Remove(filename); err != nil {
					t.Fatal(err)
				}
			},
			templateReplacementForUpgrade: map[string]string{
				"out.txt":   "goodbye",
				"spec.yaml": includeDotSpec,
			},
			want: &Result{
				Conflicts: []ActionTaken{
					{
						Action:               DeleteEditConflict,
						Path:                 "out.txt",
						IncomingTemplatePath: "out.txt.abcmerge_locally_deleted_vs_new_template_version",
					},
				},
			},
			wantDestContentsAfterUpgrade: map[string]string{
				"out.txt.abcmerge_locally_deleted_vs_new_template_version": "goodbye",
			},
			wantManifestAfterUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.ModificationTime = afterUpgradeTime
			}),
		},
		{
			// This test simulates a situation where:
			//  - A template outputs two files
			//  - The user deletes one of the files
			//  - The upgraded template version doesn't change the deleted file,
			//    but does change an unrelated file (so the template dirhash is
			//    different)
			//  - There's no conflict. The user's deletion takes priority.
			name: "user_deleted_and_template_is_unchanged",
			origTemplateDirContents: map[string]string{
				"user_deletes_this_file.txt":     "hello",
				"template_changes_this_file.txt": "initial contents",
				"spec.yaml":                      includeDotSpec,
			},
			wantManifestBeforeUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.OutputFiles = []*manifest.OutputFile{
					{
						File: mdl.S("template_changes_this_file.txt"),
					},
					{
						File: mdl.S("user_deletes_this_file.txt"),
					},
				}
			}),
			localEdits: func(tb testing.TB, installedDir string) { //nolint:thelper
				deleteFile := filepath.Join(installedDir, "user_deletes_this_file.txt")
				if err := os.Remove(deleteFile); err != nil {
					t.Fatal(err)
				}
			},
			templateReplacementForUpgrade: map[string]string{
				"user_deletes_this_file.txt":     "hello",
				"template_changes_this_file.txt": "modified contents",
				"spec.yaml":                      includeDotSpec,
			},
			want: &Result{
				NonConflicts: []ActionTaken{
					{Action: WriteNew, Path: "template_changes_this_file.txt"},
					{Action: Noop, Path: "user_deletes_this_file.txt"},
				},
			},
			wantDestContentsAfterUpgrade: map[string]string{
				"template_changes_this_file.txt": "modified contents",
			},
			wantManifestAfterUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.ModificationTime = afterUpgradeTime.UTC()
				m.OutputFiles = []*manifest.OutputFile{
					{
						File: mdl.S("template_changes_this_file.txt"),
					},
					{
						File: mdl.S("user_deletes_this_file.txt"),
					},
				}
			}),
		},
		{
			name: "user_edited_and_template_unchanged",
			origTemplateDirContents: map[string]string{
				"out.txt":   "initial contents",
				"spec.yaml": includeDotSpec,
				// We need another file in the template that changes on upgrade,
				// otherwise the dirhash will match and the template upgrade
				// will be short-circuited as "no need for upgrade".
				"some_other_file.txt": "foo",
			},
			wantManifestBeforeUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.OutputFiles = []*manifest.OutputFile{
					{
						File: mdl.S("out.txt"),
					},
					{
						File: mdl.S("some_other_file.txt"),
					},
				}
			}),
			localEdits: func(tb testing.TB, installedDir string) { //nolint:thelper
				abctestutil.Overwrite(tb, installedDir, "out.txt", "modified contents")
			},
			templateReplacementForUpgrade: map[string]string{
				"out.txt":             "initial contents",
				"some_other_file.txt": "bar",
				"spec.yaml":           includeDotSpec,
			},
			want: &Result{
				NonConflicts: []ActionTaken{
					{Action: Noop, Path: "out.txt"},
					{Action: WriteNew, Path: "some_other_file.txt"},
				},
			},
			wantDestContentsAfterUpgrade: map[string]string{
				"out.txt":             "modified contents",
				"some_other_file.txt": "bar",
			},
			wantManifestAfterUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.ModificationTime = afterUpgradeTime.UTC()
				m.OutputFiles = []*manifest.OutputFile{
					{
						File: mdl.S("out.txt"),
					},
					{
						File: mdl.S("some_other_file.txt"),
					},
				}
			}),
		},
		{
			// This test simulates a situation where:
			//  - The template is initially rendered doesn't have a file named
			//    out.txt
			//  - The user coincidentally creates a file named out.txt
			//  - The template is upgraded to a new version that *does* output a
			//    file named out.txt, which is different than the user's added
			//    file.
			//  - This should be an add/add conflict
			name: "add_add_conflict",
			origTemplateDirContents: map[string]string{
				"spec.yaml":           includeDotSpec,
				"some_other_file.txt": "some other file contents",
			},
			wantManifestBeforeUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.OutputFiles = []*manifest.OutputFile{
					{
						File: mdl.S("some_other_file.txt"),
					},
				}
			}),
			localEdits: func(tb testing.TB, installedDir string) { //nolint:thelper
				abctestutil.Overwrite(tb, installedDir, "out.txt", "my cool new file")
			},
			templateReplacementForUpgrade: map[string]string{
				"out.txt":             "template now outputs this",
				"some_other_file.txt": "some other file contents",
				"spec.yaml":           includeDotSpec,
			},
			want: &Result{
				NonConflicts: []ActionTaken{
					{
						Action: "noop",
						Path:   "some_other_file.txt",
					},
				},
				Conflicts: []ActionTaken{
					{
						Action:               "addAddConflict",
						Path:                 "out.txt",
						OursPath:             "out.txt.abcmerge_locally_added",
						IncomingTemplatePath: "out.txt.abcmerge_from_new_template",
					},
				},
			},
			wantDestContentsAfterUpgrade: map[string]string{
				"out.txt.abcmerge_locally_added":     "my cool new file",
				"out.txt.abcmerge_from_new_template": "template now outputs this",
				"some_other_file.txt":                "some other file contents",
			},
			wantManifestAfterUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.ModificationTime = afterUpgradeTime.UTC()
				m.OutputFiles = []*manifest.OutputFile{
					{
						File: mdl.S("out.txt"),
					},
					{
						File: mdl.S("some_other_file.txt"),
					},
				}
			}),
		},
		{
			// This test simulates a situation where:
			//  - The template is initially rendered doesn't have a file named
			//    foo
			//  - The user coincidentally creates a file named foo
			//  - The template is upgraded to a new version that *does* output a
			//    file named foo, which is different than the user's added file.
			//  - This should be an add/add conflict
			name: "add_add_no_conflict",
			origTemplateDirContents: map[string]string{
				"spec.yaml":           includeDotSpec,
				"some_other_file.txt": "some other file contents",
			},
			wantManifestBeforeUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.OutputFiles = []*manifest.OutputFile{
					{
						File: mdl.S("some_other_file.txt"),
					},
				}
			}),
			localEdits: func(tb testing.TB, installedDir string) { //nolint:thelper
				abctestutil.Overwrite(tb, installedDir, "out.txt", "identical contents")
			},
			templateReplacementForUpgrade: map[string]string{
				"out.txt":             "identical contents",
				"some_other_file.txt": "some other file contents",
				"spec.yaml":           includeDotSpec,
			},
			want: &Result{
				NonConflicts: []ActionTaken{
					{Action: "noop", Path: "out.txt"},
					{Action: "noop", Path: "some_other_file.txt"},
				},
			},
			wantDestContentsAfterUpgrade: map[string]string{
				"out.txt":             "identical contents",
				"some_other_file.txt": "some other file contents",
			},
			wantManifestAfterUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.ModificationTime = afterUpgradeTime.UTC()
				m.OutputFiles = []*manifest.OutputFile{
					{
						File: mdl.S("out.txt"),
					},
					{
						File: mdl.S("some_other_file.txt"),
					},
				}
			}),
		},
		{
			name: "abort_on_unmerged_conflicts",
			origTemplateDirContents: map[string]string{
				"spec.yaml": includeDotSpec,
				"out.txt":   "foo",
			},
			localEdits: func(tb testing.TB, installedDir string) {
				tb.Helper()
				abctestutil.Overwrite(tb, installedDir, "foo.abcmerge_locally_added", "whatever")
			},
			wantManifestBeforeUpgrade: outTxtOnlyManifest,
			templateUnionForUpgrade: map[string]string{
				// This shouldn't be written to the fs. The upgrade should be
				// aborted before the write.
				"out.txt": "bar",
			},
			wantDestContentsAfterUpgrade: map[string]string{
				"out.txt":                    "foo",
				"foo.abcmerge_locally_added": "whatever",
			},
			wantManifestAfterUpgrade: outTxtOnlyManifest,
			wantErr:                  "already an upgrade in progress",
		},
		{
			name: "include_from_destination",
			origTemplateDirContents: map[string]string{
				"spec.yaml": `
api_version: 'cli.abcxyz.dev/v1beta6'
kind: 'Template'
desc: 'my template'
steps:
  - desc: 'include a file to be modified in place'
    action: 'include'
    params:
      from: 'destination'
      paths: ['file.txt']
  - desc: 'Change favorite color'
    action: 'string_replace'
    params:
      paths: ['file.txt']
      replacements: 
        - to_replace: 'purple'
          with: 'red'  
`,
			},
			origDestContents: map[string]string{
				"file.txt": "purple is my favorite color",
			},
			wantManifestBeforeUpgrade: &manifest.Manifest{
				CreationTime:     beforeUpgradeTime,
				ModificationTime: beforeUpgradeTime,
				TemplateLocation: mdl.S("../template_dir"),
				LocationType:     mdl.S("local_git"),
				TemplateVersion:  mdl.S(abctestutil.MinimalGitHeadSHA),
				Inputs:           []*manifest.Input{},
				OutputFiles: []*manifest.OutputFile{
					{
						File: mdl.S("file.txt"),
						Patch: mdl.SP(`--- a/file.txt
+++ b/file.txt
@@ -1 +1 @@
-red is my favorite color
\ No newline at end of file
+purple is my favorite color
\ No newline at end of file
`),
					},
				},
			},
			templateReplacementForUpgrade: map[string]string{
				"spec.yaml": `
api_version: 'cli.abcxyz.dev/v1beta6'
kind: 'Template'
desc: 'my template'
steps:
  - desc: 'include a file to be modified in place'
    action: 'include'
    params:
      from: 'destination'
      paths: ['file.txt']
  - desc: 'Change favorite color'
    action: 'string_replace'
    params:
      paths: ['file.txt']
      replacements: 
        - to_replace: 'purple'
          with: 'yellow'  
`,
			},
			want: &Result{
				AlreadyUpToDate: false,
				NonConflicts: []ActionTaken{
					{
						Action:      "noop",
						Explanation: "the new template outputs the same contents as the old template, ",
						Path:        "file.txt",
					},
				},
			},
			wantManifestAfterUpgrade: &manifest.Manifest{
				CreationTime:     beforeUpgradeTime,
				ModificationTime: afterUpgradeTime,
				TemplateLocation: mdl.S("../template_dir"),
				LocationType:     mdl.S("local_git"),
				TemplateVersion:  mdl.S(abctestutil.MinimalGitHeadSHA),
				Inputs:           []*manifest.Input{},
				OutputFiles: []*manifest.OutputFile{
					{
						File: mdl.S("file.txt"),
						// TODO(upgrade): once patch-reversing is implemented,
						// then there will be a patch here.
					},
				},
			},
			wantDestContentsAfterUpgrade: map[string]string{
				// TODO(upgrade): once patch-reversing is implemented, this will
				// be "yellow is my favorite color".
				"file.txt": "red is my favorite color",
			},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempBase := t.TempDir()
			destDir := filepath.Join(tempBase, "dest_dir")
			templateDir := filepath.Join(tempBase, "template_dir")

			// Make tempBase into a valid git repo.
			abctestutil.WriteAll(t, tempBase, abctestutil.WithGitRepoAt("", nil))

			abctestutil.WriteAll(t, destDir, tc.origDestContents)

			ctx := context.Background()

			abctestutil.WriteAll(t, templateDir, tc.origTemplateDirContents)
			clk := clock.NewMock()
			clk.Set(beforeUpgradeTime)
			renderResult := mustRender(t, ctx, clk, tempBase, templateDir, destDir)

			manifestFullPath := filepath.Join(destDir, renderResult.ManifestPath)

			assertManifest(ctx, t, "before upgrade", tc.wantManifestBeforeUpgrade, manifestFullPath)

			clk.Set(afterUpgradeTime) // simulate time passing between initial installation and upgrade

			params := &Params{
				Clock:        clk,
				CWD:          destDir,
				FS:           &common.RealFS{},
				ManifestPath: manifestFullPath,
				Stdout:       os.Stdout,
			}

			if tc.localEdits != nil {
				tc.localEdits(t, destDir)
			}

			// Create the new template version that we'll upgrade to, in
			// templateDir.
			if len(tc.templateUnionForUpgrade) > 0 && len(tc.templateReplacementForUpgrade) > 0 {
				t.Fatal("test config bug: only one of templateUnionForUpgrade or templateReplacementForUpgrade should be set")
			}
			if len(tc.templateUnionForUpgrade) > 0 {
				abctestutil.WriteAll(t, templateDir, tc.templateUnionForUpgrade)
			}
			if len(tc.templateReplacementForUpgrade) > 0 {
				if err := os.RemoveAll(templateDir); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(templateDir, common.OwnerRWXPerms); err != nil {
					t.Fatal(err)
				}
				abctestutil.WriteAll(t, templateDir, tc.templateReplacementForUpgrade)
			}

			result, err := Upgrade(ctx, params)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}

			opts := []cmp.Option{
				cmpopts.EquateEmpty(),
				cmpopts.IgnoreFields(ActionTaken{}, "Explanation"), // don't assert on debugging messages. That would make test cases overly verbose.
			}
			if diff := cmp.Diff(result, tc.want, opts...); diff != "" {
				t.Errorf("result was not as expected, diff is (-got, +want): %v", diff)
			}

			assertManifest(ctx, t, "after upgrade", tc.wantManifestAfterUpgrade, manifestFullPath)

			gotDestContentsAfter := abctestutil.LoadDir(t, destDir, abctestutil.SkipGlob(".abc/manifest*"))
			if diff := cmp.Diff(gotDestContentsAfter, tc.wantDestContentsAfterUpgrade); diff != "" {
				t.Errorf("installed directory contents after upgrading were not as expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestUpgrade_NonCanonical(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tempBase := t.TempDir()
	templateDir := filepath.Join(tempBase, "template_dir")
	if err := os.MkdirAll(templateDir, common.OwnerRWXPerms); err != nil {
		t.Fatal(err)
	}

	m := &manifest.WithHeader{
		Header: &header.Fields{
			NewStyleAPIVersion: model.String{Val: "cli.abcxyz.dev/v1beta6"},
			Kind:               model.String{Val: decode.KindManifest},
		},
		Wrapped: &manifest.ForMarshaling{
			TemplateLocation: model.String{Val: ""},
			LocationType:     model.String{Val: ""},
			TemplateDirhash:  model.String{Val: "asdfasdfasdfasdfasdf"},
		},
	}

	buf, err := yaml.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}

	manifestFullPath := filepath.Join(templateDir, "manifest.yaml")
	if err := os.WriteFile(manifestFullPath, buf, common.OwnerRWPerms); err != nil {
		t.Fatal(err)
	}

	params := &Params{
		CWD:          tempBase,
		FS:           &common.RealFS{},
		ManifestPath: manifestFullPath,
	}

	_, err = Upgrade(ctx, params)
	wantErr := "this template can't be upgraded because its manifest doesn't contain a template_location"
	if diff := testutil.DiffErrString(err, wantErr); diff != "" {
		t.Fatal(diff)
	}
}

func assertManifest(ctx context.Context, tb testing.TB, whereAreWe string, want *manifest.Manifest, path string) {
	tb.Helper()

	got, err := loadManifest(ctx, &common.RealFS{}, path)
	if err != nil {
		tb.Fatal(err)
	}

	opts := []cmp.Option{
		// Don't force test authors to assert the line and column numbers
		cmpopts.IgnoreTypes(&model.ConfigPos{}, model.ConfigPos{}),

		// Don't force test author to compute hashes when writing test/updating test cases.
		cmpopts.IgnoreFields(manifest.Manifest{}, "TemplateDirhash"),
		cmpopts.IgnoreFields(manifest.OutputFile{}, "Hash"),
	}
	if diff := cmp.Diff(got, want, opts...); diff != "" {
		tb.Errorf("at %q, manifest was not as expected (-got,+want): %s", whereAreWe, diff)
	}

	// We omitted these fields from the Diff(), but make sure they look sane.
	const minHashLen = 10 // arbitrarily picked, anything shorter isn't a sane hash
	if len(got.TemplateDirhash.Val) < minHashLen {
		tb.Errorf("dirhash %q is too short", got.TemplateDirhash.Val)
	}
	for _, oh := range got.OutputFiles {
		if len(oh.Hash.Val) < minHashLen {
			tb.Errorf("output hash %q for file %q is too short", oh.Hash.Val, oh.File.Val)
		}
	}
}

func mustRender(tb testing.TB, ctx context.Context, clk clock.Clock, tempBase, templateDir, destDir string) *render.Result {
	tb.Helper()

	downloader, err := templatesource.ParseSource(ctx, &templatesource.ParseSourceParams{
		CWD:    tempBase,
		Source: templateDir,
	})
	if err != nil {
		tb.Fatal(err)
	}

	result, err := render.Render(ctx, &render.Params{
		Clock:       clk,
		Cwd:         tempBase,
		DestDir:     destDir,
		Downloader:  downloader,
		FS:          &common.RealFS{},
		Manifest:    true,
		OutDir:      destDir,
		TempDirBase: tempBase,
	})
	if err != nil {
		tb.Fatal(err)
	}
	return result
}

// A convenience function for "I want a copy of this manifest but with one small
// change". This isn't a deep copy, so callers should only modify the top-level
// fields of the given manifest if the input manifest m is shared across test
// cases.
func manifestWith(m *manifest.Manifest, change func(*manifest.Manifest)) *manifest.Manifest {
	out := *m
	change(&out)
	return &out
}
