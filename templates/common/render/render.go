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

package render

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/benbjohnson/clock"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/input"
	"github.com/abcxyz/abc/templates/common/paths"
	"github.com/abcxyz/abc/templates/common/specutil"
	"github.com/abcxyz/abc/templates/common/templatesource"
	"github.com/abcxyz/abc/templates/model"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta3"
	"github.com/abcxyz/pkg/logging"
)

// Params contains the arguments to Render().
type Params struct {
	// BackupDir is the directory where overwritten files will be backed up.
	// BackupDir is ignored if Backups is false.
	BackupDir string
	Backups   bool

	// Fakeable time for testing.
	Clock clock.Clock

	// The fakeable working directory for testing.
	Cwd string

	// The value of --debug-scratch-contents.
	DebugScratchContents bool

	// The value of --debug-step-diffs.
	DebugStepDiffs bool

	// The value of --dest.
	DestDir string

	// The value of --force-overwrite.
	ForceOverwrite bool

	// A fakeable filesystem for error injection in tests.
	FS common.FS

	// The value of --git-protocol.
	GitProtocol string

	// The value of --input-files.
	InputFiles []string

	// The value of --input, or another source of input values (e.g. the golden
	// test test.yaml).
	Inputs map[string]string

	// The value of --keep-temp-dirs.
	KeepTempDirs bool

	// The value of --manifest.
	Manifest bool

	// Whether to prompt the user for inputs on stdin in the case where they're
	// not all provided in Inputs or InputFiles.
	Prompt bool
	// If Prompt is true, Prompter will be used if needed to ask the user for
	// any missing inputs. If Prompt is false, this is ignored.
	Prompter input.Prompter

	// The value of --skip-input-validation.
	SkipInputValidation bool

	// The location from which the template is installed, as provided by the
	// user on the command line.
	Source string

	// The output stream used by "print" actions.
	Stdout io.Writer

	// The directory under which to create temp directories. Normally empty,
	// except in testing.
	TempDirBase string
}

// Render does the full sequence of steps involved in rendering a template. It
// downloads the template, parses the spec file, read template inputs, conditionally
// prompts the user for missing inputs, runs all the template actions, commits the
// output to the destination, and more.
//
// This is a library function because template rendering is a reusable operation
// that is called as a subroutine by "golden-test" and "upgrade" commands.
func Render(ctx context.Context, p *Params) (outErr error) {
	logger := logging.FromContext(ctx).With("logger", "Render")

	tempRemover := newTempDirRemover(p.FS, p.KeepTempDirs)
	defer func() {
		outErr = errors.Join(outErr, tempRemover.maybeRemoveAll(ctx))
	}()

	dlMeta, templateDir, err := templatesource.Download(ctx, &templatesource.DownloadParams{
		CWD:         p.Cwd,
		FS:          p.FS,
		TempDirBase: p.TempDirBase,
		Source:      p.Source,
		Dest:        p.DestDir,
		GitProtocol: p.GitProtocol,
	})
	tempRemover.append(templateDir) // templateDir might be set even if there's an error
	if err != nil {
		return err //nolint:wrapcheck
	}

	spec, err := specutil.Load(ctx, p.FS, templateDir, p.Source)
	if err != nil {
		return err //nolint:wrapcheck
	}

	resolvedInputs, err := input.Resolve(ctx, &input.ResolveParams{
		FS:                  p.FS,
		InputFiles:          p.InputFiles,
		Inputs:              p.Inputs,
		Prompt:              p.Prompt,
		Prompter:            p.Prompter,
		SkipInputValidation: p.SkipInputValidation,
		Spec:                spec,
	})
	if err != nil {
		return err //nolint:wrapcheck
	}

	scratchDir, err := p.FS.MkdirTemp(p.TempDirBase, paths.ScratchDirNamePart)
	if err != nil {
		return fmt.Errorf("failed to create temp directory for scratch directory: %w", err)
	}
	tempRemover.append(scratchDir)

	logger.DebugContext(ctx, "created temporary scratch directory", "path", scratchDir)

	debugStepDiffsDir, err := initDebugStepDiffsDir(ctx, p, scratchDir)
	if err != nil {
		return err
	}

	sp := &stepParams{
		debugDiffsDir:  debugStepDiffsDir,
		ignorePatterns: spec.Ignore,
		features:       spec.Features,
		rp:             p,
		scope:          common.NewScope(resolvedInputs),
		scratchDir:     scratchDir,
		templateDir:    templateDir,
	}

	if err := executeSteps(ctx, spec.Steps, sp); err != nil {
		return err
	}

	if err := commitTentatively(ctx, p, &commitParams{
		dlMeta:           dlMeta,
		includedFromDest: sliceToSet(sp.includedFromDest),
		inputs:           resolvedInputs,
		scratchDir:       scratchDir,
		templateDir:      templateDir,
	}); err != nil {
		return err
	}

	if p.DebugStepDiffs {
		// Use default log level.
		logger.WarnContext(
			ctx,
			fmt.Sprintf(
				"Please navigate to '%s' or use 'git --git-dir=%s log' to see commits/diffs for each step",
				debugStepDiffsDir, debugStepDiffsDir),
		)
	}

	return nil
}

// Configure the git directory that will contain a commit per step for debugging
// purposes. If --debug-step-diffs is false, this is a noop.
func initDebugStepDiffsDir(ctx context.Context, p *Params, scratchDir string) (string, error) {
	if !p.DebugStepDiffs {
		return "", nil // This particular debugging feature isn't enabled
	}

	out, err := p.FS.MkdirTemp(p.TempDirBase, paths.DebugStepDiffsDirNamePart)
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory for debug directory: %w", err)
	}

	cmds := [][]string{
		// Make debug dir a git repository with a detached work tree in scratch
		// dir, meaning it will track the file changes in scratch dir without
		// affecting the scratch dir.
		{"git", "--git-dir", out, "--work-tree", scratchDir, "init"},

		// Set git user name and email, required for ubuntu and windows os.
		{"git", "--git-dir", out, "config", "user.name", "abc CLI"},
		{"git", "--git-dir", out, "config", "user.email", "abc@abcxyz.com"},
	}

	if _, _, err := common.RunMany(ctx, cmds...); err != nil {
		return "", fmt.Errorf("failed initializing git repo for --debug-step-diffs: %w", err)
	}
	return out, nil
}

// stepParams contains all the values provided to the action* functions that
// are needed to do their job.
type stepParams struct {
	rp *Params

	// The feature flags controlling how to interpret the spec file.
	features *spec.Features

	// Files and directories included in spec that match ignorePatterns will be
	// ignored while being copied to destination directory.
	ignorePatterns []model.String

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

	// Scope contains all variable names that are in scope. This includes
	// user-provided inputs, as well as any programmatically created variables
	// like for_each keys.
	scope *common.Scope

	debugDiffsDir string
	scratchDir    string
	templateDir   string
}

// WithScope returns a copy of this stepParams with a new inner variable scope
// containing some extra variable bindings.
func (s *stepParams) WithScope(vars map[string]string) *stepParams {
	out := *s
	out.scope = s.scope.With(vars)
	return &out
}

// executeSteps is the heart of template rendering. It executes each action in
// the spec sequentially.
func executeSteps(ctx context.Context, steps []*spec.Step, sp *stepParams) error {
	logger := logging.FromContext(ctx).With("logger", "executeSteps")

	for i, step := range steps {
		if err := executeOneStep(ctx, i, step, sp); err != nil {
			return err
		}

		if sp.debugDiffsDir != "" {
			// Commit the diffs after each step.
			m := fmt.Sprintf("action %s at line %d", step.Action.Val, step.Pos.Line)
			cmds := [][]string{
				{"git", "--git-dir", sp.debugDiffsDir, "add", "-A"},
				{"git", "--git-dir", sp.debugDiffsDir, "commit", "-a", "-m", m, "--allow-empty", "--no-gpg-sign"},
			}
			if _, _, err := common.RunMany(ctx, cmds...); err != nil {
				return fmt.Errorf("failed committing to git for --debug-step-diffs: %w", err)
			}
		}

		logger.DebugContext(ctx, "completed template action", "action", step.Action.Val)
		if sp.rp.DebugScratchContents {
			contents, err := scratchContents(ctx, i, step, sp)
			if err != nil {
				return err
			}
			logger.WarnContext(ctx, contents)
		}
	}
	return nil
}

// executeOneStep runs one action from the spec.
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

// scratchContents returns the contents of the scratch dir for debugging purposes; it's
// only used if --debug-scratch-contents=true.
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

// commitParams contains the arguments to commitTentatively().
type commitParams struct {
	dlMeta           *templatesource.DownloadMetadata
	scratchDir       string
	templateDir      string
	includedFromDest map[string]struct{}
	inputs           map[string]string
}

// commitTentatively writes the contents of the scratch directory to the output
// directory. We first do a dry-run to check that the copy is likely to succeed,
// so we don't leave a half-done mess in the user's dest directory.
func commitTentatively(ctx context.Context, p *Params, cp *commitParams) error {
	for _, dryRun := range []bool{true, false} {
		outputHashes, err := commit(ctx, dryRun, p, cp.scratchDir, cp.includedFromDest)
		if err != nil {
			return err
		}

		if p.Manifest {
			if err := writeManifest(ctx, &writeManifestParams{
				clock:        p.Clock,
				cwd:          p.Cwd,
				dlMeta:       cp.dlMeta,
				destDir:      p.DestDir,
				dryRun:       dryRun,
				fs:           p.FS,
				inputs:       cp.inputs,
				outputHashes: outputHashes,
				templateDir:  cp.templateDir,
			}); err != nil {
				return err
			}
		}
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
func commit(ctx context.Context, dryRun bool, p *Params, scratchDir string, includedFromDest map[string]struct{}) (map[string][]byte, error) {
	logger := logging.FromContext(ctx).With("logger", "commit")

	visitor := func(relPath string, _ fs.DirEntry) (common.CopyHint, error) {
		_, ok := includedFromDest[relPath]
		return common.CopyHint{
			BackupIfExists: p.Backups,

			// Special case: files that were "include"d from the
			// *destination* directory (rather than the template directory),
			// are always allowed to be overwritten. For example, if we grab
			// file_to_modify.txt from the --dest dir, then we always allow
			// ourself to write back to that file, even when
			// --force-overwrite=false. When the template uses this feature,
			// we know that the intent is to modify the files in place.
			Overwrite: ok || p.ForceOverwrite,
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
		if err := rfs.MkdirAll(p.BackupDir, common.OwnerRWXPerms); err != nil {
			return "", err //nolint:wrapcheck // err already contains path, and it will be wrapped later
		}
		backupDir, err = rfs.MkdirTemp(p.BackupDir, "")
		logger.DebugContext(ctx, "created backup directory", "path", backupDir)
		return backupDir, err //nolint:wrapcheck // err already contains path, and it will be wrapped later
	}

	params := &common.CopyParams{
		BackupDirMaker: backupDirMaker,
		DryRun:         dryRun,
		DstRoot:        p.DestDir,
		Hasher:         sha256.New,
		OutHashes:      map[string][]byte{},
		SrcRoot:        scratchDir,
		FS:             p.FS,
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

// tempDirRemover helps manage the removal of temporary directories when
// rendering is finished.
type tempDirRemover struct {
	fs           common.FS
	tempDirs     []string
	keepTempDirs bool
}

func newTempDirRemover(fs common.FS, keepTempDirs bool) *tempDirRemover {
	return &tempDirRemover{
		fs:           fs,
		keepTempDirs: keepTempDirs,
	}
}

func (t *tempDirRemover) append(dir string) {
	if dir == "" {
		return
	}
	t.tempDirs = append(t.tempDirs, dir)
}

// maybeRemoveAll should be called in a defer to clean up temp dirs, like this:
//
//		 defer func() {
//		   outErr = errors.Join(outErr, t.maybeRemoveAll(ctx))
//	     }()
func (t *tempDirRemover) maybeRemoveAll(ctx context.Context) error {
	logger := logging.FromContext(ctx).With("logger", "tempDirRemover.Remove")
	if t.keepTempDirs {
		logger.WarnContext(ctx, "keeping temporary directories due to --keep-temp-dirs",
			"paths", t.tempDirs)
		return nil
	}

	logger.DebugContext(ctx, "removing all temporary directories (skip this with --keep-temp-dirs)")

	var merr error
	for _, p := range t.tempDirs {
		merr = errors.Join(merr, t.fs.RemoveAll(p))
	}
	return merr
}

func sliceToSet[T comparable](vals []T) map[T]struct{} {
	out := make(map[T]struct{}, len(vals))
	for _, v := range vals {
		out[v] = struct{}{}
	}
	return out
}