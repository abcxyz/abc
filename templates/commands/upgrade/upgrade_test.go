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
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/benbjohnson/clock"
	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/render"
	"github.com/abcxyz/abc/templates/common/templatesource"
	"github.com/abcxyz/abc/templates/common/upgrade"
	abctestutil "github.com/abcxyz/abc/templates/testutil"
	"github.com/abcxyz/abc/templates/testutil/prompt"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/testutil"
)

func TestUpgradeCommand(t *testing.T) {
	// Some of this is copied from the tests in common/upgrade.

	t.Parallel()

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

	cases := []struct {
		name                    string
		origTemplateDirContents map[string]string
		localEdits              func(tb testing.TB, installedDir string)
		upgradedTemplate        map[string]string
		initialDestContents     map[string]string

		wantExitCode int
		wantStdout   string
		wantErr      []string
	}{
		{
			name: "noop_because_template_is_already_up_to_date",
			origTemplateDirContents: map[string]string{
				"out.txt":   "hello, world\n",
				"spec.yaml": includeDotSpec,
			},
			upgradedTemplate: map[string]string{
				"out.txt":   "hello, world\n",
				"spec.yaml": includeDotSpec,
			},
			wantStdout: "Already up to date with latest template version\n",
		},
		{
			// The user manually added a file, and the upgraded template added a
			// file, but the contents happen to be the same, so there's no
			// conflict.
			name: "merges_automatically_resolved_without_conflicts",
			origTemplateDirContents: map[string]string{
				"spec.yaml": includeDotSpec,
			},
			localEdits: func(tb testing.TB, installedDir string) {
				tb.Helper()
				abctestutil.OverwriteJoin(tb, installedDir, "greet.txt", "goodbye\n")
			},
			upgradedTemplate: map[string]string{
				"spec.yaml": includeDotSpec,
				"greet.txt": "goodbye\n",
			},
			wantStdout: "Upgrade complete with no conflicts\n",
		},
		{
			name: "conflicts",
			origTemplateDirContents: map[string]string{
				"greet.txt": "hello, world\n",
				"spec.yaml": includeDotSpec,
			},
			upgradedTemplate: map[string]string{
				"greet.txt": "hello, venus\n",
				"color.txt": "blue",
				"spec.yaml": includeDotSpec,
			},
			localEdits: func(tb testing.TB, installedDir string) {
				tb.Helper()
				abctestutil.OverwriteJoin(tb, installedDir, "greet.txt", "hello, mars\n")
				abctestutil.OverwriteJoin(tb, installedDir, "color.txt", "red\n")
			},
			wantExitCode: 1,
			wantErr:      []string{"exit code 1"},
			wantStdout: `When upgrading manifest TEMPDIR/dest_dir/.abc/manifest_.._template_dir_1970-01-01T00:00:00Z.lock.yaml:
` + mergeInstructions + `

List of conflicting files:
--
file: color.txt
conflict type: addAddConflict
incoming file: color.txt.abcmerge_from_new_template
--
file: greet.txt
conflict type: editEditConflict
incoming file: greet.txt.abcmerge_from_new_template
--

After manually resolving the merge conflict, re-run the upgrade command to
upgrade any other rendered templates in this location that may still need
upgrading.
`,
		},
		{
			name:                "patch_reversal_conflict",
			initialDestContents: map[string]string{"hello.txt": "a\nb\nc\n"},
			origTemplateDirContents: map[string]string{
				"spec.yaml": `
api_version: 'cli.abcxyz.dev/v1beta6'
kind: 'Template'

desc: 'my template'

steps:
  - desc: 'include'
    action: 'include'
    params:
      from: 'destination'
      paths: ['hello.txt']
  - desc: 'replace b with B'
    action: 'string_replace'
    params:
      paths: ['hello.txt']
      replacements:
        - to_replace: "b"
          with: "X"`,
			},
			localEdits: func(tb testing.TB, installedDir string) {
				tb.Helper()
				abctestutil.OverwriteJoin(tb, installedDir, "hello.txt", "a\nY\nc\n")
			},
			upgradedTemplate: map[string]string{
				"spec.yaml": `
api_version: 'cli.abcxyz.dev/v1beta6'
kind: 'Template'

desc: 'my template'

steps:
  - desc: 'include'
    action: 'include'
    params:
      from: 'destination'
      paths: ['hello.txt']
  - desc: 'replace b with B'
    action: 'string_replace'
    params:
      paths: ['hello.txt']
      replacements:
        - to_replace: "b"
          with: "Z"`,
			},
			wantExitCode: 2,
			wantStdout: `When upgrading manifest TEMPDIR/dest_dir/.abc/manifest_.._template_dir_1970-01-01T00:00:00Z.lock.yaml:
` + patchReversalInstructions + `

--
your file: TEMPDIR/dest_dir/hello.txt
Rejected hunks for you to apply: TEMPDIR/dest_dir/hello.txt.patch.rej
--

After manually applying the rejected hunks, re-run the upgrade command with
these flags:

  --already-resolved=hello.txt
`,
			wantErr: []string{"exit code 2"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempBase := t.TempDir()
			destDir := filepath.Join(tempBase, "dest_dir")
			templateDir := filepath.Join(tempBase, "template_dir")

			// Make tempBase into a valid git repo.
			abctestutil.WriteAll(t, tempBase, abctestutil.WithGitRepoAt("", nil))

			abctestutil.WriteAll(t, destDir, tc.initialDestContents)

			ctx := logging.WithLogger(context.Background(), logging.TestLogger(t))

			abctestutil.WriteAll(t, templateDir, tc.origTemplateDirContents)

			downloader, err := templatesource.ParseSource(ctx, &templatesource.ParseSourceParams{
				CWD:    tempBase,
				Source: templateDir,
			})
			if err != nil {
				t.Fatal(err)
			}

			clk := clock.NewMock()

			renderResult, err := render.Render(ctx, &render.Params{
				Clock:       clk,
				Cwd:         tempBase,
				DestDir:     destDir,
				Downloader:  downloader,
				FS:          &common.RealFS{},
				OutDir:      destDir,
				TempDirBase: tempBase,
			})
			if err != nil {
				t.Fatal(err)
			}

			if tc.localEdits != nil {
				tc.localEdits(t, destDir)
			}

			manifestFullPath := filepath.Join(destDir, renderResult.ManifestPath)

			if err := os.RemoveAll(templateDir); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(templateDir, common.OwnerRWXPerms); err != nil {
				t.Fatal(err)
			}
			abctestutil.WriteAll(t, templateDir, tc.upgradedTemplate)

			cmd := &Command{}

			var stdout bytes.Buffer
			cmd.SetStdout(&stdout)

			err = cmd.Run(ctx, []string{"--verbose", manifestFullPath})
			for _, wantErr := range tc.wantErr {
				if diff := testutil.DiffErrString(err, wantErr); diff != "" {
					t.Error(diff)
				}
			}

			gotExitCode := 0
			var exitCodeErr *common.ExitCodeError
			if errors.As(err, &exitCodeErr) {
				gotExitCode = exitCodeErr.Code
			}
			if gotExitCode != tc.wantExitCode {
				t.Errorf("got exit code %d, want %d", gotExitCode, tc.wantExitCode)
			}

			if err != nil && len(tc.wantErr) == 0 {
				t.Fatal(err)
			}

			gotStdoutCleaned := strings.ReplaceAll(stdout.String(), tempBase, "TEMPDIR")
			if diff := cmp.Diff(gotStdoutCleaned, tc.wantStdout); diff != "" {
				t.Errorf("stdout was not as expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestMissingManifest(t *testing.T) {
	t.Parallel()

	cmd := &Command{}
	ctx := context.Background()
	err := cmd.Run(ctx, []string{"nonexistent_file.txt"})
	if diff := testutil.DiffErrString(err, "found no template manifests to upgrade"); diff != "" {
		t.Fatal(diff)
	}
}

func TestPrompting(t *testing.T) {
	t.Parallel()

	specNoInputs := `
api_version: 'cli.abcxyz.dev/v1beta6'
kind: 'Template'
desc: 'My template'
steps:
- desc: 'Include all files'
  action: 'include'
  params:
    paths: ['.']
`
	specOneInput := `
api_version: 'cli.abcxyz.dev/v1beta6'
kind: 'Template'
desc: 'My template'
inputs:
- name: 'animal'
  desc: 'An animal name'
steps:
- desc: 'Include all files'
  action: 'include'
  params:
    paths: ['.']
- desc: 'Append animal name to out.txt'
  action: 'append'
  params:
    paths: ['.']
    with: '{{.animal}}'
`

	cases := []struct {
		name              string
		dialog            []prompt.DialogStep
		prompt            bool
		inputFileContents string
		origTemplate      map[string]string
		origInputs        map[string]string
		upgradedTemplate  map[string]string

		wantDestContents map[string]string
		wantErr          string
	}{
		{
			name:   "upgraded_template_adds_input",
			prompt: true,
			origTemplate: map[string]string{
				"out.txt":   "",
				"spec.yaml": specNoInputs,
			},
			upgradedTemplate: map[string]string{
				"out.txt":   "",
				"spec.yaml": specOneInput,
			},
			dialog: []prompt.DialogStep{
				{
					WaitForPrompt: "Input name:   animal\nDescription:  An animal name\n\nEnter value: ",
					ThenRespond:   "alligator\n",
				},
				{
					WaitForPrompt: "Upgrade complete with no conflicts\n",
				},
			},
			wantDestContents: map[string]string{
				"out.txt": "alligator\n",
			},
		},
		{
			name: "upgraded_template_adds_input_no_prompt_flag",
			origTemplate: map[string]string{
				"out.txt":   "",
				"spec.yaml": specNoInputs,
			},
			upgradedTemplate: map[string]string{
				"out.txt":   "",
				"spec.yaml": specOneInput,
			},
			wantErr: `when upgrading the manifest at TEMPDIR/dest_dir/.abc/manifest_.._template_dir_1970-01-01T00:00:00Z.lock.yaml:
failed rendering template: missing input(s): animal, you may want to use one of the flags --prompt, --input, or --input-file`,
			wantDestContents: map[string]string{
				"out.txt": "",
			},
		},
		{
			name:   "mix_of_prompting_and_input_file",
			prompt: true,
			origTemplate: map[string]string{
				"out.txt":   "",
				"spec.yaml": specNoInputs,
			},
			upgradedTemplate: map[string]string{
				"out.txt": "",
				"spec.yaml": `
api_version: 'cli.abcxyz.dev/v1beta6'
kind: 'Template'
desc: 'My template'
inputs:
- name: 'animal'
  desc: 'An animal name'
- name: 'color'
  desc: 'A color'
steps:
- desc: 'Include all files'
  action: 'include'
  params:
    paths: ['.']
- desc: 'Append inputs name to out.txt'
  action: 'append'
  params:
    paths: ['.']
    with: '{{.color}} {{.animal}}'
`,
			},
			inputFileContents: `color: 'orange'`,
			dialog: []prompt.DialogStep{
				{
					WaitForPrompt: "Input name:   animal\nDescription:  An animal name\n\nEnter value: ",
					ThenRespond:   "alligator\n",
				},
				{
					WaitForPrompt: "Upgrade complete with no conflicts\n",
				},
			},
			wantDestContents: map[string]string{
				"out.txt": "orange alligator\n",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tempBase := t.TempDir()

			inputFileName := filepath.Join(tempBase, "my_inputs.yaml")
			if tc.inputFileContents != "" {
				if err := os.WriteFile(inputFileName, []byte(tc.inputFileContents), common.OwnerRWPerms); err != nil {
					t.Fatal(err)
				}
			}

			// Make the tempdir into a valid git repo so that the template
			// locations will be treated as canonical.
			abctestutil.WriteAll(t, tempBase, abctestutil.WithGitRepoAt("", nil))

			destDir := filepath.Join(tempBase, "dest_dir")
			templateDir := filepath.Join(tempBase, "template_dir")
			ctx := context.Background()

			abctestutil.WriteAll(t, templateDir, tc.origTemplate)

			downloader, err := templatesource.ParseSource(ctx, &templatesource.ParseSourceParams{
				CWD:    tempBase,
				Source: templateDir,
			})
			if err != nil {
				t.Fatal(err)
			}

			renderResult, err := render.Render(ctx, &render.Params{
				Clock:       clock.NewMock(),
				Cwd:         tempBase,
				DestDir:     destDir,
				Downloader:  downloader,
				FS:          &common.RealFS{},
				OutDir:      destDir,
				TempDirBase: tempBase,
			})
			if err != nil {
				t.Fatal(err)
			}

			cmd := &Command{skipPromptTTYCheck: true}

			manifestFullPath := filepath.Join(destDir, renderResult.ManifestPath)

			abctestutil.WriteAll(t, templateDir, tc.upgradedTemplate)

			args := []string{
				"--verbose", // make it print the "upgrade complete" messages
				fmt.Sprintf("--prompt=%t", tc.prompt),
			}
			if len(tc.inputFileContents) > 0 {
				args = append(args, "--input-file="+inputFileName)
			}
			args = append(args, manifestFullPath)

			err = prompt.DialogTest(ctx, t, tc.dialog, cmd, args)
			wantErr := strings.ReplaceAll(tc.wantErr, "TEMPDIR", tempBase)
			if diff := testutil.DiffErrString(err, wantErr); diff != "" {
				t.Fatal(diff)
			}

			gotDestContents := abctestutil.LoadDir(t, destDir, abctestutil.SkipGlob(".abc/manifest*"))
			if diff := cmp.Diff(gotDestContents, tc.wantDestContents); diff != "" {
				t.Errorf("dest directory contents were not as expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestSummarizeResult(t *testing.T) {
	t.Parallel()

	const location = "my-location"

	cases := []struct {
		name        string
		result      *upgrade.ManifestResult
		wantMessage string
	}{
		{
			name: "success",
			result: &upgrade.ManifestResult{
				Type: upgrade.Success,
			},
			wantMessage: "Upgrade complete with no conflicts",
		},
		{
			name: "already_up_to_date",
			result: &upgrade.ManifestResult{
				Type: upgrade.AlreadyUpToDate,
			},
			wantMessage: "Already up to date with latest template version",
		},
		{
			name: "conflicts",
			result: &upgrade.ManifestResult{
				Type:         upgrade.MergeConflict,
				ManifestPath: "foo/bar/my_manifest.yaml",
				MergeConflicts: []upgrade.ActionTaken{
					{
						Action:               upgrade.EditEditConflict,
						Explanation:          "ignored",
						Path:                 "some/file.txt",
						IncomingTemplatePath: "some/file.txt" + upgrade.SuffixFromNewTemplate,
					},
					{
						Action:               upgrade.DeleteEditConflict,
						Explanation:          "ignored",
						Path:                 "some/other/file.txt",
						IncomingTemplatePath: "some/other/file.txt" + upgrade.SuffixFromNewTemplateLocallyDeleted,
					},
				},
				NonConflicts: []upgrade.ActionTaken{{Path: "should_not_appear.txt", Action: upgrade.WriteNew}},
			},
			wantMessage: `When upgrading manifest my-location/foo/bar/my_manifest.yaml:
` + mergeInstructions + `

List of conflicting files:
--
file: some/file.txt
conflict type: editEditConflict
incoming file: some/file.txt.abcmerge_from_new_template
--
file: some/other/file.txt
conflict type: deleteEditConflict
incoming file: some/other/file.txt.abcmerge_locally_deleted_vs_new_template_version
--

After manually resolving the merge conflict, re-run the upgrade command to
upgrade any other rendered templates in this location that may still need
upgrading.`,
		},
		{
			name: "reversal_conflict",
			result: &upgrade.ManifestResult{
				Type:         upgrade.PatchReversalConflict,
				ManifestPath: "/foo/bar/my_manifest.yaml",
				ReversalConflicts: []*upgrade.ReversalConflict{
					{
						RelPath:       "some/path.txt",
						AbsPath:       "/my/template/output/dir/some/path.txt",
						RejectedHunks: "/my/template/output/dir/some/path.txt.patch.rej",
					},
					{
						RelPath:       "some/other/path.txt",
						AbsPath:       "/my/template/output/dir/some/other/path.txt",
						RejectedHunks: "/my/template/output/dir/some/other/path.txt.patch.rej",
					},
				},
			},
			wantMessage: `When upgrading manifest my-location/foo/bar/my_manifest.yaml:
` + patchReversalInstructions + `

--
your file: /my/template/output/dir/some/path.txt
Rejected hunks for you to apply: /my/template/output/dir/some/path.txt.patch.rej
--
your file: /my/template/output/dir/some/other/path.txt
Rejected hunks for you to apply: /my/template/output/dir/some/other/path.txt.patch.rej
--

After manually applying the rejected hunks, re-run the upgrade command with
these flags:

  --already-resolved=some/path.txt,some/other/path.txt --resume-from=/foo/bar/my_manifest.yaml`,
		},

		{
			name: "reversal_conflict_with_weird_filename_characters_escaped",
			result: &upgrade.ManifestResult{
				Type:         upgrade.PatchReversalConflict,
				ManifestPath: "/foo/bar/my_manifest.yaml",
				ReversalConflicts: []*upgrade.ReversalConflict{
					{
						RelPath:       "a?b!c@d#e$f`g-h^i&j'k*l(m)n[o]p{q}r.txt",
						AbsPath:       "/my/template/output/dir/some/?!@#$%^&*()[]{}.txt",
						RejectedHunks: "/my/template/output/dir/some/?!@#$%^&*()[]{}.txt.patch.rej",
					},
					{
						RelPath:       "a;b'c,d.e?f~g\"h'i.txt",
						AbsPath:       "/my/template/output/dir/some/?!@#$%^&*()[]{}.txt",
						RejectedHunks: "/my/template/output/dir/some/?!@#$%^&*()[]{}.txt.patch.rej",
					},
				},
			},
			wantMessage: `When upgrading manifest my-location/foo/bar/my_manifest.yaml:
` + patchReversalInstructions + `

--
your file: /my/template/output/dir/some/?!@#$%^&*()[]{}.txt
Rejected hunks for you to apply: /my/template/output/dir/some/?!@#$%^&*()[]{}.txt.patch.rej
--
your file: /my/template/output/dir/some/?!@#$%^&*()[]{}.txt
Rejected hunks for you to apply: /my/template/output/dir/some/?!@#$%^&*()[]{}.txt.patch.rej
--

After manually applying the rejected hunks, re-run the upgrade command with
these flags:

  --already-resolved='a?b!c@d#e$f` + "`" + `g-h^i&j'"'"'k*l(m)n[o]p{q}r.txt','a;b'"'"'c,d.e?f~g"h'"'"'i.txt' --resume-from=/foo/bar/my_manifest.yaml`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			message := summarizeResult(tc.result, location)
			if diff := cmp.Diff(message, tc.wantMessage); diff != "" {
				t.Errorf("message was not as expected (-got,+want): %s", diff)
			}
		})
	}
}
