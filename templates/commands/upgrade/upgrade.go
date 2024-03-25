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
	"os"
	"path/filepath"

	"github.com/benbjohnson/clock"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/upgrade"
	"github.com/abcxyz/pkg/cli"
)

// Command implements cli.Command for template upgrades.
type Command struct {
	cli.BaseCommand
	flags Flags

	// Used in prompt tests to bypass "is the input a terminal" check..
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

const mergeInstructions = `
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

func (c *Command) Run(ctx context.Context, args []string) error {
	if err := c.Flags().Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	fs := &common.RealFS{}

	absManifestPath, err := filepath.Abs(c.flags.Manifest)
	if err != nil {
		return fmt.Errorf("filepath.Abs(%q): %w", c.flags.Manifest, err)
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

	if r.AlreadyUpToDate {
		fmt.Fprintf(c.Stdout(), "Already up to date with latest template version\n")
		return nil
	}

	if len(r.Conflicts) == 0 {
		fmt.Fprintf(c.Stdout(), "Upgrade complete with no conflicts\n")
		return nil
	}

	// TODO(upgrade):
	//  - suggest diff / meld / vim commands?
	fmt.Fprint(c.Stdout(), mergeInstructions+"\n\n--\n")
	for _, cf := range r.Conflicts {
		fmt.Fprintf(c.Stdout(), "file: %s\n", cf.Path)
		fmt.Fprintf(c.Stdout(), "conflict type: %s\n", cf.Action)
		if cf.OursPath != "" {
			fmt.Fprintf(c.Stdout(), "our file was renamed to: %s\n", cf.OursPath)
		}
		if cf.IncomingTemplatePath != "" {
			fmt.Fprintf(c.Stdout(), "incoming file: %s\n", cf.IncomingTemplatePath)
		}
		fmt.Fprintf(c.Stdout(), "--\n")
	}

	return nil
}
