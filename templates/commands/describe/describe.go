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

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/specutil"
	"github.com/abcxyz/abc/templates/common/templatesource"
	"github.com/abcxyz/abc/templates/model/decode"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta1"
	"github.com/abcxyz/pkg/cli"
)

type Command struct {
	cli.BaseCommand
	flags DescribeFlags

	testFS common.FS
}

// Desc implements cli.Command.
func (c *Command) Desc() string {
	return "show the description and inputs of a given template."
}

func (c *Command) Help() string {
	return `
Usage: {{ COMMAND }} [options] <source>

The {{ COMMAND }} command describe the given template.

The "<source>" is the location of the template to be instantiated. Many forms
are accepted:

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

	specPath := filepath.Join(templateDir, specutil.SpecFileName)
	f, err := rp.fs.Open(specPath)
	if err != nil {
		return fmt.Errorf("error opening template spec: ReadFile(): %w", err)
	}
	defer f.Close()

	specI, err := decode.DecodeValidateUpgrade(ctx, f, specutil.SpecFileName, decode.KindTemplate)
	if err != nil {
		return fmt.Errorf("error reading template spec file: %w", err)
	}

	spec, ok := specI.(*spec.Spec)
	if !ok {
		return fmt.Errorf("internal error: spec file did not decode to spec.Spec")
	}

	specutil.FormatAttrList(c.Stdout(), c.specFieldsForDescribe(spec))
	return nil
}

// specFieldsForDescribe get Description and Inputs fields for spec.
func (c *Command) specFieldsForDescribe(spec *spec.Spec) [][]string {
	l := make([][]string, 0)
	l = append(l, specutil.SpecDescriptionForDescribe(spec)...)
	l = append(l, specutil.AllSpecInputVarForDescribe(spec)...)
	return l
}