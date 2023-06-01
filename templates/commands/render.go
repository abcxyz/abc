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
	"io/fs"
	"os"
	"path/filepath"

	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
	"github.com/hashicorp/go-getter/v2"
	"go.uber.org/zap"
)

type Render struct {
	cli.BaseCommand

	logger *zap.SugaredLogger

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

// The fs.StatFS doesn't include all the methods we need, so add some.
type renderFS interface {
	fs.StatFS
	RemoveAll(string) error
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

	return nil
}

// We can't use os.DirFS or StatFS because they lack some methods we need. So we
// implement our own filesystem interface.
type realFS struct{}

func (r *realFS) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name) //nolint:wrapcheck
}

func (r *realFS) Open(name string) (fs.File, error) {
	return os.Open(name) //nolint:wrapcheck
}

func (r *realFS) RemoveAll(name string) error {
	return os.RemoveAll(name) //nolint:wrapcheck
}

func (r *Render) Run(ctx context.Context, args []string) error {
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

	if r.logger == nil {
		r.logger = logging.NewFromEnv("ABC_")
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("os.Getwd(): %w", err)
	}

	return r.realRun(ctx, &runParams{
		fs:           fSys,
		getter:       gg,
		cwd:          wd,
		tempDirNamer: tempDirName,
	})
}

type runParams struct {
	fs     renderFS
	getter getterClient
	cwd    string

	// Constructs a name for a temp directory that doesn't exist yet and won't
	// collide with other directories. Doesn't actually create a directory, the
	// caller does that. This accommodates quirky behavior of go-getter that
	// doesn't want the destination directory to exist already.
	tempDirNamer func(debugID string) (string, error)
}

// realRun is for testability; it's Run() with fakeable interfaces.
func (r *Render) realRun(ctx context.Context, rp *runParams) (outErr error) {
	templateDir, err := r.copyTemplate(ctx, rp)
	if templateDir != "" { // templateDir might be set even if there's an error
		defer func() {
			outErr = errors.Join(outErr, r.maybeRemoveTempDirs(rp.fs, templateDir))
		}()
	}
	if err != nil {
		return err
	}

	_ = templateDir // TODO: add template rendering logic

	return nil
}

// Downloads the template and returns the name of the temp directory where it
// was saved. If error is returned, then the returned directory name may or may
// not exist, and may or may not be empty.
func (r *Render) copyTemplate(ctx context.Context, rp *runParams) (string, error) {
	templateDir, err := rp.tempDirNamer("template-copy")
	if err != nil {
		return "", err
	}
	req := &getter.Request{
		Src:     r.source,
		Dst:     templateDir,
		Pwd:     rp.cwd,
		GetMode: getter.ModeAny,
	}

	res, err := rp.getter.Get(ctx, req)
	if err != nil {
		return templateDir, fmt.Errorf("go-getter.Get(): %w", err)
	}

	logger.Debugf("copied source template %q into temporary directory %q", r.source, res.Dst)
	return templateDir, nil
}

// Calls RemoveAll on each temp directory. A nonexistent directory is not an error.
func (r *Render) maybeRemoveTempDirs(fs renderFS, tempDirs ...string) error {
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

// Generate the name for a temporary directory, without creating it. debugID is
// an optional name that can be included to help understand which directory is
// which during debugging.
//
// We can't use os.MkdirTemp() for a go-getter output directory because
// go-getter silently fails to clone a git repo into an existing (empty)
// directory. go-getter assumes that the dir must already be a git repo if it
// exists.
func tempDirName(debugID string) (string, error) {
	rnd, err := randU64()
	if err != nil {
		return "", err
	}
	basename := fmt.Sprintf("abc-%s-%d", debugID, rnd)
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
