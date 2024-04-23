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

// Package render implements the template rendering related subcommands.
package upgrade

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/benbjohnson/clock"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/upgrade"
	"github.com/abcxyz/pkg/cli"
)

// Command implements cli.Command for template upgrades.
type Command struct {
	cli.BaseCommand
	flags Flags

	// Used in prompt tests to bypass "is the input a terminal" check.
	skipPromptTTYCheck bool
}

// Desc implements cli.Command.
func (c *Command) Desc() string {
	return "apply a new template version to an already-rendered template output directory"
}

// Help implements cli.Command.
func (c *Command) Help() string {
	return `
Usage: {{ COMMAND }} [options] <manifest>

The {{ COMMAND }} command upgrades an already-rendered template output to use
the latest version of a template.

The "<manifest>" is the path to the manifest_*.lock.yaml file that was created when the
template was originally rendered, usually found in the .abc subdirectory.
`
}

// Hidden implements cli.Command.
func (c *Command) Hidden() bool {
	// TODO(upgrade): unhide the upgrade command when it's ready.
	return true
}

func (c *Command) Flags() *cli.FlagSet {
	set := c.NewFlagSet()
	c.flags.Register(set)
	return set
}

const (
	mergeInstructions = `
Some manual conflict resolution is required because of a conflict between your
local edits and the new version of the template. Please look at all files ending
in .abcmerge_* and either edit, delete, or rename them to reflect your decision.

Background on conflict types:

 - editEditConflict: you made some local edits to this file that was installed
   by the template, which conflicts with the new version of the template which
   wants to edit the file. Both versions of the file are left in your output
   directory, named "yourfile.abcmerge_locally_edited" and
   "yourfile.abcmerge_from_new_template". Please resolve the conflict by either
   (1) renaming one of the files to "yourfile" and deleting the other, or (2)
   merging the two files into "yourfile".

 - editDeleteConflict: you made an edit to this file that was installed by the
   template, which conflicts with the new version of the template, which wants
   to delete this file. Your version has been renamed to
   "yourfile.abcmerge_template_wants_to_delete". Please resolve the conflict by
   renaming it back to "yourfile" or deleting it.

 - deleteEditConflict: you deleted this file that was installed by the template,
   which conflicts with the new version of the template which wants to edit it.
   The new version from the template is named
   "yourfile.abcmerge_locally_deleted_vs_new_template_version". Please resolve
   the conflict by renaming it to "yourfile" or deleting it.

 - addAddConflict: you added a file which was not originally part of the
   template, which conflicts with the new version of the template, which wants
   to create a file of the same name. Your version of the file has been renamed
   to "yourfile.abcmerge_locally_added", and the version of the template is
   named "yourfile.abcmerge_from_new_template". Please resolve the conflict by
   (1) renaming one of these files to "yourfile and deleting the other, or (2)
   merging the two files into "yourfile".`

	patchReversalInstructions = `
There was a merge conflict when trying to undo changes to file(s) that were
modified in-place by a previous version of the template.

Background: the upgrade algorithm has a special case for files that were
modified in place by a previous version of the template (aka "include from
destination"). When the file was previously modified in place, a patch was saved
to undo that modification, so a future template version could start fresh and
redo the modification in place based on new template logic. Just now, this patch
was applied to the file, but the patch didn't apply cleanly. This happens when
the file was modified since the previous version of this template was installed;
that could happen because somebody edited the file, or that same file was
modified in place by a different template.

To resolve this conflict, please manually apply the rejected hunks in the given
.rej file, for each entry in the following list:

`
)

func (c *Command) Run(ctx context.Context, args []string) error {
	if err := c.Flags().Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	fi, err := os.Stat(c.flags.Location)
	if err != nil {
		return err //nolint:wrapcheck
	}
	if fi.IsDir() {
		return c.upgradeAll(ctx, c.flags.Location)
	}
	return c.upgradeOne(ctx, c.flags.Location)
}

func (c *Command) upgradeAll(ctx context.Context, startDir string) error {
	numUpgraded := 0
	for {
		var manifestToUpgrade string
		err := filepath.WalkDir(startDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			baseName := filepath.Base(path)
			ext := filepath.Ext(path)
			parentDir := filepath.Base(filepath.Dir(path))
			if strings.HasPrefix(baseName, "manifest") && ext == ".yaml" && parentDir == common.ABCInternalDir {
				manifestToUpgrade = path
				return fs.SkipAll
			}
			return nil
		})
		if err != nil {
			return err
		}

		if manifestToUpgrade == "" {
			break
		}
		if err := c.upgradeOne(ctx, manifestToUpgrade); err != nil {
			return err
		}
		numUpgraded++
	}

	if numUpgraded == 0 {
		return fmt.Errorf("found no template manifests to upgrade")
	}
	return nil
}

func (c *Command) upgradeOne(ctx context.Context, manifestPath string) error {
	absManifestPath, err := filepath.Abs(manifestPath)
	if err != nil {
		return fmt.Errorf("filepath.Abs(%q): %w", manifestPath, err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("os.Getwd(): %w", err)
	}

	r, err := upgrade.Upgrade(ctx, &upgrade.Params{
		Clock:                clock.New(),
		CWD:                  cwd,
		DebugStepDiffs:       c.flags.DebugStepDiffs,
		DebugScratchContents: c.flags.DebugScratchContents,
		FS:                   fs,
		GitProtocol:          c.flags.GitProtocol,
		InputFiles:           c.flags.InputFiles,
		Inputs:               c.flags.Inputs,
		KeepTempDirs:         c.flags.KeepTempDirs,
		ManifestPath:         absManifestPath,
		Prompt:               c.flags.Prompt,
		Prompter:             c,
		SkipInputValidation:  c.flags.SkipInputValidation,
		SkipPromptTTYCheck:   c.skipPromptTTYCheck,
		Stdout:               c.Stdout(),
	})
	if err != nil {
		return err //nolint:wrapcheck
	}

	exitCode := processResult(c.Stdout(), r)
	if exitCode == 0 {
		return nil
	}
	return &common.ExitCodeError{Code: exitCode}
}

func processResult(stdout io.Writer, r *upgrade.Result) (exitCode int) {
	switch r.Type {
	case upgrade.AlreadyUpToDate:
		fmt.Fprintf(stdout, "Already up to date with latest template version\n")
		return 0
	case upgrade.Success:
		fmt.Fprintf(stdout, "Upgrade complete with no conflicts\n")
		return 0
	case upgrade.MergeConflict:
		// TODO(upgrade):
		//  - suggest diff / meld / vim commands?
		fmt.Fprint(stdout, mergeInstructions+"\n\n--\n")
		for _, cf := range r.Conflicts {
			fmt.Fprintf(stdout, "file: %s\n", cf.Path)
			fmt.Fprintf(stdout, "conflict type: %s\n", cf.Action)
			if cf.OursPath != "" {
				fmt.Fprintf(stdout, "our file was renamed to: %s\n", cf.OursPath)
			}
			if cf.IncomingTemplatePath != "" {
				fmt.Fprintf(stdout, "incoming file: %s\n", cf.IncomingTemplatePath)
			}
			fmt.Fprintf(stdout, "--\n")
		}
		return 2
	case upgrade.PatchReversalConflict:
		fmt.Fprint(stdout, patchReversalInstructions)
		for _, rc := range r.ReversalConflicts {
			fmt.Fprintf(stdout, "File: %s", rc.AbsPath)
			fmt.Fprintf(stdout, "Rejected hunks for you to apply: %s", rc.RejectedHunks)
		}
		return 3
	}
	panic("unreachable") // TODO verify exhaustiveness
}
