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

package render

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/abcxyz/pkg/cli"
)

type Command struct {
	cli.BaseCommand

	template        string
	spec            string
	dest            string
	gitProtocol     string
	logLevel        string
	allowNonGitDest bool
	forceOverwrite  bool
	keepTempDirs    bool
	inputs          map[string]string
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
		Target:  &c.template,
		Usage: `the location of the template to be instantiated. Many forms are accepted. ` +
			`"helloworld@v1" means "github.com/abcxyz/helloworld repo at revision v1; this is for a template owned by abcxyz. ` +
			`"myorg/helloworld@v1" means github.com/myorg/helloworld repo at revision v1; this is for a template not owned by abcxyz but still on GitHub. ` +
			`"mygithost.com/mygitrepo/helloworld@v1" is for a template in a remote git repo but not owned by abcxyz and not on GitHub. ` +
			`"mylocaltarball.tgz" is for a template not in git but present on the local filesystem. ` +
			`"http://example.com/myremotetarball.tgz" os for a non-Git template in a remote tarball.`,
	})
	sect.StringVar(&cli.StringVar{
		Name:    "spec",
		Aliases: []string{"s"},
		Example: "path/to/spec.yaml",
		Target:  &c.spec,
		Default: "./spec.yaml",
		Usage:   "the path of the .yaml file within the template directory that specifies how the template is rendered.",
	})
	sect.StringVar(&cli.StringVar{
		Name:    "dest",
		Aliases: []string{"d"},
		Example: "/my/git/dir",
		Target:  &c.dest,
		Default: ".",
		Usage:   "the target directory in which to write the output files.",
	})
	sect.StringVar(&cli.StringVar{
		Name:    "git-protocol",
		Aliases: []string{"p"},
		Example: "https",
		Target:  &c.gitProtocol,
		Usage:   "either ssh or https, the protocol for connecting to GitHub.",
	})
	sect.StringMapVar(&cli.StringMapVar{
		Name:    "input",
		Aliases: []string{"i"},
		Example: "foo=bar",
		Target:  &c.inputs,
		Usage:   "key=val pairs of template values; may be repeated.",
	})
	sect.StringVar(&cli.StringVar{
		Name:    "log-level",
		Aliases: []string{"l"},
		Example: "info",
		Default: "warning",
		Target:  &c.logLevel,
		Usage:   "how verbose to log; any of debug|info|warning|error.",
	})
	sect.BoolVar(&cli.BoolVar{
		Name:    "allow-non-git-dest",
		Aliases: []string{"a"},
		Target:  &c.allowNonGitDest,
		Usage:   "skip the sanity checks that the dest directory is a git repo.",
	})
	sect.BoolVar(&cli.BoolVar{
		Name:    "force-overwrite",
		Aliases: []string{"o"},
		Target:  &c.forceOverwrite,
		Usage:   "if an output file already exists in the destination, overwrite it instead of failing.",
	})
	sect.BoolVar(&cli.BoolVar{
		Name:    "keep-temp-dirs-on-failure",
		Aliases: []string{"k"},
		Target:  &c.keepTempDirs,
		Usage:   "if there is an error, preserve the temp directories instead of deleting them normally.",
	})

	return set
}

func (c *Command) parseFlags(args []string) error {
	if err := c.Flags().Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	if c.template == "" {
		return fmt.Errorf("-template is required")
	}
	if c.dest == "" {
		return fmt.Errorf("-dest is required")
	}

	if c.allowNonGitDest {
		ok, err := isGitRepo(c.dest)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("the provided -dest dir is not a git repo; if this is intentional, use --allow-non-git-dest")
		}
	}

	if c.template == "" {
		return fmt.Errorf("-template is required")
	}

	return nil
}

func (c *Command) Run(ctx context.Context, args []string) error {
	if err := c.parseFlags(args); err != nil {
		return err
	}
	return fmt.Errorf("stub")
}

func isGitRepo(dir string) (bool, error) {
	path := filepath.Join(dir, ".git")
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("os.Stat(%s): %w", path, err)
}
