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
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/render"
	"github.com/abcxyz/abc/templates/common/templatesource"
	"github.com/abcxyz/abc/templates/model"
	manifest "github.com/abcxyz/abc/templates/model/manifest/v1alpha1"
	abctestutil "github.com/abcxyz/abc/templates/testutil"
	mdl "github.com/abcxyz/abc/templates/testutil/model"
	"github.com/abcxyz/pkg/testutil"
)

const includeDotSpec = `api_version: 'cli.abcxyz.dev/v1beta6'
kind: 'Template'
desc: 'my template'
steps:
  - desc: 'include .'
    action: 'include'
    params:
      paths: ['.']
`

func TestUpgradeAll(t *testing.T) {
	t.Parallel()

	// We don't use UTC time here because we want to make sure local time
	// gets converted to UTC time before saving.
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("time.LoadLocation(): %v", err)
	}
	beforeUpgradeTime := time.Date(2024, 3, 1, 4, 5, 6, 7, loc)
	afterUpgradeTime := beforeUpgradeTime.Add(time.Hour)

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

	wantDLMeta := &templatesource.DownloadMetadata{
		IsCanonical:     true,
		CanonicalSource: "../template_dir",
		LocationType:    templatesource.LocalGit,
		Version:         abctestutil.MinimalGitHeadSHA,
		Vars: templatesource.DownloaderVars{
			GitSHA:      abctestutil.MinimalGitHeadSHA,
			GitShortSHA: abctestutil.MinimalGitHeadShortSHA,
		},
	}

	cases := []struct {
		name string

		// We're doing an `abc render` followed by an `abc
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

		// If these fields are set, then we'll fake a remote download using
		// fakeXXXDownloaderFactory. This allows testing of the "upgrade_channel"
		// logic.
		fakeDownloader               *fakeDownloader
		fakeUpgradeDownloaderFactory *fakeUpgradeDownloaderFactory

		// wantRejectFile, if set, is a path to a file that should contain the
		// rejected hunks from the patch command. This is a hack since Mac and
		// Linux `patch` commands generate different formats for their reject
		// hunk files. We therefore just test for the presence of the file, not
		// the contents.
		wantRejectFile string
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
				Overall: Success,
				Results: []*ManifestResult{
					{
						Type:         Success,
						NonConflicts: []ActionTaken{{Path: "out.txt", Action: WriteNew}},
						DLMeta:       wantDLMeta,
						ManifestPath: ".",
					},
				},
			},
			wantDestContentsAfterUpgrade: map[string]string{
				"out.txt": "hello\nworld\n",
			},
			wantManifestAfterUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.ModificationTime = afterUpgradeTime.UTC()
			}),
		},
		{
			name: "short_circuit_if_already_latest_version",
			want: &Result{
				Overall: AlreadyUpToDate,
				Results: []*ManifestResult{
					{
						ManifestPath: ".",
						Type:         AlreadyUpToDate,
						DLMeta:       wantDLMeta,
					},
				},
			},
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
				Overall: Success,
				Results: []*ManifestResult{
					{
						ManifestPath: ".",
						Type:         Success,
						NonConflicts: []ActionTaken{
							{Action: WriteNew, Path: "another_file.txt"},
							{Action: Noop, Path: "out.txt"},
						},
						DLMeta: wantDLMeta,
					},
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
				Overall: Success,
				Results: []*ManifestResult{
					{
						ManifestPath: ".",
						Type:         Success,
						NonConflicts: []ActionTaken{
							{Action: DeleteAction, Path: "another_file.txt"},
							{Action: Noop, Path: "out.txt"},
						},
						DLMeta: wantDLMeta,
					},
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
				Overall: MergeConflict,
				Results: []*ManifestResult{
					{
						ManifestPath: ".",
						Type:         MergeConflict,
						NonConflicts: []ActionTaken{
							{
								Action: Noop,
								Path:   "out.txt",
							},
						},
						MergeConflicts: []ActionTaken{
							{
								Action:   EditDeleteConflict,
								Path:     "another_file.txt",
								OursPath: "another_file.txt.abcmerge_template_wants_to_delete",
							},
						},
						DLMeta: wantDLMeta,
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
				Overall: Success,
				Results: []*ManifestResult{
					{
						ManifestPath: ".",
						Type:         Success,
						NonConflicts: []ActionTaken{
							{Action: Noop, Path: "another_file.txt"},
							{Action: Noop, Path: "out.txt"},
						},
						DLMeta: wantDLMeta,
					},
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
				Overall: MergeConflict,
				Results: []*ManifestResult{
					{
						ManifestPath: ".",
						Type:         MergeConflict,
						MergeConflicts: []ActionTaken{
							{
								Action:               EditEditConflict,
								Path:                 "out.txt",
								IncomingTemplatePath: "out.txt.abcmerge_from_new_template",
							},
						},
						DLMeta: wantDLMeta,
					},
				},
			},
			wantDestContentsAfterUpgrade: map[string]string{
				"out.txt":                            "my edited contents",
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
				Overall: MergeConflict,
				Results: []*ManifestResult{
					{
						ManifestPath: ".",
						Type:         MergeConflict,
						MergeConflicts: []ActionTaken{
							{
								Action:               DeleteEditConflict,
								Path:                 "out.txt",
								IncomingTemplatePath: "out.txt.abcmerge_locally_deleted_vs_new_template_version",
							},
						},
						DLMeta: wantDLMeta,
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
				Overall: Success,
				Results: []*ManifestResult{
					{
						ManifestPath: ".",
						Type:         Success,
						NonConflicts: []ActionTaken{
							{Action: WriteNew, Path: "template_changes_this_file.txt"},
							{Action: Noop, Path: "user_deletes_this_file.txt"},
						},
						DLMeta: wantDLMeta,
					},
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
				Overall: Success,
				Results: []*ManifestResult{
					{
						ManifestPath: ".",
						Type:         Success,
						NonConflicts: []ActionTaken{
							{Action: Noop, Path: "out.txt"},
							{Action: WriteNew, Path: "some_other_file.txt"},
						},
						DLMeta: wantDLMeta,
					},
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
				Overall: MergeConflict,
				Results: []*ManifestResult{
					{
						ManifestPath: ".",
						Type:         MergeConflict,
						NonConflicts: []ActionTaken{
							{
								Action: "noop",
								Path:   "some_other_file.txt",
							},
						},
						MergeConflicts: []ActionTaken{
							{
								Action:               "addAddConflict",
								Path:                 "out.txt",
								IncomingTemplatePath: "out.txt.abcmerge_from_new_template",
							},
						},
						DLMeta: wantDLMeta,
					},
				},
			},
			wantDestContentsAfterUpgrade: map[string]string{
				"out.txt":                            "my cool new file",
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
				Overall: Success,
				Results: []*ManifestResult{
					{
						ManifestPath: ".",
						Type:         Success,
						NonConflicts: []ActionTaken{
							{Action: "noop", Path: "out.txt"},
							{Action: "noop", Path: "some_other_file.txt"},
						},
						DLMeta: wantDLMeta,
					},
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
			// This test simulates a template being downloaded from a remote
			// source using a fake downloader.
			name: "add_file_with_remote_template",
			origTemplateDirContents: map[string]string{
				"spec.yaml": includeDotSpec,
				"out.txt":   "out.txt contents",
			},
			wantManifestBeforeUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.TemplateLocation.Val = "fake_canonical_source"
				m.LocationType.Val = "fake_location_type"
				m.TemplateVersion.Val = "fake_version"
				m.UpgradeChannel.Val = "fake_upgrade_channel"
				m.OutputFiles = []*manifest.OutputFile{
					{
						File: mdl.S("out.txt"),
					},
				}
			}),
			localEdits: func(tb testing.TB, installedDir string) { //nolint:thelper
				abctestutil.Overwrite(tb, installedDir, "out.txt", "identical contents")
			},
			templateUnionForUpgrade: map[string]string{
				"some_other_file.txt": "some other file contents",
			},
			fakeDownloader: &fakeDownloader{
				outDLMeta: &templatesource.DownloadMetadata{
					IsCanonical:     true,
					CanonicalSource: "fake_canonical_source",
					LocationType:    "fake_location_type",
					Version:         "fake_version",
					UpgradeChannel:  "fake_upgrade_channel",
				},
			},
			fakeUpgradeDownloaderFactory: &fakeUpgradeDownloaderFactory{
				wantParams: &templatesource.ForUpgradeParams{
					LocType:           "fake_location_type",
					CanonicalLocation: "fake_canonical_source",
					Version:           "fake_upgrade_channel",
				},
				outDownloader: &fakeDownloader{
					outDLMeta: &templatesource.DownloadMetadata{
						IsCanonical:     true,
						CanonicalSource: "fake_canonical_source",
						LocationType:    "fake_location_type",
						Version:         "fake_version",
						UpgradeChannel:  "fake_upgrade_channel",
					},
				},
			},
			want: &Result{
				Overall: Success,
				Results: []*ManifestResult{
					{
						ManifestPath: ".",
						Type:         Success,
						NonConflicts: []ActionTaken{
							{Action: "noop", Path: "out.txt"},
							{Action: "writeNew", Path: "some_other_file.txt"},
						},
						DLMeta: &templatesource.DownloadMetadata{
							IsCanonical:     true,
							CanonicalSource: "fake_canonical_source",
							LocationType:    "fake_location_type",
							Version:         "fake_version",
							UpgradeChannel:  "fake_upgrade_channel",
						},
					},
				},
			},
			wantDestContentsAfterUpgrade: map[string]string{
				"out.txt":             "identical contents",
				"some_other_file.txt": "some other file contents",
			},
			wantManifestAfterUpgrade: manifestWith(outTxtOnlyManifest, func(m *manifest.Manifest) {
				m.TemplateLocation.Val = "fake_canonical_source"
				m.LocationType.Val = "fake_location_type"
				m.TemplateVersion.Val = "fake_version"
				m.UpgradeChannel.Val = "fake_upgrade_channel"
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
			want:                     &Result{}, // errors are checked separately
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
            with: 'red'`,
			},
			origDestContents: map[string]string{
				"file.txt": "purple is my favorite color\n",
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
+purple is my favorite color
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
				Overall: Success,
				Results: []*ManifestResult{
					{
						ManifestPath: ".",
						Type:         Success,
						NonConflicts: []ActionTaken{
							{
								Action: WriteNew,
								Path:   "file.txt",
							},
						},
						DLMeta: wantDLMeta,
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
						Patch: mdl.SP(`--- a/file.txt
+++ b/file.txt
@@ -1 +1 @@
-yellow is my favorite color
+purple is my favorite color
`),
					},
				},
			},
			wantDestContentsAfterUpgrade: map[string]string{
				"file.txt": "yellow is my favorite color\n",
			},
		},
		{
			name: "rejected_reversal_include_from_destination_with_local_edits",
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
				"file.txt": "purple is my favorite color\n",
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
+purple is my favorite color
`),
					},
				},
			},
			localEdits: func(tb testing.TB, installedDir string) {
				tb.Helper()
				abctestutil.Overwrite(tb, installedDir, "file.txt", "green is my favorite color\n")
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
				Overall: PatchReversalConflict,
				Results: []*ManifestResult{
					{
						ManifestPath: ".",
						DLMeta:       wantDLMeta,
						Type:         PatchReversalConflict,
						ReversalConflicts: []*ReversalConflict{
							{
								RelPath:       "file.txt",
								AbsPath:       "file.txt",
								RejectedHunks: "file.txt.patch.rej",
							},
						},
					},
				},
			},
			// manifest should be unchanged if there's a reversal conflict
			wantManifestAfterUpgrade: &manifest.Manifest{
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
+purple is my favorite color
`),
					},
				},
			},
			wantRejectFile: "file.txt.patch.rej",
			wantDestContentsAfterUpgrade: map[string]string{
				"file.txt": "green is my favorite color\n",
			},
		},

		{
			name: "fuzzy_patch_reversal",
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
				"file.txt": "purple is my favorite color\n",
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
+purple is my favorite color
`),
					},
				},
			},
			localEdits: func(tb testing.TB, installedDir string) {
				tb.Helper()
				abctestutil.Prepend(tb, installedDir, "file.txt", "an arbitrary line of text to trigger fuzzy patching\n")
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
				Overall: Success,
				Results: []*ManifestResult{
					{
						ManifestPath: ".",
						Type:         Success,
						NonConflicts: []ActionTaken{
							{
								Action: "writeNew",
								Path:   "file.txt",
							},
						},
						DLMeta: wantDLMeta,
					},
				},
			},
			wantDestContentsAfterUpgrade: map[string]string{
				"file.txt": `an arbitrary line of text to trigger fuzzy patching
yellow is my favorite color
`,
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
						Patch: mdl.SP(`--- a/file.txt
+++ b/file.txt
@@ -1,2 +1,2 @@
 an arbitrary line of text to trigger fuzzy patching
-yellow is my favorite color
+purple is my favorite color
`),
					},
				},
			},
		},

		// TODO(upgrade): add tests:
		//  multiple conflicting files
		//  multiple templates targeting same file causing reversal conflict
		//  some hunks applied and some rejected
		//  a previous regular-included file becomes ifd
		//  an ifd file becomes regular-included
		//  IFD with all conflicts: add-add, add-edit, EditDelete, DeleteEdit
		//  a mix of included-from-dest and regular-include
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempBase := t.TempDir()
			// Make tempBase into a valid git repo.
			abctestutil.WriteAll(t, tempBase, abctestutil.WithGitRepoAt("", nil))

			destDir := filepath.Join(tempBase, "dest_dir")
			templateDir := filepath.Join(tempBase, "template_dir")

			abctestutil.WriteAll(t, destDir, tc.origDestContents)

			ctx := context.Background()

			abctestutil.WriteAll(t, templateDir, tc.origTemplateDirContents)
			clk := clock.NewMock()
			clk.Set(beforeUpgradeTime)

			if tc.fakeDownloader != nil {
				tc.fakeDownloader.sourceDir = templateDir // inject per-testcase value that's not known when the testcase is created
			}
			renderResult := mustRender(t, ctx, clk, tc.fakeDownloader, tempBase, templateDir, destDir)

			manifestFullPath := filepath.Join(destDir, renderResult.ManifestPath)

			assertManifest(ctx, t, "before upgrade", tc.wantManifestBeforeUpgrade, manifestFullPath)

			clk.Set(afterUpgradeTime) // simulate time passing between initial installation and upgrade

			var dlFactory downloaderFactory
			if tc.fakeUpgradeDownloaderFactory != nil {
				dlFactory = tc.fakeUpgradeDownloaderFactory.New
				tc.fakeUpgradeDownloaderFactory.tb = t
				tc.fakeUpgradeDownloaderFactory.outDownloader.sourceDir = templateDir
			}

			params := &Params{
				Clock:             clk,
				CWD:               destDir,
				FS:                &common.RealFS{},
				Location:          manifestFullPath,
				Stdout:            os.Stdout,
				downloaderFactory: dlFactory,
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

			upgradeResult := UpgradeAll(ctx, params)
			if diff := testutil.DiffErrString(upgradeResult.Err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}

			opts := []cmp.Option{
				cmpopts.EquateEmpty(),
				cmpopts.IgnoreFields(ActionTaken{}, "Explanation"), // don't assert on debugging messages. That would make test cases overly verbose.
				cmpopts.IgnoreFields(Result{}, "Err"),              // errors are verified separately
				abctestutil.TransformStructFields(
					abctestutil.TrimStringPrefixTransformer(destDir+"/"),
					ReversalConflict{},
					"AbsPath", "RejectedHunks",
				),
			}
			if diff := cmp.Diff(upgradeResult, tc.want, opts...); diff != "" {
				t.Errorf("result was not as expected, diff is (-got, +want): %v", diff)
			}

			assertManifest(ctx, t, "after upgrade", tc.wantManifestAfterUpgrade, manifestFullPath)

			gotDestContentsAfter := abctestutil.LoadDir(t, destDir,
				abctestutil.SkipGlob(".abc/manifest*"),
				abctestutil.SkipGlob("*.patch.rej"), // rejected hunk files are asserted separately
			)
			if diff := cmp.Diff(gotDestContentsAfter, tc.wantDestContentsAfterUpgrade); diff != "" {
				t.Errorf("installed directory contents after upgrading were not as expected (-got,+want): %s", diff)
			}

			if tc.wantRejectFile != "" {
				ok, err := common.Exists(filepath.Join(destDir, tc.wantRejectFile))
				if err != nil {
					t.Fatal(err)
				}
				if !ok {
					t.Errorf("reject file %q was missing", tc.wantRejectFile)
				}
			}
		})
	}
}

type fakeUpgradeDownloaderFactory struct {
	tb testing.TB

	wantParams    *templatesource.ForUpgradeParams
	outDownloader *fakeDownloader
}

func (f *fakeUpgradeDownloaderFactory) New(_ context.Context, p *templatesource.ForUpgradeParams) (templatesource.Downloader, error) {
	opts := []cmp.Option{cmpopts.IgnoreFields(templatesource.ForUpgradeParams{}, "InstalledDir")}
	if diff := cmp.Diff(p, f.wantParams, opts...); diff != "" {
		f.tb.Fatalf("upgrade params were not as expected (-got,+want): %s", diff)
	}
	return f.outDownloader, nil
}

type fakeDownloader struct {
	sourceDir string
	outDLMeta *templatesource.DownloadMetadata
}

func (f *fakeDownloader) Download(ctx context.Context, cwd, templateDir, destDir string) (*templatesource.DownloadMetadata, error) {
	if err := common.CopyRecursive(ctx, nil, &common.CopyParams{
		SrcRoot: f.sourceDir,
		DstRoot: templateDir,
		FS:      &common.RealFS{},
	}); err != nil {
		return nil, err
	}

	return f.outDLMeta, nil
}

// TODO(upgrade): test non-canonical upgrade with a remote foo@main style template.
func TestUpgrade_NonCanonical(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tempBase := t.TempDir()
	templateDir := filepath.Join(tempBase, "template_dir")
	destDir := filepath.Join(tempBase, "dest")
	if err := os.MkdirAll(templateDir, common.OwnerRWXPerms); err != nil {
		t.Fatal(err)
	}

	origTemplateDirContents := map[string]string{
		"out.txt":   "hello\n",
		"spec.yaml": includeDotSpec,
	}
	abctestutil.WriteAll(t, templateDir, origTemplateDirContents)
	clk := clock.NewMock()
	clk.Set(time.Date(2024, 3, 1, 4, 5, 6, 7, time.UTC))
	mustRender(t, ctx, clk, nil, tempBase, templateDir, destDir)

	clk.Add(time.Second)
	params := &Params{
		Clock:    clk,
		CWD:      tempBase,
		FS:       &common.RealFS{},
		Location: destDir,
	}

	// Attempting to upgrade without a canonical location should fail
	result := UpgradeAll(ctx, params)
	wantErr := "this template was installed without a canonical location; please use the --template-location flag to specify where to upgrade from"
	if diff := testutil.DiffErrString(result.Err, wantErr); diff != "" {
		t.Fatal(diff)
	}

	// "Upgrading" to the original template should return "already up to date"
	params.TemplateLocation = templateDir
	result = UpgradeAll(ctx, params)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if result.Overall != AlreadyUpToDate {
		t.Fatalf("got result.Overall %q, want %q", result.Overall, AlreadyUpToDate)
	}

	// Now modify the template so there's something to actually do during upgrade
	abctestutil.Overwrite(t, templateDir, "out.txt", "new contents")
	result = UpgradeAll(ctx, params)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if result.Overall != Success {
		t.Fatalf("got result.Overall %q, want %q", result.Overall, Success)
	}
}

func TestPatchReversalManualResolution(t *testing.T) {
	t.Parallel()

	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("time.LoadLocation(): %v", err)
	}
	renderTime1 := time.Date(2024, 3, 1, 4, 5, 6, 7, loc)

	tempBase := t.TempDir()
	// Make tempBase into a valid git repo.
	abctestutil.WriteAll(t, tempBase, abctestutil.WithGitRepoAt("", nil))

	templateDir := filepath.Join(tempBase, "template_dir")

	origTemplateDirContents := map[string]string{
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
	}
	abctestutil.WriteAll(t, templateDir, origTemplateDirContents)

	origDestContents := map[string]string{
		"file.txt": "purple is my favorite color\n",
	}

	// We have multiple template installations so we can test resuming after
	// resolving a conflict.
	destDirsBase := filepath.Join(tempBase, "dest")
	destDir1 := filepath.Join(destDirsBase, "1")
	destDir2 := filepath.Join(destDirsBase, "2")

	abctestutil.WriteAll(t, destDir1, origDestContents)
	abctestutil.WriteAll(t, destDir2, origDestContents)

	ctx := context.Background()
	clk := clock.NewMock()
	clk.Set(renderTime1)
	renderResult1 := mustRender(t, ctx, clk, nil, tempBase, templateDir, destDir1)

	wantManifestBeforeUpgrade := &manifest.Manifest{
		CreationTime:     renderTime1,
		ModificationTime: renderTime1,
		TemplateLocation: mdl.S("../../template_dir"),
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
+purple is my favorite color
`),
			},
		},
	}
	manifestFullPath := filepath.Join(destDir1, renderResult1.ManifestPath)
	assertManifest(ctx, t, "before upgrade", wantManifestBeforeUpgrade, manifestFullPath)

	clk.Add(time.Second)
	mustRender(t, ctx, clk, nil, tempBase, templateDir, destDir2) // don't bother checking the manifest for the second render; it's the same as the first one

	// Simulate the user making some edits to the included-from-destination file
	// after the render operation but before the upgrade
	abctestutil.Overwrite(t, destDir1, "file.txt", "green is my favorite color\n")
	abctestutil.Overwrite(t, destDir2, "file.txt", "green is my favorite color\n")

	templateReplacementForUpgrade := map[string]string{
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
	}
	abctestutil.WriteAll(t, templateDir, templateReplacementForUpgrade)

	clk.Add(time.Second)
	upgradeParams := &Params{
		Clock:    clk,
		CWD:      destDirsBase,
		FS:       &common.RealFS{},
		Location: destDirsBase,
		Stdout:   os.Stdout,
	}

	result := UpgradeAll(ctx, upgradeParams)
	if result.Err != nil {
		t.Fatal(err)
	}
	wantResult := &Result{
		Overall: PatchReversalConflict,
		Results: []*ManifestResult{
			{
				ManifestPath: "1/.abc/manifest_..%2F..%2Ftemplate_dir_2024-03-01T12:05:06.000000007Z.lock.yaml",
				Type:         PatchReversalConflict,
				ReversalConflicts: []*ReversalConflict{
					{
						RelPath:       "file.txt",
						AbsPath:       filepath.Join(destDir1, "file.txt"),
						RejectedHunks: filepath.Join(destDir1, "file.txt.patch.rej"),
					},
				},
				DLMeta: &templatesource.DownloadMetadata{
					IsCanonical:     true,
					CanonicalSource: "../../template_dir",
					LocationType:    "local_git",
					Version:         abctestutil.MinimalGitHeadSHA,
					Vars: templatesource.DownloaderVars{
						GitSHA:      abctestutil.MinimalGitHeadSHA,
						GitShortSHA: abctestutil.MinimalGitHeadShortSHA,
					},
				},
			},
		},
	}
	opts := []cmp.Option{
		cmpopts.EquateEmpty(),
		cmpopts.IgnoreFields(ActionTaken{}, "Explanation"), // don't assert on debugging messages. That would make test cases overly verbose.
	}
	if diff := cmp.Diff(result, wantResult, opts...); diff != "" {
		t.Errorf("result was not as expected, diff is (-got, +want): %v", diff)
	}

	// manifest should be unchanged if there's a reversal conflict
	wantDestContentsAfterFailedUpgrade := map[string]string{
		"file.txt": "green is my favorite color\n",

		// Don't assert the contents of the .rej file, because its contents vary
		// between macos and linux due to the differences between freebsd patch
		// and GNU patch.
		//
		// "file.txt.patch.rej": ... ,
	}

	wantManifestAfterFailedUpgrade := wantManifestBeforeUpgrade
	assertManifest(ctx, t, "after upgrade", wantManifestAfterFailedUpgrade, manifestFullPath)

	gotDestContentsAfterFailedUpgrade := abctestutil.LoadDir(t, destDir1,
		abctestutil.SkipGlob(".abc/manifest*"),     // the manifest is verified separately
		abctestutil.SkipGlob("file.txt.patch.rej"), // the patch reject file is just checked for presence, separately
	)
	if diff := cmp.Diff(gotDestContentsAfterFailedUpgrade, wantDestContentsAfterFailedUpgrade); diff != "" {
		t.Errorf("installed directory contents after upgrading were not as expected (-got,+want): %s", diff)
	}
	ok, err := common.Exists(filepath.Join(destDir1, "file.txt.patch.rej"))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("got no file.txt.patch.rej rejected patch file, but wanted one")
	}

	// Resolve the merge conflict
	abctestutil.Overwrite(t, destDir1, "file.txt", "purple is my favorite color\n")
	abctestutil.Remove(t, destDir1, "file.txt.patch.rej")

	// Inform the upgrade command that patch reversal has already happened
	upgradeParams.AlreadyResolved = []string{"file.txt"}
	upgradeParams.ResumeFrom = "1/.abc/manifest_..%2F..%2Ftemplate_dir_2024-03-01T12:05:06.000000007Z.lock.yaml"

	result = UpgradeAll(ctx, upgradeParams)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	wantResult = &Result{
		Overall: PatchReversalConflict,
		Results: []*ManifestResult{
			{
				Type:         Success,
				ManifestPath: "1/.abc/manifest_..%2F..%2Ftemplate_dir_2024-03-01T12:05:06.000000007Z.lock.yaml",
				NonConflicts: []ActionTaken{
					{
						Action: "writeNew",
						Path:   "file.txt",
					},
				},
				DLMeta: &templatesource.DownloadMetadata{
					IsCanonical:     true,
					CanonicalSource: "../../template_dir",
					LocationType:    templatesource.LocalGit,
					Version:         abctestutil.MinimalGitHeadSHA,
					Vars: templatesource.DownloaderVars{
						GitSHA:      abctestutil.MinimalGitHeadSHA,
						GitShortSHA: abctestutil.MinimalGitHeadShortSHA,
					},
				},
			},
			{
				Type:         PatchReversalConflict,
				ManifestPath: "2/.abc/manifest_..%2F..%2Ftemplate_dir_2024-03-01T12:05:07.000000007Z.lock.yaml",
				ReversalConflicts: []*ReversalConflict{
					{
						RelPath:       "file.txt",
						AbsPath:       filepath.Join(destDir2, "file.txt"),
						RejectedHunks: filepath.Join(destDir2, "file.txt.patch.rej"),
					},
				},
				DLMeta: &templatesource.DownloadMetadata{
					IsCanonical:     true,
					CanonicalSource: "../../template_dir",
					LocationType:    "local_git",
					Version:         abctestutil.MinimalGitHeadSHA,
					Vars: templatesource.DownloaderVars{
						GitSHA:      abctestutil.MinimalGitHeadSHA,
						GitShortSHA: abctestutil.MinimalGitHeadShortSHA,
					},
				},
			},
		},
	}
	if diff := cmp.Diff(result, wantResult, opts...); diff != "" {
		t.Errorf("result was not as expected, diff is (-got, +want): %v", diff)
	}

	wantDestContentsAfterSuccessfulUpgrade := map[string]string{
		"file.txt": "yellow is my favorite color\n",
	}
	gotDestContentsAfterSuccessfulUpgrade := abctestutil.LoadDir(t, destDir1, abctestutil.SkipGlob(".abc/manifest*"))
	if diff := cmp.Diff(gotDestContentsAfterSuccessfulUpgrade, wantDestContentsAfterSuccessfulUpgrade); diff != "" {
		t.Errorf("installed directory contents after upgrading were not as expected (-got,+want): %s", diff)
	}

	// Resolve the merge conflict
	abctestutil.Overwrite(t, destDir2, "file.txt", "purple is my favorite color\n")
	abctestutil.Remove(t, destDir2, "file.txt.patch.rej")

	// Inform the upgrade command that patch reversal has already happened
	upgradeParams.AlreadyResolved = []string{"file.txt"}
	upgradeParams.ResumeFrom = "2/.abc/manifest_..%2F..%2Ftemplate_dir_2024-03-01T12:05:07.000000007Z.lock.yaml"

	result = UpgradeAll(ctx, upgradeParams)
	if result.Err != nil {
		t.Fatal(result.Err)
	}

	wantResult = &Result{
		Overall: Success,
		Results: []*ManifestResult{
			{
				Type:         Success,
				ManifestPath: "2/.abc/manifest_..%2F..%2Ftemplate_dir_2024-03-01T12:05:07.000000007Z.lock.yaml",
				NonConflicts: []ActionTaken{
					{
						Action: "writeNew",
						Path:   "file.txt",
					},
				},
				DLMeta: &templatesource.DownloadMetadata{
					IsCanonical:     true,
					CanonicalSource: "../../template_dir",
					LocationType:    templatesource.LocalGit,
					Version:         abctestutil.MinimalGitHeadSHA,
					Vars: templatesource.DownloaderVars{
						GitSHA:      abctestutil.MinimalGitHeadSHA,
						GitShortSHA: abctestutil.MinimalGitHeadShortSHA,
					},
				},
			},
		},
	}

	if diff := cmp.Diff(result, wantResult, opts...); diff != "" {
		t.Errorf("result was not as expected, diff is (-got, +want): %v", diff)
	}
}

func TestDetectUnmergedConflicts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{
			name:  "no_conflicts",
			files: map[string]string{"file.txt": ""},
		},
		{
			name:    "abcmerge_detected",
			files:   map[string]string{"file.txt.abcmerge_locally_added": ""},
			wantErr: "file.txt.abcmerge_locally_added",
		},
		{
			name:    "subdir",
			files:   map[string]string{"dir1/file.txt.abcmerge_locally_added": ""},
			wantErr: "dir1/file.txt.abcmerge_locally_added",
		},
		{
			name:    "rejected_patch",
			files:   map[string]string{"file.txt.patch.rej": ""},
			wantErr: "file.txt.patch.rej",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			abctestutil.WriteAll(t, tempDir, tc.files)
			err := detectUnmergedConflicts(tempDir)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}
		})
	}
}

// TODO(upgrade): add tests:
//   - upgrade multiple with already-resolved
//   - upgrade template-that-outputs-template
func TestUpgradeAll_MultipleTemplates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	clk := clock.NewMock()

	tempBase := t.TempDir()

	// Make the temp dir into a git repo so template locations will be treated
	// as canonical.
	abctestutil.WriteAll(t, tempBase, abctestutil.WithGitRepoAt("", nil))

	template1Files := map[string]string{
		"spec.yaml":  includeDotSpec,
		"myfile.txt": "my old template1 file contents",
	}
	template2Files := map[string]string{
		"spec.yaml":  includeDotSpec,
		"myfile.txt": "my old template2 file contents",
	}

	templateDir1 := filepath.Join(tempBase, "templateDir1")
	templateDir2 := filepath.Join(tempBase, "templateDir2")
	destBase := filepath.Join(tempBase, "dest")
	destDir1 := filepath.Join(destBase, "destDir1")
	destDir2 := filepath.Join(destBase, "destDir2")
	abctestutil.WriteAll(t, templateDir1, template1Files)
	abctestutil.WriteAll(t, templateDir2, template2Files)
	mustRender(t, ctx, clk, nil, tempBase, templateDir1, destDir1)
	mustRender(t, ctx, clk, nil, tempBase, templateDir2, destDir2)

	upgradedTemplate1Files := map[string]string{
		"spec.yaml":  includeDotSpec,
		"myfile.txt": "my new template1 file contents",
	}
	upgradedTemplate2Files := map[string]string{
		"spec.yaml":  includeDotSpec,
		"myfile.txt": "my new template2 file contents",
	}

	abctestutil.WriteAll(t, templateDir1, upgradedTemplate1Files)
	abctestutil.WriteAll(t, templateDir2, upgradedTemplate2Files)

	allResult := UpgradeAll(ctx, &Params{
		Clock:    clk,
		CWD:      tempBase,
		FS:       &common.RealFS{},
		Location: tempBase,
		Stdout:   os.Stdout,
	})

	if allResult.Err != nil {
		t.Fatal(allResult.Err)
	}

	if len(allResult.Results) != 2 {
		t.Errorf("got %d results, expected exactly 2", len(allResult.Results))
	}
	for _, result := range allResult.Results {
		if result.Type != Success {
			t.Fatalf("got upgrade result %q, expected %q", result.Type, Success)
		}
	}

	wantDestContents := map[string]string{
		"destDir1/myfile.txt": "my new template1 file contents",
		"destDir2/myfile.txt": "my new template2 file contents",
	}
	opt := abctestutil.SkipGlob("*/.abc/manifest*") // manifest are too unpredictable, don't assert their contents
	gotDestContents := abctestutil.LoadDir(t, destBase, opt)
	if diff := cmp.Diff(gotDestContents, wantDestContents); diff != "" {
		t.Errorf("dest contents were not as expected (-got,+want):\n%s", diff)
	}
}

func TestUpgradeAll_MultipleTemplatesWithResumedConflict(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	clk := clock.NewMock()

	tempBase := t.TempDir()

	// Make the temp dir into a git repo so template locations will be treated
	// as canonical.
	abctestutil.WriteAll(t, tempBase, abctestutil.WithGitRepoAt("", nil))

	template1Files := map[string]string{
		"spec.yaml":  includeDotSpec,
		"myfile.txt": "my old template1 file contents",
	}
	template2Files := map[string]string{
		"spec.yaml":  includeDotSpec,
		"myfile.txt": "my old template2 file contents",
	}

	templateDir1 := filepath.Join(tempBase, "templateDir1")
	templateDir2 := filepath.Join(tempBase, "templateDir2")
	destBase := filepath.Join(tempBase, "dest")
	destDir1 := filepath.Join(destBase, "destDir1")
	destDir2 := filepath.Join(destBase, "destDir2")
	abctestutil.WriteAll(t, templateDir1, template1Files)
	abctestutil.WriteAll(t, templateDir2, template2Files)
	mustRender(t, ctx, clk, nil, tempBase, templateDir1, destDir1)
	mustRender(t, ctx, clk, nil, tempBase, templateDir2, destDir2)

	abctestutil.Overwrite(t, destDir1, "myfile.txt", "my local edits")

	upgradedTemplate1Files := map[string]string{
		"spec.yaml":  includeDotSpec,
		"myfile.txt": "my new template1 file contents",
	}
	upgradedTemplate2Files := map[string]string{
		"spec.yaml":  includeDotSpec,
		"myfile.txt": "my new template2 file contents",
	}

	abctestutil.WriteAll(t, templateDir1, upgradedTemplate1Files)
	abctestutil.WriteAll(t, templateDir2, upgradedTemplate2Files)

	upgradeParams := &Params{
		Clock:    clk,
		CWD:      tempBase,
		FS:       &common.RealFS{},
		Location: tempBase,
		Stdout:   os.Stdout,
	}
	allResult := UpgradeAll(ctx, upgradeParams)
	if allResult.Err != nil {
		t.Fatal(allResult.Err)
	}
	if allResult.Overall != MergeConflict {
		t.Errorf("got overall result %q, want %q", allResult.Overall, MergeConflict)
	}

	if len(allResult.Results) != 1 {
		t.Errorf("got %d results, expected exactly 1", len(allResult.Results))
	}
	result := allResult.Results[0]

	if result.Type != MergeConflict {
		t.Fatalf("got result type %q, wanted %q", result.Type, MergeConflict)
	}

	wantDestContents := map[string]string{
		"destDir1/myfile.txt" + SuffixFromNewTemplate: "my new template1 file contents",
		"destDir1/myfile.txt":                         "my local edits",
		"destDir2/myfile.txt":                         "my old template2 file contents",
	}
	opt := abctestutil.SkipGlob("*/.abc/manifest*") // manifest are too unpredictable, don't assert their contents
	gotDestContents := abctestutil.LoadDir(t, destBase, opt)
	if diff := cmp.Diff(gotDestContents, wantDestContents); diff != "" {
		t.Errorf("dest contents were not as expected (-got,+want):\n%s", diff)
	}

	abctestutil.Overwrite(t, destDir1, "myfile.txt", "my resolved contents")
	abctestutil.Remove(t, destDir1, "myfile.txt"+SuffixFromNewTemplate)

	allResult = UpgradeAll(ctx, upgradeParams)
	if allResult.Err != nil {
		t.Fatal(allResult.Err)
	}
	if allResult.Overall != Success {
		t.Errorf("got overall result %q, want %q", allResult.Overall, Success)
	}

	if len(allResult.Results) != 2 {
		t.Fatalf("got %d results, expected exactly 2", len(allResult.Results))
	}
	if got := allResult.Results[0].Type; got != AlreadyUpToDate {
		t.Fatalf("got result[0] %q, expected %q", got, AlreadyUpToDate)
	}
	if got := allResult.Results[1].Type; got != Success {
		t.Fatalf("got result[1] %q, expected %q", got, Success)
	}

	wantDestContents = map[string]string{
		"destDir1/myfile.txt": "my resolved contents",
		"destDir2/myfile.txt": "my new template2 file contents",
	}
	gotDestContents = abctestutil.LoadDir(t, destBase, opt)
	if diff := cmp.Diff(gotDestContents, wantDestContents); diff != "" {
		t.Errorf("dest contents were not as expected (-got,+want):\n%s", diff)
	}
}

func TestUpgradeAll_Dependency(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	clk := clock.NewMock()

	tempBase := t.TempDir()

	// Make the temp dir into a git repo so template locations will be treated
	// as canonical.
	abctestutil.WriteAll(t, tempBase, abctestutil.WithGitRepoAt("", nil))

	template1Files := map[string]string{
		"spec.yaml":             includeDotSpec,
		"outer_output_file.txt": "my old outer output file",
		"inner/spec.yaml":       includeDotSpec,
		"inner/output.txt":      "my old inner output file",
	}
	template2Files := map[string]string{
		"spec.yaml":  includeDotSpec,
		"myfile.txt": "my old template2 file contents",
	}

	templateDir1 := filepath.Join(tempBase, "templateDir1")
	templateDir2 := filepath.Join(tempBase, "templateDir2")
	destBase := filepath.Join(tempBase, "dest")
	destDirA := filepath.Join(destBase, "destDirA")
	destDirB := filepath.Join(destBase, "destDirB")
	destDirC := filepath.Join(destBase, "destDirC")
	abctestutil.WriteAll(t, templateDir1, template1Files)
	abctestutil.WriteAll(t, templateDir2, template2Files)
	mustRender(t, ctx, clk, nil, tempBase, templateDir1, destDirA)
	mustRender(t, ctx, clk, nil, tempBase, templateDir2, destDirB)
	mustRender(t, ctx, clk, nil, tempBase, destDirA+"/inner", destDirC)

	wantRendered := map[string]string{
		"destDirA/outer_output_file.txt": "my old outer output file",
		"destDirA/inner/spec.yaml":       includeDotSpec,
		"destDirA/inner/output.txt":      "my old inner output file",
		"destDirB/myfile.txt":            "my old template2 file contents",
		"destDirC/output.txt":            "my old inner output file",
	}
	gotRendered := abctestutil.LoadDir(t, destBase,
		abctestutil.SkipGlob("*/.abc/*"), // manifests are too unpredictable, don't assert their contents
	)
	if diff := cmp.Diff(gotRendered, wantRendered); diff != "" {
		t.Fatalf("initially rendered template output was not as expected (-got,+want):\n%s", diff)
	}

	upgradedTemplate1Files := map[string]string{
		"outer_output_file.txt": "my new outer output file",
		"inner/output.txt":      "my new inner output file",
	}
	upgradedTemplate2Files := map[string]string{
		"spec.yaml":  includeDotSpec,
		"myfile.txt": "my new template2 file contents",
	}

	abctestutil.WriteAll(t, templateDir1, upgradedTemplate1Files)
	abctestutil.WriteAll(t, templateDir2, upgradedTemplate2Files)

	result := UpgradeAll(ctx, &Params{
		Clock:    clk,
		CWD:      tempBase,
		FS:       &common.RealFS{},
		Location: destBase,
		Stdout:   os.Stdout,
	})

	if result.Err != nil {
		t.Fatal(result.Err)
	}

	const wantResults = 3
	if len(result.Results) != wantResults {
		t.Errorf("got %d results, expected exactly %d", len(result.Results), wantResults)
	}
	for _, r := range result.Results {
		if r.Type != Success {
			t.Fatalf("got upgrade result %q, expected %q", r.Type, Success)
		}
	}

	wantDestContents := map[string]string{
		"destDirA/outer_output_file.txt": "my new outer output file",
		"destDirA/inner/spec.yaml":       includeDotSpec,
		"destDirA/inner/output.txt":      "my new inner output file",
		"destDirB/myfile.txt":            "my new template2 file contents",
		"destDirC/output.txt":            "my new inner output file",
	}
	opt := abctestutil.SkipGlob("*/.abc/*") // manifests are too unpredictable, don't assert their contents
	gotDestContents := abctestutil.LoadDir(t, destBase, opt)
	if diff := cmp.Diff(gotDestContents, wantDestContents); diff != "" {
		t.Errorf("dest contents were not as expected (-got,+want):\n%s", diff)
	}

	// Since destDirC depends on destDirA (because destDirA contains the
	// spec.yaml that was used to render destDirC), then destDirA should come
	// before destDirC in the
	indexA := mustIndexFunc(t, result.Results, func(mr *ManifestResult) bool {
		return strings.Contains(mr.ManifestPath, "destDirA")
	})
	indexC := mustIndexFunc(t, result.Results, func(mr *ManifestResult) bool {
		return strings.Contains(mr.ManifestPath, "destDirC")
	})
	if indexA > indexC {
		t.Errorf("upgrades were out of order. destDirC index %d should be less than destDirA index %d",
			indexC, indexA)
	}
	cResult := result.Results[indexC]
	if len(cResult.DependedOn) == 0 {
		t.Errorf("destDirC showed no dependencies, sbut should have depended on destDirA")
	} else {
		const wantPrefix = "destDirA/.abc/manifest"
		if !strings.HasPrefix(cResult.DependedOn[0], wantPrefix) {
			t.Errorf("destDirC depended on %s, but should have a dependency beginning with %q",
				cResult.DependedOn[0], wantPrefix)
		}
	}
}

// mustIndexFunc is a wrapper around slices.IndexFunc that saves the caller from
// worrying about the case where nothing is found and -1 is returned.
func mustIndexFunc[T any](t *testing.T, s []T, f func(T) bool) int {
	t.Helper()

	idx := slices.IndexFunc(s, f)
	if idx < 0 {
		t.Fatal("IndexFunc returned -1")
	}
	return idx
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

func mustRender(tb testing.TB, ctx context.Context, clk clock.Clock, fakeDL *fakeDownloader, tempBase, templateDir, destDir string) *render.Result {
	tb.Helper()

	var downloader templatesource.Downloader = fakeDL
	if fakeDL == nil {
		var err error
		downloader, err = templatesource.ParseSource(ctx, &templatesource.ParseSourceParams{
			CWD:    tempBase,
			Source: templateDir,
		})
		if err != nil {
			tb.Fatal(err)
		}
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
