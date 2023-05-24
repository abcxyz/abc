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
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"

	"github.com/abcxyz/pkg/cli"
)

type Render struct {
	cli.BaseCommand

	testFS fs.StatFS

	source             string
	flagSpec           string
	flagDest           string
	flagGitProtocol    string
	flagLogLevel       string
	flagForceOverwrite bool
	flagKeepTempDirs   bool
	flagInputs         map[string]string
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

func (r *Render) Run(ctx context.Context, args []string) error {
	if err := r.parseFlags(args); err != nil {
		return err
	}

	useFS := r.testFS // allow filesystem interaction to be faked for testing
	if useFS == nil {
		useFS = os.DirFS("/").(fs.StatFS) //nolint:forcetypeassert // safe per docs: https://pkg.go.dev/os#DirFS
	}

	if err := destOK(useFS, r.flagDest); err != nil {
		return err
	}

	return fmt.Errorf("not implemented")
}

// destOK makes sure that the output directory looks sane; we don't want to clobber the user's
// home directory or something.
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
