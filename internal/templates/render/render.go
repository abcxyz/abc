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
package render

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/abcxyz/pkg/cli"
)

type Command struct {
	cli.BaseCommand

	fs fs

	fv flagValues
}

type flagValues struct {
	template       string
	spec           string
	dest           string
	gitProtocol    string
	logLevel       string
	forceOverwrite bool
	keepTempDirs   bool
	inputs         map[string]string
}

// Desc implements cli.Command.
func (c *Command) Desc() string {
	return "Instantiate a template to setup a new app or add config files"
}

func (c *Command) Help() string {
	return `
Usage: {{ COMMAND }} [options]

  The {{ COMMAND }} command renders the given template into the given dest
  directory.
`
}

func (c *Command) Flags() *cli.FlagSet {
	set := cli.NewFlagSet()

	sect := set.NewSection("Render options")
	sect.StringVar(&cli.StringVar{
		Name:    "template",
		Aliases: []string{"t"},
		Example: "helloworld@v1",
		Target:  &c.fv.template,
		Usage: `Required. The location of the template to be instantiated. Many forms are accepted. ` +
			`"helloworld@v1" means "github.com/abcxyz/helloworld repo at revision v1; this is for a template owned by abcxyz. ` +
			`"myorg/helloworld@v1" means github.com/myorg/helloworld repo at revision v1; this is for a template not owned by abcxyz but still on GitHub. ` +
			`"mygithost.com/mygitrepo/helloworld@v1" is for a template in a remote git repo but not owned by abcxyz and not on GitHub. ` +
			`"mylocaltarball.tgz" is for a template not in git but present on the local filesystem. ` +
			`"http://example.com/myremotetarball.tgz" os for a non-Git template in a remote tarball.`,
	})
	sect.StringVar(&cli.StringVar{
		Name:    "spec",
		Example: "path/to/spec.yaml",
		Target:  &c.fv.spec,
		Default: "./spec.yaml",
		Usage:   "The path of the .yaml file within the unpacked template directory that specifies how the template is rendered.",
	})
	sect.StringVar(&cli.StringVar{
		Name:    "dest",
		Aliases: []string{"d"},
		Example: "/my/git/dir",
		Target:  &c.fv.dest,
		Default: ".",
		Usage:   "Required. The target directory in which to write the output files.",
	})
	sect.StringVar(&cli.StringVar{
		Name:    "git-protocol",
		Example: "https",
		Default: "https",
		Target:  &c.fv.gitProtocol,
		Usage:   "Either ssh or https, the protocol for connecting to GitHub. Only used if the template source is GitHub.",
	})
	sect.StringMapVar(&cli.StringMapVar{
		Name:    "input",
		Example: "foo=bar",
		Target:  &c.fv.inputs,
		Usage:   "The key=val pairs of template values; may be repeated.",
	})
	sect.StringVar(&cli.StringVar{
		Name:    "log-level",
		Example: "info",
		Default: "warning",
		Target:  &c.fv.logLevel,
		Usage:   "How verbose to log; any of debug|info|warning|error.",
	})
	sect.BoolVar(&cli.BoolVar{
		Name:    "force-overwrite",
		Target:  &c.fv.forceOverwrite,
		Default: false,
		Usage:   "If an output file already exists in the destination, overwrite it instead of failing.",
	})
	sect.BoolVar(&cli.BoolVar{
		Name:    "keep-temp-dirs",
		Target:  &c.fv.keepTempDirs,
		Default: false,
		Usage:   "Preserve the temp directories instead of deleting them normally.",
	})

	return set
}

func (c *Command) parseFlags(args []string) error {
	if err := c.Flags().Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	if c.fv.template == "" {
		return fmt.Errorf("-template is required")
	}
	if c.fv.dest == "" {
		return fmt.Errorf("-dest is required")
	}

	return nil
}

func (c *Command) Run(ctx context.Context, args []string) error {
	if err := c.parseFlags(args); err != nil {
		return err
	}

	if c.fs == nil { // allow filesystem interaction to be faked for testing
		c.fs = &realFS{}
	}

	if err := destOK(c.fs, &c.fv); err != nil {
		return err
	}

	return fmt.Errorf("stub")
}

// destOK makes sure that the output directory looks sane; we don't want to clobber the user's
// home directory or something.
func destOK(fs fs, fv *flagValues) error {
	fi, err := fs.Stat(fv.dest)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("the destination directory doesn't exist: %q", fv.dest)
		}
		return fmt.Errorf("os.Stat(%s): %w", fv.dest, err)
	}

	if !fi.IsDir() {
		return fmt.Errorf("the destination %q is not a directory", fv.dest)
	}

	return nil
}

// An interface that allows for a fake filesystem to be used in tests.
type fs interface {
	Stat(string) (os.FileInfo, error)
}
