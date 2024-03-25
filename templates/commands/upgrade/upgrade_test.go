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

package upgrade

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/render"
	"github.com/abcxyz/abc/templates/common/templatesource"
	abctestutil "github.com/abcxyz/abc/templates/testutil"
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

		wantStdout string
		wantErr    []string
	}{
		// TODO tests:
		//  try interactive inputs
		//  try input files
		//  errors?
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
				overwrite(tb, installedDir, "greet.txt", "goodbye\n")
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
				overwrite(tb, installedDir, "greet.txt", "hello, mars\n")
				overwrite(tb, installedDir, "color.txt", "red\n")
			},
			wantStdout: mergeInstructions + `

--
file: color.txt
conflict type: addAddConflict
our file was renamed to: color.txt.abcmerge_locally_added
incoming file: color.txt.abcmerge_from_new_template
--
file: greet.txt
conflict type: editEditConflict
our file was renamed to: greet.txt.abcmerge_locally_edited
incoming file: greet.txt.abcmerge_from_new_template
--
`,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempBase := t.TempDir()
			destDir := filepath.Join(tempBase, "dest_dir")
			manifestDir := filepath.Join(destDir, common.ABCInternalDir)
			templateDir := filepath.Join(tempBase, "template_dir")

			// Make tempBase into a valid git repo.
			abctestutil.WriteAll(t, tempBase, abctestutil.WithGitRepoAt("", nil))

			ctx := context.Background()

			abctestutil.WriteAll(t, templateDir, tc.origTemplateDirContents)

			downloader, err := templatesource.ParseSource(ctx, &templatesource.ParseSourceParams{
				CWD:    tempBase,
				Source: templateDir,
			})
			if err != nil {
				t.Fatal(err)
			}

			clk := clock.NewMock()

			if err := render.Render(ctx, &render.Params{
				Clock:       clk,
				Cwd:         tempBase,
				DestDir:     destDir,
				Downloader:  downloader,
				FS:          &common.RealFS{},
				Manifest:    true,
				OutDir:      destDir,
				TempDirBase: tempBase,
			}); err != nil {
				t.Fatal(err)
			}

			if tc.localEdits != nil {
				tc.localEdits(t, destDir)
			}

			manifestBaseName := findManifest(t, manifestDir)
			manifestFullPath := filepath.Join(manifestDir, manifestBaseName)

			if err := os.RemoveAll(templateDir); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(templateDir, common.OwnerRWXPerms); err != nil {
				t.Fatal(err)
			}
			abctestutil.WriteAll(t, templateDir, tc.upgradedTemplate)

			cmd := &Command{}

			var stdout, stderr bytes.Buffer
			cmd.SetStdout(&stdout)
			cmd.SetStderr(&stderr)

			err = cmd.Run(ctx, []string{manifestFullPath})
			for _, wantErr := range tc.wantErr {
				if diff := testutil.DiffErrString(err, wantErr); diff != "" {
					t.Error(diff)
				}
			}
			if err != nil && len(tc.wantErr) == 0 {
				t.Fatal(err)
			}

			t.Logf("stdout was:\n%s", stdout.String())
			if diff := cmp.Diff(stdout.String(), tc.wantStdout); diff != "" {
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
	if diff := testutil.DiffErrString(err, "failed to open manifest file"); diff != "" {
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
		dialog            []abctestutil.DialogStep
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
			dialog: []abctestutil.DialogStep{
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
			wantErr: "missing input(s): animal",
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
			dialog: []abctestutil.DialogStep{
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
		tc := tc
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
			manifestDir := filepath.Join(destDir, common.ABCInternalDir)
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

			if err := render.Render(ctx, &render.Params{
				Clock:       clock.New(),
				Cwd:         tempBase,
				DestDir:     destDir,
				Downloader:  downloader,
				FS:          &common.RealFS{},
				Manifest:    true,
				OutDir:      destDir,
				TempDirBase: tempBase,
			}); err != nil {
				t.Fatal(err)
			}

			cmd := &Command{skipPromptTTYCheck: true}

			stdinReader, stdinWriter := io.Pipe()
			stdoutReader, stdoutWriter := io.Pipe()
			_, stderrWriter := io.Pipe()

			cmd.SetStdin(stdinReader)
			cmd.SetStdout(stdoutWriter)
			cmd.SetStderr(stderrWriter)

			manifestBaseName := findManifest(t, manifestDir)
			manifestFullPath := filepath.Join(manifestDir, manifestBaseName)

			abctestutil.WriteAll(t, templateDir, tc.upgradedTemplate)

			errCh := make(chan error)
			go func() {
				defer close(errCh)
				args := []string{fmt.Sprintf("--prompt=%t", tc.prompt)}
				if len(tc.inputFileContents) > 0 {
					args = append(args, "--input-file="+inputFileName)
				}
				args = append(args, manifestFullPath)
				errCh <- cmd.Run(ctx, args)
			}()

			// TODO factor out dialog test function
			go func() {
				for _, ds := range tc.dialog {
					abctestutil.ReadWithTimeout(t, stdoutReader, ds.WaitForPrompt)
					abctestutil.WriteWithTimeout(t, stdinWriter, ds.ThenRespond)
				}
			}()

			select {
			case err := <-errCh:
				if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
					t.Fatal(diff)
				}
			case <-time.After(time.Second):
				t.Errorf("timed out waiting for background goroutine to finish")
				buf := make([]byte, 1_000_000)
				length := runtime.Stack(buf, true)
				t.Logf("Stack trace:\n%s", buf[:length])
				t.FailNow()
			}

			gotDestContents := abctestutil.LoadDir(t, destDir, abctestutil.SkipGlob(".abc/manifest*"))
			if diff := cmp.Diff(gotDestContents, tc.wantDestContents); diff != "" {
				t.Errorf("dest directory contents were not as expected (-got,+want): %s", diff)
			}
		})
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "nonexistent_manifest",
			args:    []string{"nonexistent.yaml"},
			wantErr: "failed to open manifest file",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			err := (&Command{}).Run(ctx, tc.args)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func findManifest(tb testing.TB, dir string) string {
	tb.Helper()

	joined := filepath.Join(dir, "manifest*.yaml")
	matches, err := filepath.Glob(joined)
	if err != nil {
		tb.Fatalf("filepath.Glob(%q): %v", joined, err)
	}

	if len(matches) == 0 {
		tb.Fatalf("no manifest was found in %q", dir)
	}
	if len(matches) > 1 {
		tb.Fatalf("multiple manifests were found in %q: %s", dir, strings.Join(matches, ", "))
	}

	return filepath.Base(matches[0])
}

func overwrite(tb testing.TB, dir, baseName, contents string) {
	tb.Helper()

	filename := filepath.Join(dir, baseName)
	if err := os.WriteFile(filename, []byte(contents), common.OwnerRWPerms); err != nil {
		tb.Fatal(err)
	}
}
