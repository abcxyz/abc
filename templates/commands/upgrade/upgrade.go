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
	"path/filepath"
	"strings"

	"al.essio.dev/pkg/shellescape"
	"github.com/benbjohnson/clock"
	"github.com/posener/complete/v2"
	"github.com/posener/complete/v2/predict"

	"github.com/abcxyz/abc-updater/pkg/metrics"
	"github.com/abcxyz/abc/internal/metricswrap"
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

func (c *Command) Flags() *cli.FlagSet {
	set := c.NewFlagSet()
	c.flags.Register(set)
	return set
}

func (c *Command) PredictArgs() complete.Predictor {
	return predict.Files("") // "Files" will predict both files and dirs
}

const (
	mergeInstructions = `
Some manual conflict resolution is required because of a conflict between your
local edits and the new version of the template. Please look at all files ending
in .abcmerge_* and either edit, delete, or rename them to reflect your decision.

Background on conflict types:

 - editEditConflict: you made some local edits to this file that was installed
   by the template, which conflicts with the new version of the template which
   wants to edit the file. Your locally edited file is unchanged, and the
   incoming file from the template has the additional extension
   ".abcmerge_from_new_template". Please resolve the conflict by either
     - selectively incorporating some of the changes from the
       .abcmerge_from_new_template file into your local file
	 - rejecting this incoming change by removing the
	   .abcmerge_from_new_template file
	 - overwriting your locally edit file with the .abcmerge_from_new_template
	   incoming file.

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
.rej file, for each entry in the following list:`
)

func (c *Command) Run(ctx context.Context, args []string) error {
	mClient := metrics.FromContext(ctx)
	cleanup := metricswrap.WriteMetric(ctx, mClient, "command_upgrade", 1)
	defer cleanup()

	if err := c.Flags().Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	absLocation, err := filepath.Abs(c.flags.Location)
	if err != nil {
		return fmt.Errorf("filepath.Abs(%q): %w", c.flags.Location, err)
	}

	result := upgrade.UpgradeAll(ctx, &upgrade.Params{
		AcceptDefaults:       c.flags.AcceptDefaults,
		AlreadyResolved:      c.flags.AlreadyResolved,
		Clock:                clock.New(),
		DebugStepDiffs:       c.flags.DebugStepDiffs,
		DebugScratchContents: c.flags.DebugScratchContents,
		ContinueIfCurrent:    c.flags.ContinueIfCurrent,
		FS:                   &common.RealFS{},
		GitProtocol:          c.flags.GitProtocol,
		InputFiles:           c.flags.InputFiles,
		InputsFromFlags:      c.flags.Inputs,
		KeepTempDirs:         c.flags.KeepTempDirs,
		Location:             absLocation,
		ManifestFilter:       c.flags.ManifestFilter,
		Prompt:               c.flags.Prompt,
		Prompter:             c,
		SkipInputValidation:  c.flags.SkipInputValidation,
		SkipPromptTTYCheck:   c.skipPromptTTYCheck,
		Stdout:               c.Stdout(),
		TemplateLocation:     c.flags.TemplateLocation,
		UpgradeChannel:       c.flags.UpgradeChannel,
		Version:              c.flags.Version,
	})
	if result.Err != nil {
		if result.ErrManifestPath != "" {
			return fmt.Errorf("when upgrading the manifest at %s:\n%w",
				result.ErrManifestPath, result.Err)
		}
		return result.Err
	}

	for i, oneManifestResult := range result.Results {
		isLast := i == len(result.Results)-1
		if isPrintable(c.flags.Verbose, isLast, oneManifestResult.Type) {
			fmt.Fprintln(c.Stdout(), summarizeResult(oneManifestResult, absLocation))
		}
	}

	exitCode := exitCode(result.Overall)
	if exitCode != 0 {
		return &common.ExitCodeError{Code: exitCode}
	}

	return nil
}

func isPrintable(verboseFlag, isLast bool, rt upgrade.ResultType) bool {
	if verboseFlag {
		return true
	}
	if !isLast {
		// all results before the last are successful, because we abort on
		// failure. Therefore we only print them if we're in verbose mode.
		return false
	}
	return rt.RequiresUserAttention()
}

func exitCode(overallResult upgrade.ResultType) int {
	switch overallResult {
	case upgrade.AlreadyUpToDate, upgrade.Success:
		return 0
	case upgrade.MergeConflict:
		return 1
	case upgrade.PatchReversalConflict:
		return 2
	}
	panic("unreachable") // the go lint exhaustive check prevents this
}

func summarizeResult(r *upgrade.ManifestResult, location string) string {
	// You might wonder: why are the merge instructions printed here, *inside*
	// the loop that loops over manifests? Won't that result in a large block of
	// instructions being printed multiple times? No, because there's at most
	// one failing upgrade, because we stop after a single failure (merge
	// conflict or patch reversal conflict).
	manifestPath := filepath.Join(location, r.ManifestPath)
	switch r.Type {
	case upgrade.AlreadyUpToDate:
		// TODO(upgrade): show version
		return "Already up to date with latest template version"
	case upgrade.Success:
		// TODO(upgrade): show version upgraded to
		return "Upgrade complete with no conflicts"
	case upgrade.MergeConflict:
		// TODO(upgrade):
		//  - suggest diff / meld / vim commands?
		var out strings.Builder
		fmt.Fprintf(&out, "When upgrading manifest %s:\n", manifestPath)

		fmt.Fprintf(&out, mergeInstructions+"\n\nList of conflicting files:\n--")
		for _, cf := range r.MergeConflicts {
			fmt.Fprintf(&out, "\nfile: %s\n", cf.Path)
			fmt.Fprintf(&out, "conflict type: %s\n", cf.Action)
			if cf.OursPath != "" {
				fmt.Fprintf(&out, "your file was renamed to: %s\n", cf.OursPath)
			}
			if cf.IncomingTemplatePath != "" {
				fmt.Fprintf(&out, "incoming file: %s\n", cf.IncomingTemplatePath)
			}
			fmt.Fprintf(&out, "--")
		}
		fmt.Fprintf(&out, `

After manually resolving the merge conflict, re-run the upgrade command to
upgrade any other rendered templates in this location that may still need
upgrading.`)

		return out.String()
	case upgrade.PatchReversalConflict:
		var out strings.Builder
		fmt.Fprintf(&out, "When upgrading manifest %s:\n", manifestPath)
		fmt.Fprint(&out, patchReversalInstructions+"\n\n--")
		relPaths := make([]string, 0, len(r.ReversalConflicts))
		for _, rc := range r.ReversalConflicts {
			fmt.Fprintf(&out, "\nyour file: %s\n", rc.AbsPath)
			fmt.Fprintf(&out, "Rejected hunks for you to apply: %s\n", rc.RejectedHunks)
			fmt.Fprintf(&out, "--")
			relPaths = append(relPaths, shellescape.Quote(rc.RelPath))
		}

		// In the case where the user specified just a single manifest to
		// upgrade, like "abc upgrade foo/.abc/manifest.yaml, then we'll leave
		// out the "--resume-from=" flag. This would be confusing and
		// unnecessary, since you don't need to specify a template to resume
		// from when you're just upgrading one template.
		resumeFrom := ""
		if r.ManifestPath != "." {
			resumeFrom = fmt.Sprintf(" --resume-from=%s", r.ManifestPath)
		}
		fmt.Fprintf(&out, `

After manually applying the rejected hunks, re-run the upgrade command with
these flags:

  --already-resolved=%s%s`,
			strings.Join(relPaths, ","), resumeFrom)
		return out.String()
	}
	panic("unreachable") // the go lint exhaustive check prevents this
}
