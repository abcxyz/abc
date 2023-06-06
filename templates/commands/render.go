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

// Package render implements the "templates render" subcommand for installing a template.
package commands

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
	"github.com/hashicorp/go-getter/v2"
)

const (
	// These will be used as part of the names of the temporary directories to
	// make them identifiable.
	templateDirNamePart = "template-copy"
	scratchDirNamePart  = "scratch"
)

type Render struct {
	cli.BaseCommand

	testFS     renderFS
	testGetter getterClient

	source             string
	flagSpec           string
	flagDest           string
	flagGitProtocol    string
	flagLogLevel       string
	flagForceOverwrite bool
	flagKeepTempDirs   bool
	flagInputs         map[string]string
}

// Abstracts filesystem operations.
//
// We can't use os.DirFS or fs.StatFS because they lack some methods we need. So
// we created our own interface.
type renderFS interface {
	fs.StatFS

	// These methods correspond to methods in the "os" package of the same name.
	Mkdir(name string, perm os.FileMode) error
	MkdirAll(string, os.FileMode) error
	ReadFile(string) ([]byte, error)
	RemoveAll(string) error
	WriteFile(string, []byte, os.FileMode) error
}

// Allows the github.com/hashicorp/go-getter/Client to be faked.
type getterClient interface {
	Get(ctx context.Context, req *getter.Request) (*getter.GetResult, error)
}

// Desc implements cli.Command.
func (r *Render) Desc() string {
	return "instantiate a template to setup a new app or add config files"
}

func (r *Render) Help() string {
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

func (r *Render) Flags() *cli.FlagSet {
	set := cli.NewFlagSet()

	f := set.NewSection("RENDER OPTIONS")
	f.StringVar(&cli.StringVar{
		Name:    "spec",
		Example: "path/to/spec.yaml",
		Target:  &r.flagSpec,
		Default: "./spec.yaml",
		Usage:   "The path of the .yaml file within the unpacked template directory that specifies how the template is rendered.",
	})
	f.StringVar(&cli.StringVar{
		Name:    "dest",
		Aliases: []string{"d"},
		Example: "/my/git/dir",
		Target:  &r.flagDest,
		Default: ".",
		Usage:   "Required. The target directory in which to write the output files.",
	})
	f.StringMapVar(&cli.StringMapVar{
		Name:    "input",
		Example: "foo=bar",
		Target:  &r.flagInputs,
		Usage:   "The key=val pairs of template values; may be repeated.",
	})
	f.StringVar(&cli.StringVar{
		Name:    "log-level",
		Example: "info",
		Default: "warning",
		Target:  &r.flagLogLevel,
		Usage:   "How verbose to log; any of debug|info|warning|error.",
	})
	f.BoolVar(&cli.BoolVar{
		Name:    "force-overwrite",
		Target:  &r.flagForceOverwrite,
		Default: false,
		Usage:   "If an output file already exists in the destination, overwrite it instead of failing.",
	})
	f.BoolVar(&cli.BoolVar{
		Name:    "keep-temp-dirs",
		Target:  &r.flagKeepTempDirs,
		Default: false,
		Usage:   "Preserve the temp directories instead of deleting them normally.",
	})

	g := set.NewSection("GIT OPTIONS")
	g.StringVar(&cli.StringVar{
		Name:    "git-protocol",
		Example: "https",
		Default: "https",
		Target:  &r.flagGitProtocol,
		Usage:   "Either ssh or https, the protocol for connecting to git. Only used if the template source is a git repo.",
	})

	return set
}

func (r *Render) parseFlags(args []string) error {
	flagSet := r.Flags()

	if err := flagSet.Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	parsedArgs := flagSet.Args()
	if len(parsedArgs) != 1 {
		return flag.ErrHelp
	}

	r.source = parsedArgs[0]

	if err := safeRelPath(r.flagSpec); err != nil {
		return fmt.Errorf("invalid --spec path %q: %w", r.flagSpec, err)
	}

	return nil
}

// This is the non-test implementation of the filesystem interface.
type realFS struct{}

func (r *realFS) Mkdir(name string, perm os.FileMode) error {
	return os.Mkdir(name, perm) //nolint:wrapcheck
}

func (r *realFS) MkdirAll(name string, perm os.FileMode) error {
	return os.MkdirAll(name, perm) //nolint:wrapcheck
}

func (r *realFS) Open(name string) (fs.File, error) {
	return os.Open(name) //nolint:wrapcheck
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

func (r *Render) Run(ctx context.Context, args []string) error {
	ctx = logging.WithLogger(ctx, logging.NewFromEnv("ABC_"))

	if err := r.parseFlags(args); err != nil {
		return err
	}

	fSys := r.testFS // allow filesystem interaction to be faked for testing
	if fSys == nil {
		fSys = &realFS{}
	}

	if err := destOK(fSys, r.flagDest); err != nil {
		return err
	}

	gg := r.testGetter
	if gg == nil {
		gg = &getter.Client{
			Getters:       getter.Getters,
			Decompressors: getter.Decompressors,
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("os.Getwd(): %w", err)
	}

	return r.realRun(ctx, &runParams{
		cwd:          wd,
		fs:           fSys,
		getter:       gg,
		stdout:       os.Stdout,
		tempDirNamer: tempDirName,
	})
}

type runParams struct {
	cwd    string
	fs     renderFS
	getter getterClient
	stdout io.Writer

	// Constructs a name for a temp directory that doesn't exist yet and won't
	// collide with other directories. Doesn't actually create a directory, the
	// caller does that. This accommodates quirky behavior of go-getter that
	// doesn't want the destination directory to exist already.
	tempDirNamer func(namePart string) (string, error)
}

// realRun is for testability; it's Run() with fakeable interfaces.
func (r *Render) realRun(ctx context.Context, rp *runParams) (outErr error) {
	templateDir, err := r.copyTemplate(ctx, rp)
	if templateDir != "" { // templateDir might be set even if there's an error
		defer func() {
			outErr = errors.Join(outErr, r.maybeRemoveTempDirs(ctx, rp.fs, templateDir))
		}()
	}
	if err != nil {
		return err
	}

	spec, err := loadSpecFile(rp.fs, templateDir, r.flagSpec)
	if err != nil {
		return err
	}

	scratchDir, err := rp.tempDirNamer(scratchDirNamePart)
	if err != nil {
		return err
	}
	if err := rp.fs.MkdirAll(scratchDir, 0o700); err != nil {
		return fmt.Errorf("failed to create scratch directory: MkdirAll(): %w", err)
	}

	if err := executeSpec(ctx, spec, &stepParams{
		inputs:      r.flagInputs,
		fs:          rp.fs,
		scratchDir:  scratchDir,
		stdout:      rp.stdout,
		templateDir: templateDir,
	}); err != nil {
		return err
	}

	return nil
}

func executeSpec(ctx context.Context, spec *model.Spec, sp *stepParams) error {
	for _, step := range spec.Steps {
		if err := executeOneStep(ctx, step, sp); err != nil {
			return err
		}
	}
	return nil
}

type stepParams struct {
	fs          renderFS
	inputs      map[string]string
	scratchDir  string
	stdout      io.Writer
	templateDir string
}

func executeOneStep(ctx context.Context, step *model.Step, sp *stepParams) error {
	switch {
	case step.Print != nil:
		return actionPrint(ctx, step.Print, sp)
	case step.Include != nil:
		return actionInclude(ctx, step.Include, sp)
	case step.RegexReplace != nil:
		return actionRegexReplace(ctx, step.RegexReplace, sp)
	case step.StringReplace != nil:
		return actionStringReplace(ctx, step.StringReplace, sp)
	case step.GoTemplate != nil:
		return actionGoTemplate(ctx, step.GoTemplate, sp)
	default:
		return fmt.Errorf("internal error: unknown step action type %q", step.Action.Val)
	}
}

// A template parser helper to remove the boilerplate of parsing with our
// desired options.
func parseGoTmpl(tpl string) (*template.Template, error) {
	return template.New("").Option("missingkey=error").Parse(tpl) //nolint:wrapcheck
}

func parseAndExecuteGoTmpl(m model.String, inputs map[string]string) (string, error) {
	goTmpl, err := parseGoTmpl(m.Val)
	if err != nil {
		return "", model.ErrWithPos(m.Pos, `error compiling as go-template: %w`, err) //nolint:wrapcheck
	}

	var sb strings.Builder
	if err := goTmpl.Execute(&sb, inputs); err != nil {
		return "", model.ErrWithPos(m.Pos, "template.Execute() failed: %w", err) //nolint:wrapcheck
	}

	return sb.String(), nil
}

// "from" may be a file or directory. "pos" is only used for error messages.
func copyRecursive(pos *model.ConfigPos, from, to string, rfs renderFS) error {
	return fs.WalkDir(rfs, from, func(path string, d fs.DirEntry, err error) error { //nolint:wrapcheck
		if err != nil {
			return err // There was some filesystem error. Give up.
		}

		// We don't have to worry about symlinks here because we passed
		// DisableSymlinks=true to go-getter.

		relToSrc, err := filepath.Rel(from, path)
		if err != nil {
			return model.ErrWithPos(pos, "filepath.Rel(%s,%s): %w", from, path, err) //nolint:wrapcheck
		}

		dst := filepath.Join(to, relToSrc)

		// The spec file may specify a file to copy that's deep in a
		// directory tree, without naming its parent directory. We can't
		// rely on WalkDir having traversed the parent directory of $path,
		// so we must create the target directory if it doesn't exist.
		dirToCreate := dst
		if !d.IsDir() {
			dirToCreate = filepath.Dir(dst)
		}
		if err := rfs.MkdirAll(dirToCreate, 0o700); err != nil {
			return model.ErrWithPos(pos, "MkdirAll(): %w", err) //nolint:wrapcheck
		}
		if d.IsDir() {
			return nil
		}

		buf, err := rfs.ReadFile(path)
		if err != nil {
			return model.ErrWithPos(pos, "ReadFile(): %w", err) //nolint:wrapcheck
		}

		info, err := rfs.Stat(path)
		if err != nil {
			return fmt.Errorf("Stat(): %w", err)
		}

		// The permission bits on the output file are copied from the input file;
		// this preserves the execute bit on executable files.
		if err := rfs.WriteFile(dst, buf, info.Mode().Perm()); err != nil {
			return fmt.Errorf("failed writing to scratch file: WriteFile(): %w", err)
		}

		return nil
	})
}

func loadSpecFile(fs renderFS, templateDir, flagSpec string) (*model.Spec, error) {
	f, err := fs.Open(filepath.Join(templateDir, flagSpec))
	if err != nil {
		return nil, fmt.Errorf("error opening template spec: ReadFile(): %w", err)
	}
	defer f.Close()

	decoder := model.NewDecoder(f)
	var spec model.Spec
	if err := decoder.Decode(&spec); err != nil {
		return nil, fmt.Errorf("error parsing YAML spec file: %w", err)
	}
	return &spec, nil
}

// Downloads the template and returns the name of the temp directory where it
// was saved. If error is returned, then the returned directory name may or may
// not exist, and may or may not be empty.
func (r *Render) copyTemplate(ctx context.Context, rp *runParams) (string, error) {
	templateDir, err := rp.tempDirNamer(templateDirNamePart)
	if err != nil {
		return "", err
	}
	req := &getter.Request{
		DisableSymlinks: true,
		Dst:             templateDir,
		GetMode:         getter.ModeAny,
		Pwd:             rp.cwd,
		Src:             r.source,
	}

	res, err := rp.getter.Get(ctx, req)
	if err != nil {
		return templateDir, fmt.Errorf("go-getter.Get(): %w", err)
	}

	logging.FromContext(ctx).Debugf("copied source template %q into temporary directory %q", r.source, res.Dst)
	return templateDir, nil
}

// Calls RemoveAll on each temp directory. A nonexistent directory is not an error.
func (r *Render) maybeRemoveTempDirs(ctx context.Context, fs renderFS, tempDirs ...string) error {
	logger := logging.FromContext(ctx)
	if r.flagKeepTempDirs {
		logger.Infof("keeping temporary directories due to --keep-temp-dirs. Locations are: %v", tempDirs)
		return nil
	}
	logger.Debugf("removing temporary directories (skip this with --keep-temp-dirs)")

	var merr error
	for _, p := range tempDirs {
		merr = errors.Join(merr, fs.RemoveAll(p))
	}
	return merr
}

// Generate the name for a temporary directory, without creating it. namePart is
// an optional name that can be included to help template developers distinguish
// between the various template directories created by this program, such as
// "template" or "scratch".
//
// We can't use os.MkdirTemp() for a go-getter output directory because
// go-getter silently fails to clone a git repo into an existing (empty)
// directory. go-getter assumes that the dir must already be a git repo if it
// exists.
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
			return fmt.Errorf("the destination directory doesn't exist: %q", dest)
		}
		return fmt.Errorf("os.Stat(%s): %w", dest, err)
	}

	if !fi.IsDir() {
		return fmt.Errorf("the destination %q is not a directory", dest)
	}

	return nil
}

// safeRelPath returns an error if the path is absolute or if it contains a ".." traversal.
func safeRelPath(p string) error {
	if strings.Contains(p, "..") {
		return fmt.Errorf(`path must not contain ".."`)
	}
	if filepath.IsAbs(p) {
		return fmt.Errorf(`path must be relative, not absolute`)
	}
	return nil
}
