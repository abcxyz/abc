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
package render

// This file implements the "templates render" subcommand for installing a template.

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/input"
	"github.com/abcxyz/abc/templates/common/specutil"
	"github.com/abcxyz/abc/templates/common/templatesource"
	"github.com/abcxyz/abc/templates/model"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta3"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
)

const (
	// These will be used as part of the names of the temporary directories to
	// make them identifiable.
	templateDirNamePart = "template-copy-"
	scratchDirNamePart  = "scratch-"
	debugDirNamePart    = "debug-"
)

type Command struct {
	cli.BaseCommand
	flags RenderFlags

	testFS common.FS
}

// Desc implements cli.Command.
func (c *Command) Desc() string {
	return "instantiate a template to setup a new app or add config files"
}

// Help implements cli.Command.
func (c *Command) Help() string {
	return `
Usage: {{ COMMAND }} [options] <source>

The {{ COMMAND }} command renders the given template.

The "<source>" is the location of the template to be rendered. This may have a
few forms:

  - A remote GitHub or GitLab repo with either a version @tag or with the magic
    version "@latest". Examples:
    - github.com/abcxyz/abc/t/rest_server@latest
    - github.com/abcxyz/abc/t/rest_server@v0.3.1
  - A local directory, like /home/me/mydir
  - (Deprecated) A go-getter-style location, with or without ?ref=foo. Examples:
    - github.com/abcxyz/abc.git//t/react_template?ref=latest
	- github.com/abcxyz/abc.git//t/react_template
`
}

// Flags implements cli.Command.
func (c *Command) Flags() *cli.FlagSet {
	set := c.NewFlagSet()
	c.flags.Register(set)
	return set
}

func (c *Command) Run(ctx context.Context, args []string) error {
	if err := c.Flags().Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	fSys := c.testFS // allow filesystem interaction to be faked for testing
	if fSys == nil {
		fSys = &common.RealFS{}
	}

	if err := destOK(fSys, c.flags.Dest); err != nil {
		return err
	}

	wd, err := c.WorkingDir()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home dir: %w", err)
	}
	backupDir := filepath.Join(
		homeDir,
		".abc",
		"backups",
		fmt.Sprint(time.Now().Unix()))

	return c.realRun(ctx, &runParams{
		backupDir: backupDir,
		clock:     clock.New(),
		cwd:       wd,
		fs:        fSys,
		stdout:    c.Stdout(),
	})
}

type runParams struct {
	backupDir string
	clock     clock.Clock
	cwd       string
	fs        common.FS
	stdout    io.Writer

	// The directory under which temp directories will be created. The default
	// if this is empty is to use the OS temp directory.
	tempDirBase string
}

// realRun is for testability; it's Run() with fakeable interfaces.
func (c *Command) realRun(ctx context.Context, rp *runParams) (outErr error) {
	var tempDirs []string
	defer func() {
		// This does not remove debugDir if there is one.
		err := c.maybeRemoveTempDirs(ctx, rp.fs, tempDirs...)
		outErr = errors.Join(outErr, err)
	}()

	dlMeta, templateDir, err := templatesource.Download(ctx, &templatesource.DownloadParams{
		FS:          rp.fs,
		TempDirBase: rp.tempDirBase,
		Source:      c.flags.Source,
		Dest:        c.flags.Dest,
		GitProtocol: c.flags.GitProtocol,
	})
	if templateDir != "" { // templateDir might be set even if there's an error
		tempDirs = append(tempDirs, templateDir)
	}
	if err != nil {
		return err //nolint:wrapcheck
	}

	spec, err := specutil.Load(ctx, rp.fs, templateDir, c.flags.Source)
	if err != nil {
		return err //nolint:wrapcheck
	}

	resolvedInputs, err := input.Resolve(ctx, &input.ResolveParams{
		FS:                  rp.fs,
		InputFiles:          c.flags.InputFiles,
		Inputs:              c.flags.Inputs,
		Prompt:              c.flags.Prompt,
		Prompter:            c,
		SkipInputValidation: c.flags.SkipInputValidation,
		Stdin:               c.Stdin(),
		Spec:                spec,
	})
	if err != nil {
		return err //nolint:wrapcheck
	}

	scratchDir, err := rp.fs.MkdirTemp(rp.tempDirBase, scratchDirNamePart)
	if err != nil {
		return fmt.Errorf("failed to create temp directory for scratch directory: %w", err)
	}
	tempDirs = append(tempDirs, scratchDir)
	logger := logging.FromContext(ctx)
	logger.DebugContext(ctx, "created temporary scratch directory", "path", scratchDir)

	sp := &stepParams{
		flags:           &c.flags,
		fs:              rp.fs,
		scope:           common.NewScope(resolvedInputs),
		scratchDir:      scratchDir,
		stdout:          rp.stdout,
		templateDir:     templateDir,
		ignorePatterns:  spec.Ignore,
		upgradeFeatures: spec.UpgradeFeatures,
	}

	var debugDir string
	if c.flags.DebugStepDiffs {
		debugDir, err = rp.fs.MkdirTemp(rp.tempDirBase, debugDirNamePart)
		if err != nil {
			return fmt.Errorf("failed to create temp directory for debug directory: %w", err)
		}
		sp.debugDir = debugDir

		argsList := [][]string{
			// Make debug dir a git repository with a detached work tree in scratch
			// dir, meaning it will track the file changes in scratch dir without
			// affecting the scratch dir.
			{"git", "--git-dir", debugDir, "--work-tree", sp.scratchDir, "init"},

			// Set git user name and email, required for ubuntu and windows os.
			{"git", "--git-dir", debugDir, "config", "user.name", "abc cli"},
			{"git", "--git-dir", debugDir, "config", "user.email", "abc@abcxyz.com"},
		}
		if err := runCmds(ctx, argsList); err != nil {
			return err
		}
	}

	if err := executeSteps(ctx, spec.Steps, sp); err != nil {
		return err
	}

	includedFromDest := sliceToSet(sp.includedFromDest)
	var outputHashes map[string][]byte

	// Commit the contents of the scratch directory to the output directory. We
	// first do a dry-run to check that the copy is likely to succeed, so we
	// don't leave a half-done mess in the user's dest directory.
	for _, dryRun := range []bool{true, false} {
		if outputHashes, err = c.commit(ctx, dryRun, rp, scratchDir, includedFromDest); err != nil {
			return err
		}

		if c.flags.Manifest {
			if err := writeManifest(ctx, &writeManifestParams{
				clock:        rp.clock,
				cwd:          rp.cwd,
				dlMeta:       dlMeta,
				destDir:      c.flags.Dest,
				dryRun:       dryRun,
				fs:           rp.fs,
				inputs:       resolvedInputs,
				outputHashes: outputHashes,
				src:          c.flags.Source,
				templateDir:  templateDir,
			}); err != nil {
				return err
			}
		}
	}

	if sp.flags.DebugStepDiffs {
		// Use default log level.
		logger.WarnContext(
			ctx,
			fmt.Sprintf(
				"Please navigate to '%s' or use 'git --git-dir=%s log' to see commits/diffs for each step",
				debugDir, debugDir,
			),
		)
	}

	return nil
}

// commit copies the contents of scratchDir to rp.Dest. If dryRun==true, then
// files are read but nothing is written to the destination. includedFromDest is
// a set of files that were the subject of an "include" action that set "from:
// destination".
//
// The return value is a map containing a SHA256 hash of each file in
// scratchDir. The keys are paths relative to scratchDir, using forward slashes
// regardless of the OS.
func (c *Command) commit(ctx context.Context, dryRun bool, rp *runParams, scratchDir string, includedFromDest map[string]struct{}) (map[string][]byte, error) {
	logger := logging.FromContext(ctx).With("logger", "commit")

	visitor := func(relPath string, _ fs.DirEntry) (common.CopyHint, error) {
		_, ok := includedFromDest[relPath]
		return common.CopyHint{
			BackupIfExists: true,

			// Special case: files that were "include"d from the
			// *destination* directory (rather than the template directory),
			// are always allowed to be overwritten. For example, if we grab
			// file_to_modify.txt from the --dest dir, then we always allow
			// ourself to write back to that file, even when
			// --force-overwrite=false. When the template uses this feature,
			// we know that the intent is to modify the files in place.
			Overwrite: ok || c.flags.ForceOverwrite,
		}, nil
	}

	// We only want to call MkdirTemp once, and use the resulting backup
	// directory for every step in this rendering operation.
	var backupDir string
	var err error
	backupDirMaker := func(rfs common.FS) (string, error) {
		if backupDir != "" {
			return backupDir, nil
		}
		if err := rfs.MkdirAll(rp.backupDir, common.OwnerRWXPerms); err != nil {
			return "", err //nolint:wrapcheck // err already contains path, and it will be wrapped later
		}
		backupDir, err = rfs.MkdirTemp(rp.backupDir, "")
		logger.DebugContext(ctx, "created backup directory", "path", backupDir)
		return backupDir, err //nolint:wrapcheck // err already contains path, and it will be wrapped later
	}

	params := &common.CopyParams{
		BackupDirMaker: backupDirMaker,
		DryRun:         dryRun,
		DstRoot:        c.flags.Dest,
		Hasher:         sha256.New,
		OutHashes:      map[string][]byte{},
		SrcRoot:        scratchDir,
		RFS:            rp.fs,
		Visitor:        visitor,
	}
	if err := common.CopyRecursive(ctx, nil, params); err != nil {
		return nil, fmt.Errorf("failed writing to --dest directory: %w", err)
	}
	if dryRun {
		logger.DebugContext(ctx, "template render (dry run) succeeded")
	} else {
		logger.InfoContext(ctx, "template render succeeded")
	}
	return params.OutHashes, nil
}

func sliceToSet[T comparable](vals []T) map[T]struct{} {
	out := make(map[T]struct{}, len(vals))
	for _, v := range vals {
		out[v] = struct{}{}
	}
	return out
}

func executeSteps(ctx context.Context, steps []*spec.Step, sp *stepParams) error {
	logger := logging.FromContext(ctx).With("logger", "executeSteps")

	for i, step := range steps {
		if err := executeOneStep(ctx, i, step, sp); err != nil {
			return err
		}

		if sp.flags.DebugStepDiffs {
			// Commit the diffs after each step.
			m := fmt.Sprintf("action %s at line %d", step.Action.Val, step.Pos.Line)
			argsList := [][]string{
				{"git", "--git-dir", sp.debugDir, "add", "-A"},
				{"git", "--git-dir", sp.debugDir, "commit", "-a", "-m", m, "--allow-empty"},
			}
			if err := runCmds(ctx, argsList); err != nil {
				return err
			}
		}

		logger.DebugContext(ctx, "completed template action", "action", step.Action.Val)
		if sp.flags.DebugScratchContents {
			contents, err := scratchContents(ctx, i, step, sp)
			if err != nil {
				return err
			}
			logger.WarnContext(ctx, contents)
		}
	}
	return nil
}

func scratchContents(ctx context.Context, stepIdx int, step *spec.Step, sp *stepParams) (string, error) {
	sb := &strings.Builder{}
	fmt.Fprintf(sb, "Scratch dir contents after step %d (starting from 0), which is action type %q, defined at spec file line %d:\n",
		stepIdx, step.Action.Val, step.Action.Pos.Line)
	err := filepath.WalkDir(sp.scratchDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err // some filesystem error happened
		}
		if d.IsDir() {
			// it's not possible to have an empty directory in the
			// scratch directory, and directory names will be shown as
			// part of filenames, so we don't show plain directory
			// names. Like in Git.
			return nil
		}
		rel, err := filepath.Rel(sp.scratchDir, path)
		if err != nil {
			return fmt.Errorf("filepath.Rel(): %w", err)
		}
		fmt.Fprintf(sb, " %s", rel)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("error crawling scratch directory: %w", err)
	}
	return sb.String(), nil
}

type stepParams struct {
	flags *RenderFlags
	fs    common.FS

	// Scope contains all variable names that are in scope. This includes
	// user-provided inputs, as well as any programmatically created variables
	// like for_each keys.
	scope *common.Scope

	scratchDir  string
	stdout      io.Writer
	templateDir string

	// Temporary directory to hold debug information when debug-step-diffs is
	// enabled, if not enabled, it will be an empty string.
	debugDir string

	// Files and directories included in spec that match ignorePatterns will be
	// ignored while being copied to destination directory.
	ignorePatterns []model.String

	// Mutable fields that are updated by action* functions go below this line.

	// includedFromDest is a list of every file (no directories) that was copied
	// from the destination directory into the scratch directory. We want to
	// track these because they are treated specially in the final phase of
	// rendering. When we commit the template output from the scratch directory
	// into the destination directory, these paths are always allowed to be
	// overwritten. For other files not in this list, it's an error to try to
	// write to an existing file. This whole scheme supports the feature of
	// modifying files that already exist in the destination.
	//
	// These are paths relative to the --dest directory (which is the same thing
	// as being relative to the scratch directory, the paths within these dirs
	// are the same).
	includedFromDest []string

	// TODO: doc
	upgradeFeatures *spec.UpgradeFeatures
}

// WithScope returns a copy of this stepParams with a new inner variable scope
// containing some extra variable bindings.
func (s *stepParams) WithScope(vars map[string]string) *stepParams {
	out := *s
	out.scope = s.scope.With(vars)
	return &out
}

func executeOneStep(ctx context.Context, stepIdx int, step *spec.Step, sp *stepParams) error {
	logger := logging.FromContext(ctx).With("logger", "executeOneStep")

	if step.If.Val != "" {
		var celResult bool
		if err := common.CelCompileAndEval(ctx, sp.scope, step.If, &celResult); err != nil {
			return fmt.Errorf(`"if" expression "%s" failed at step index %d action %q: %w`,
				step.If.Val, stepIdx, step.Action.Val, err)
		}
		if !celResult {
			logger.DebugContext(ctx, `skipping step because "if" expression evaluated to false`,
				"step_index_from_0", stepIdx,
				"action", step.Action.Val,
				"cel_expr", step.If.Val)
			return nil
		}
		logger.DebugContext(ctx, `proceeding to execute step because "if" expression evaluated to true`,
			"step_index_from_0", stepIdx,
			"action", step.Action.Val,
			"cel_expr", step.If.Val)
	}

	switch {
	case step.Append != nil:
		return actionAppend(ctx, step.Append, sp)
	case step.ForEach != nil:
		return actionForEach(ctx, step.ForEach, sp)
	case step.GoTemplate != nil:
		return actionGoTemplate(ctx, step.GoTemplate, sp)
	case step.Include != nil:
		return actionInclude(ctx, step.Include, sp)
	case step.Print != nil:
		return actionPrint(ctx, step.Print, sp)
	case step.RegexNameLookup != nil:
		return actionRegexNameLookup(ctx, step.RegexNameLookup, sp)
	case step.RegexReplace != nil:
		return actionRegexReplace(ctx, step.RegexReplace, sp)
	case step.StringReplace != nil:
		return actionStringReplace(ctx, step.StringReplace, sp)
	default:
		return fmt.Errorf("internal error: unknown step action type %q", step.Action.Val)
	}
}

// Calls RemoveAll on each temp directory. A nonexistent directory is not an
// error. The "maybe" in the function name just means that we're not strict
// about the directories existing or about tempDirs being non-empty.
func (c *Command) maybeRemoveTempDirs(ctx context.Context, fs common.FS, tempDirs ...string) error {
	logger := logging.FromContext(ctx)
	if c.flags.KeepTempDirs {
		logger.WarnContext(ctx, "keeping temporary directories due to --keep-temp-dirs",
			"paths", tempDirs)
		return nil
	}
	logger.DebugContext(ctx, "removing all temporary directories (skip this with --keep-temp-dirs)")

	var merr error
	for _, p := range tempDirs {
		merr = errors.Join(merr, fs.RemoveAll(p))
	}
	return merr
}

// destOK makes sure that the output directory looks sane.
func destOK(fs fs.StatFS, dest string) error {
	fi, err := fs.Stat(dest)
	if err != nil {
		if common.IsStatNotExistErr(err) {
			return nil
		}
		return fmt.Errorf("os.Stat(%s): %w", dest, err)
	}

	if !fi.IsDir() {
		return fmt.Errorf("the destination %q exists but isn't a directory", dest)
	}

	return nil
}

func runCmds(ctx context.Context, argsList [][]string) error {
	for _, args := range argsList {
		_, _, err := common.Run(ctx, args...)
		if err != nil {
			return err //nolint:wrapcheck
		}
	}
	return nil
}
