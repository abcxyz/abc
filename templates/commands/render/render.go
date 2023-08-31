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
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
	"github.com/hashicorp/go-getter/v2"
	"github.com/mattn/go-isatty"
)

const (
	// These will be used as part of the names of the temporary directories to
	// make them identifiable.
	templateDirNamePart = "template-copy"
	scratchDirNamePart  = "scratch"

	// Permission bits: rwx------ .
	ownerRWXPerms = 0o700
	// Permission bits: rw------- .
	ownerRWPerms = 0o600

	defaultLogLevel = "warn"
	defaultLogMode  = "dev"

	// The spec file is always located in the template root dir and named spec.yaml.
	specName = "spec.yaml"
)

type RenderCommand struct {
	cli.BaseCommand
	flags RenderFlags

	testFS     renderFS
	testGetter getterClient
}

// Desc implements cli.Command.
func (c *RenderCommand) Desc() string {
	return "instantiate a template to setup a new app or add config files"
}

func (c *RenderCommand) Help() string {
	return `
Usage: {{ COMMAND }} [options] <source>

The {{ COMMAND }} command renders the given template.

The "<source>" is the location of the template to be instantiated. Many forms
are accepted:

  - "helloworld@v1" means "github.com/abcxyz/helloworld repo at revision v1;
    this is for a template owned by abcxyz.
  - "myorg/helloworld@v1" means github.com/myorg/helloworld repo at revision
    v1; this is for a template not owned by abcxyz but still on GitHub.
  - "mygithost.com/mygitrepo/helloworld@v1" is for a template in a remote git
    repo but not owned by abcxyz and not on GitHub.
  - "mylocaltarball.tgz" is for a template not in git but present on the local
    filesystem.
  - "http://example.com/myremotetarball.tgz" os for a non-Git template in a
    remote tarball.`
}

func (c *RenderCommand) Flags() *cli.FlagSet {
	set := c.NewFlagSet()
	c.flags.Register(set)
	return set
}

func (c *RenderCommand) Run(ctx context.Context, args []string) error {
	if err := c.Flags().Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	c.setLogEnvVars()
	ctx = logging.WithLogger(ctx, logging.NewFromEnv("ABC_"))

	fSys := c.testFS // allow filesystem interaction to be faked for testing
	if fSys == nil {
		fSys = &realFS{}
	}

	if err := destOK(fSys, c.flags.Dest); err != nil {
		return err
	}

	gg := c.testGetter
	if gg == nil {
		gg = &getter.Client{
			Getters:       getter.Getters,
			Decompressors: getter.Decompressors,
		}
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
		backupDir:    backupDir,
		cwd:          wd,
		fs:           fSys,
		getter:       gg,
		stdout:       c.Stdout(),
		tempDirNamer: tempDirName,
	})
}

// Abstracts filesystem operations.
//
// We can't use os.DirFS or fs.StatFS because they lack some methods we need. So
// we created our own interface.
type renderFS interface {
	fs.StatFS

	// These methods correspond to methods in the "os" package of the same name.
	MkdirAll(string, os.FileMode) error
	MkdirTemp(string, string) (string, error)
	OpenFile(string, int, os.FileMode) (*os.File, error)
	ReadFile(string) ([]byte, error)
	RemoveAll(string) error
	WriteFile(string, []byte, os.FileMode) error
}

// Allows the github.com/hashicorp/go-getter/Client to be faked.
type getterClient interface {
	Get(ctx context.Context, req *getter.Request) (*getter.GetResult, error)
}

// This is the non-test implementation of the filesystem interface.
type realFS struct{}

func (r *realFS) MkdirAll(name string, perm os.FileMode) error {
	return os.MkdirAll(name, perm) //nolint:wrapcheck
}

func (r *realFS) MkdirTemp(dir, pattern string) (string, error) {
	return os.MkdirTemp(dir, pattern) //nolint:wrapcheck
}

func (r *realFS) Open(name string) (fs.File, error) {
	return os.Open(name) //nolint:wrapcheck
}

func (r *realFS) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag, perm) //nolint:wrapcheck
}

func (r *realFS) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name) //nolint:wrapcheck
}

func (r *realFS) RemoveAll(name string) error {
	return os.RemoveAll(name) //nolint:wrapcheck
}

func (r *realFS) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name) //nolint:wrapcheck
}

func (r *realFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm) //nolint:wrapcheck
}

type runParams struct {
	backupDir string
	cwd       string
	fs        renderFS
	getter    getterClient
	stdout    io.Writer

	// Constructs a name for a temp directory that doesn't exist yet and won't
	// collide with other directories. Doesn't actually create a directory, the
	// caller does that. This accommodates quirky behavior of go-getter that
	// doesn't want the destination directory to exist already.
	tempDirNamer func(namePart string) (string, error)
}

// realRun is for testability; it's Run() with fakeable interfaces.
func (c *RenderCommand) realRun(ctx context.Context, rp *runParams) (outErr error) {
	var tempDirs []string
	defer func() {
		err := c.maybeRemoveTempDirs(ctx, rp.fs, tempDirs...)
		outErr = errors.Join(outErr, err)
	}()

	templateDir, err := c.copyTemplate(ctx, rp)
	if templateDir != "" { // templateDir might be set even if there's an error
		tempDirs = append(tempDirs, templateDir)
	}
	if err != nil {
		return err
	}

	spec, err := loadSpecFile(rp.fs, templateDir, specName)
	if err != nil {
		return err
	}

	if err := c.resolveInputs(ctx, spec); err != nil {
		return err
	}

	scratchDir, err := rp.tempDirNamer(scratchDirNamePart)
	if err != nil {
		return err
	}
	if err := rp.fs.MkdirAll(scratchDir, ownerRWXPerms); err != nil {
		return fmt.Errorf("failed to create scratch directory: MkdirAll(): %w", err)
	}
	tempDirs = append(tempDirs, scratchDir)
	logger := logging.FromContext(ctx)
	logger.InfoContext(ctx, "created temporary scratch directory", "path", scratchDir)

	sp := &stepParams{
		flags:       &c.flags,
		fs:          rp.fs,
		scope:       common.NewScope(c.flags.Inputs),
		scratchDir:  scratchDir,
		stdout:      rp.stdout,
		templateDir: templateDir,
	}
	if err := executeSteps(ctx, spec.Steps, sp); err != nil {
		return err
	}

	includedFromDest := sliceToSet(sp.includedFromDest)

	// Commit the contents of the scratch directory to the output directory. We
	// first do a dry-run to check that the copy is likely to succeed, so we
	// don't leave a half-done mess in the user's dest directory.
	for _, dryRun := range []bool{true, false} {
		visitor := func(relPath string, _ fs.DirEntry) (copyHint, error) {
			_, ok := includedFromDest[relPath]
			return copyHint{
				backupIfExists: true,

				// Special case: files that were "include"d from the
				// *destination* directory (rather than the template directory),
				// are always allowed to be overwritten. For example, if we grab
				// file_to_modify.txt from the --dest dir, then we always allow
				// ourself to write back to that file, even when
				// --force-overwrite=false. When the template uses this feature,
				// we know that the intent is to modify the files in place.
				overwrite: ok || c.flags.ForceOverwrite,
			}, nil
		}

		// We only want to call MkdirTemp once, and use the resulting backup
		// directory for every step in this rendering operation.
		var backupDir string
		backupDirMaker := func(rfs renderFS) (string, error) {
			if backupDir != "" {
				return backupDir, nil
			}
			if err := rfs.MkdirAll(rp.backupDir, ownerRWXPerms); err != nil {
				return "", err //nolint:wrapcheck // err already contains path, and it will be wrapped later
			}
			backupDir, err = rfs.MkdirTemp(rp.backupDir, "")
			logger.InfoContext(ctx, "created backup directory", "path", backupDir)
			return backupDir, err //nolint:wrapcheck // err already contains path, and it will be wrapped later
		}

		params := &copyParams{
			backupDirMaker: backupDirMaker,
			dryRun:         dryRun,
			dstRoot:        c.flags.Dest,
			srcRoot:        scratchDir,
			rfs:            rp.fs,
			visitor:        visitor,
		}
		if err := copyRecursive(ctx, nil, params); err != nil {
			return fmt.Errorf("failed writing to --dest directory: %w", err)
		}
		if dryRun {
			logger.DebugContext(ctx, "template render (dry run) succeeded")
		} else {
			logger.DebugContext(ctx, "template render succeeded")
		}
	}

	return nil
}

func sliceToSet[T comparable](vals []T) map[T]struct{} {
	out := make(map[T]struct{}, len(vals))
	for _, v := range vals {
		out[v] = struct{}{}
	}
	return out
}

// checkUnknownInputs checks for any unknown input flags and returns them in a slice.
func (c *RenderCommand) checkUnknownInputs(spec *model.Spec) []string {
	specInputs := make(map[string]any, len(spec.Inputs))
	for _, v := range spec.Inputs {
		specInputs[v.Name.Val] = struct{}{}
	}

	unknownInputs := make([]string, 0, len(c.flags.Inputs))
	for key := range c.flags.Inputs {
		if _, ok := specInputs[key]; !ok {
			unknownInputs = append(unknownInputs, key)
		}
	}

	sort.Strings(unknownInputs)

	return unknownInputs
}

// resolveInputs combines flags, user prompts, and defaults to get the full set
// of template inputs.
func (c *RenderCommand) resolveInputs(ctx context.Context, spec *model.Spec) error {
	if unknownInputs := c.checkUnknownInputs(spec); len(unknownInputs) > 0 {
		return fmt.Errorf("unknown input(s): %s", strings.Join(unknownInputs, ", "))
	}

	if c.flags.Prompt {
		isATTY := (c.Stdin() == os.Stdin && isatty.IsTerminal(os.Stdin.Fd()))
		if !isATTY {
			return fmt.Errorf("the flag --prompt was provided, but standard input is not a terminal")
		}

		if err := c.promptForInputs(ctx, spec); err != nil {
			return err
		}
	} else {
		c.collapseDefaultInputs(spec)
		if missing := c.checkInputsMissing(spec); len(missing) > 0 {
			return fmt.Errorf("missing input(s): %s", strings.Join(missing, ", "))
		}
	}

	if err := c.validateInputs(ctx, spec.Inputs); err != nil {
		return err
	}

	return nil
}

func (c *RenderCommand) validateInputs(ctx context.Context, inputs []*model.Input) error {
	scope := common.NewScope(c.flags.Inputs)

	sb := &strings.Builder{}
	tw := tabwriter.NewWriter(sb, 8, 0, 2, ' ', 0)

	for _, input := range inputs {
		for _, rule := range input.Rules {
			var ok bool
			err := common.CelCompileAndEval(ctx, scope, rule.Rule, &ok)
			if ok && err == nil {
				continue
			}

			fmt.Fprintf(tw, "\nInput name:\t%s", input.Name.Val)
			fmt.Fprintf(tw, "\nInput value:\t%s", c.flags.Inputs[input.Name.Val])
			writeRule(tw, rule, false, 0)
			if err != nil {
				fmt.Fprintf(tw, "\nCEL error:\t%s", err.Error())
			}
			fmt.Fprintf(tw, "\n") // Add vertical relief between validation messages
		}
	}

	tw.Flush()
	if sb.Len() > 0 {
		return fmt.Errorf("input validation failed:\n%s", sb.String())
	}
	return nil
}

// promptForInputs looks for template inputs that were not provided on the
// command line and prompts the user for them. This mutates c.flags.Inputs.
//
// This must only be called when the user specified --prompt and the input is a
// terminal (or in a test).
func (c *RenderCommand) promptForInputs(ctx context.Context, spec *model.Spec) error {
	for _, i := range spec.Inputs {
		if _, ok := c.flags.Inputs[i.Name.Val]; ok {
			// Don't prompt if the cmdline had an --input for this key.
			continue
		}
		sb := &strings.Builder{}
		tw := tabwriter.NewWriter(sb, 8, 0, 2, ' ', 0)
		fmt.Fprintf(tw, "\nInput name:\t%s", i.Name.Val)
		fmt.Fprintf(tw, "\nDescription:\t%s", i.Desc.Val)
		for idx, rule := range i.Rules {
			printRuleIndex := len(i.Rules) > 1
			writeRule(tw, rule, printRuleIndex, idx)
		}

		if i.Default != nil {
			defaultStr := i.Default.Val
			if defaultStr == "" {
				// When empty string is the default, print it differently so
				// the user can actually see what's happening.
				defaultStr = `""`
			}
			fmt.Fprintf(tw, "\nDefault:\t%s", defaultStr)
		}

		tw.Flush()

		if i.Default != nil {
			fmt.Fprintf(sb, "\n\nEnter value, or leave empty to accept default: ")
		} else {
			fmt.Fprintf(sb, "\n\nEnter value: ")
		}

		inputVal, err := c.Prompt(ctx, sb.String())
		if err != nil {
			return fmt.Errorf("failed to prompt for user input: %w", err)
		}

		if inputVal == "" && i.Default != nil {
			inputVal = i.Default.Val
		}

		c.flags.Inputs[i.Name.Val] = inputVal
	}
	return nil
}

// writeRule writes a human-readable description of the given rule to the given
// tabwriter in a 2-column format.
//
// Sometimes we run this in a context where we want to include the index of the
// rule in the list of rules; in that case, pass includeIndex=true and the index
// value. If includeIndex is false, then index is ignored.
func writeRule(tw *tabwriter.Writer, rule *model.InputRule, includeIndex bool, index int) {
	indexStr := ""
	if includeIndex {
		indexStr = fmt.Sprintf(" %d", index)
	}

	fmt.Fprintf(tw, "\nRule%s:\t%s", indexStr, rule.Rule.Val)
	if rule.Message.Val != "" {
		fmt.Fprintf(tw, "\nRule%s msg:\t%s", indexStr, rule.Message.Val)
	}
}

// collapseDefaultInputs defaults any missing input flags if default is set.
func (c *RenderCommand) collapseDefaultInputs(spec *model.Spec) {
	if c.flags.Inputs == nil {
		c.flags.Inputs = map[string]string{}
	}
	for _, input := range spec.Inputs {
		if _, ok := c.flags.Inputs[input.Name.Val]; !ok && input.Default != nil {
			c.flags.Inputs[input.Name.Val] = input.Default.Val
		}
	}
}

// checkInputsMissing checks for missing input flags returns them as a slice.
func (c *RenderCommand) checkInputsMissing(spec *model.Spec) []string {
	missing := make([]string, 0, len(c.flags.Inputs))

	for _, input := range spec.Inputs {
		if _, ok := c.flags.Inputs[input.Name.Val]; !ok {
			missing = append(missing, input.Name.Val)
		}
	}

	sort.Strings(missing)

	return missing
}

func executeSteps(ctx context.Context, steps []*model.Step, sp *stepParams) error {
	logger := logging.FromContext(ctx).With("logger", "executeSpec")

	for i, step := range steps {
		if err := executeOneStep(ctx, step, sp); err != nil {
			return err
		}
		logger.DebugContext(ctx, "completed template action", "action", step.Action.Val)
		if sp.flags.DebugScratchContents {
			contents, err := scratchContents(ctx, i, step, sp)
			if err != nil {
				return err
			}
			logger.InfoContext(ctx, contents)
		}
	}
	return nil
}

func scratchContents(ctx context.Context, stepIdx int, step *model.Step, sp *stepParams) (string, error) {
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
		fmt.Fprintf(sb, "  %s\n", rel)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("error crawling scratch directory: %w", err)
	}
	return sb.String(), nil
}

type stepParams struct {
	flags *RenderFlags
	fs    renderFS

	// Scope contains all variable names that are in scope. This includes
	// user-provided inputs, as well as any programmatically created variables
	// like for_each keys.
	scope *common.Scope

	scratchDir  string
	stdout      io.Writer
	templateDir string

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
}

// WithScope returns a copy of this stepParams with a new inner variable scope
// containing some extra variable bindings.
func (s *stepParams) WithScope(vars map[string]string) *stepParams {
	out := *s
	out.scope = s.scope.With(vars)
	return &out
}

func executeOneStep(ctx context.Context, step *model.Step, sp *stepParams) error {
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

// A fancy wrapper around MkdirAll with better error messages and a dry run
// mode. In dry run mode, returns an error if the MkdirAll wouldn't succeed
// (best-effort).
func mkdirAllChecked(pos *model.ConfigPos, rfs renderFS, path string, dryRun bool) error {
	create := false
	info, err := rfs.Stat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return pos.Errorf("Stat(): %w", err)
		}
		create = true
	} else if !info.Mode().IsDir() {
		return pos.Errorf("cannot overwrite a file with a directory of the same name, %q", path)
	}

	if dryRun || !create {
		return nil
	}

	if err := rfs.MkdirAll(path, ownerRWXPerms); err != nil {
		return pos.Errorf("MkdirAll(): %w", err)
	}

	return nil
}

func loadSpecFile(fs renderFS, templateDir, flagSpec string) (*model.Spec, error) {
	specPath := filepath.Join(templateDir, flagSpec)
	f, err := fs.Open(specPath)
	if err != nil {
		return nil, fmt.Errorf("error opening template spec: ReadFile(): %w", err)
	}
	defer f.Close()

	spec, err := model.DecodeSpec(f)
	if err != nil {
		return nil, fmt.Errorf("error reading template spec file: %w", err)
	}

	return spec, nil
}

// Downloads the template and returns the name of the temp directory where it
// was saved. If error is returned, then the returned directory name may or may
// not exist, and may or may not be empty.
func (c *RenderCommand) copyTemplate(ctx context.Context, rp *runParams) (string, error) {
	templateDir, err := rp.tempDirNamer(templateDirNamePart)
	if err != nil {
		return "", err
	}
	req := &getter.Request{
		DisableSymlinks: true,
		Dst:             templateDir,
		GetMode:         getter.ModeAny,
		Pwd:             rp.cwd,
		Src:             c.flags.Source,
	}

	_, err = rp.getter.Get(ctx, req)
	if err != nil {
		return templateDir, fmt.Errorf("go-getter.Get(): %w", err)
	}
	logger := logging.FromContext(ctx)
	logger.InfoContext(ctx, "created temporary template directory",
		"path", templateDir)
	logger.InfoContext(ctx, "copied source template temporary directory",
		"source", c.flags.Source,
		"destination", templateDir)
	return templateDir, nil
}

// Calls RemoveAll on each temp directory. A nonexistent directory is not an error.
func (c *RenderCommand) maybeRemoveTempDirs(ctx context.Context, fs renderFS, tempDirs ...string) error {
	logger := logging.FromContext(ctx)
	if c.flags.KeepTempDirs {
		logger.InfoContext(ctx, "keeping temporary directories due to --keep-temp-dirs",
			"paths", tempDirs)
		return nil
	}
	logger.InfoContext(ctx, "removing all temporary directories (skip this with --keep-temp-dirs)")

	var merr error
	for _, p := range tempDirs {
		merr = errors.Join(merr, fs.RemoveAll(p))
	}
	return merr
}

func (c *RenderCommand) setLogEnvVars() {
	if os.Getenv("ABC_LOG_MODE") == "" {
		os.Setenv("ABC_LOG_MODE", defaultLogMode)
	}

	if c.flags.LogLevel != "" {
		os.Setenv("ABC_LOG_LEVEL", c.flags.LogLevel)
	} else if os.Getenv("ABC_LOG_LEVEL") == "" {
		os.Setenv("ABC_LOG_LEVEL", defaultLogLevel)
	}
}

// Generate the name for a temporary directory, without creating it. namePart is
// an optional name that can be included to help template developers distinguish
// between the various template directories created by this program, such as
// "template" or "scratch".
//
// We can't use MkdirTemp() for a go-getter output directory because go-getter
// silently fails to clone a git repo into an existing (empty) directory.
// go-getter assumes that the dir must already be a git repo if it exists.
func tempDirName(namePart string) (string, error) {
	rnd, err := randU64()
	if err != nil {
		return "", err
	}
	basename := fmt.Sprintf("abc-%s-%d", namePart, rnd)
	return filepath.Join(os.TempDir(), basename), nil
}

func randU64() (uint64, error) {
	randBuf := make([]byte, 8)
	if _, err := rand.Read(randBuf); err != nil { // safe to ignore returned int per docs, "n == len(b) if and only if err == nil"
		return 0, fmt.Errorf("rand.Read(): %w", err)
	}
	return binary.BigEndian.Uint64(randBuf), nil
}

// destOK makes sure that the output directory looks sane.
func destOK(fs fs.StatFS, dest string) error {
	fi, err := fs.Stat(dest)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("os.Stat(%s): %w", dest, err)
	}

	if !fi.IsDir() {
		return fmt.Errorf("the destination %q exists but isn't a directory", dest)
	}

	return nil
}
