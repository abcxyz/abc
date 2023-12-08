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

// Package describe implements the template describe related subcommands.
package describe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"text/tabwriter"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/templatesource"
	"github.com/abcxyz/pkg/cli"

	"github.com/abcxyz/abc/templates/model/decode"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta1"
)

type Command struct {
	cli.BaseCommand
	flags DescribeFlags

	testFS   common.FS
	testSpce *spec.Spec
}

const (
	// The spec file is always located in the template root dir and named spec.yaml.
	specName = "spec.yaml"
)

// Desc implements cli.Command
func (c *Command) Desc() string {
	return "describe a template base on it's spec.yaml"
}

func (c *Command) Help() string {
	return `
Usage: {{ COMMAND }} [options] <source>

The {{ COMMAND }} command describe the given template.

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

func (c *Command) Flags() *cli.FlagSet {
	set := c.NewFlagSet()
	c.flags.Register(set)
	return set
}

type runParams struct {
	fs     common.FS
	stdout io.Writer

	describeTempDirBase string
}

func (c *Command) Run(ctx context.Context, args []string) error {
	if err := c.Flags().Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}
	fSys := c.testFS
	if fSys == nil {
		fSys = &common.RealFS{}
	}
	return c.realRun(ctx, &runParams{
		fs:     fSys,
		stdout: c.Stdout(),
	})
}

// readRun provides fakable interface to test Run.
func (c *Command) realRun(ctx context.Context, rp *runParams) (rErr error) {
	var tempDirs []string
	defer func() {
		var rErr error
		for _, d := range tempDirs {
			rErr = errors.Join(rErr, rp.fs.RemoveAll(d))
		}
	}()
	_, templateDir, err := templatesource.Download(ctx, rp.fs, rp.describeTempDirBase, c.flags.Source, c.flags.GitProtocol)
	if templateDir != "" { // templateDir might be set even if there's an error
		tempDirs = append(tempDirs, templateDir)
	}
	if err != nil {
		return err //nolint:wrapcheck
	}

	specPath := filepath.Join(templateDir, specName)
	f, err := rp.fs.Open(specPath)
	if err != nil {
		return fmt.Errorf("error opening template spec: ReadFile(): %w", err)
	}
	defer f.Close()

	specI, err := decode.DecodeValidateUpgrade(ctx, f, specName, decode.KindTemplate)
	if err != nil {
		return fmt.Errorf("error reading template spec file: %w", err)
	}

	spec, ok := specI.(*spec.Spec)
	if !ok {
		return fmt.Errorf("internal error: spec file did not decode to spec.Spec")
	}
	tw := tabwriter.NewWriter(rp.stdout, 8, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "\nTemplate:\t%s", c.flags.Source)
	fmt.Fprintf(tw, "\nDescription:\t%s", spec.Desc.Val)
	fmt.Fprintf(tw, "\n")
	for _, i := range spec.Inputs {
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
		fmt.Fprintf(tw, "\n")
		tw.Flush()

	}
	return nil
}

// writeRule writes a human-readable description of the given rule to the given
// tabwriter in a 2-column format.
//
// Sometimes we run this in a context where we want to include the index of the
// rule in the list of rules; in that case, pass includeIndex=true and the index
// value. If includeIndex is false, then index is ignored.
func writeRule(tw *tabwriter.Writer, rule *spec.InputRule, includeIndex bool, index int) {
	indexStr := ""
	if includeIndex {
		indexStr = fmt.Sprintf(" %d", index)
	}

	fmt.Fprintf(tw, "\nRule%s:\t%s", indexStr, rule.Rule.Val)
	if rule.Message.Val != "" {
		fmt.Fprintf(tw, "\nRule%s msg:\t%s", indexStr, rule.Message.Val)
	}
}
