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
package upgrade

import (
	"context"
	"fmt"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/model/decode"
	manifest "github.com/abcxyz/abc/templates/model/manifest/v1alpha1"
	"github.com/abcxyz/pkg/cli"
)

// Command implements cli.Command for template upgrades.
type Command struct {
	cli.BaseCommand
	flags Flags

	testFS common.FS
}

// Desc implements cli.Command.
func (c *Command) Desc() string {
	return "apply a new template version to an already-rendered template output directory"
}

// Help implements cli.Command.
func (c *Command) Help() string {
	return `
Usage: {{ COMMAND }} [options] <manifest>

The {{ COMMAND }} command upgrades an already-rendered template output to use
the latest version of a template.

The "<manifest>" is the path to the *.lock.yaml file that was created when the
template was originally rendered.
`
}

// Hidden implements cli.Command.
func (c *Command) Hidden() bool {
	// TODO(#191): unhide the upgrade command when it's ready.
	return true
}

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

	return c.realRun(ctx, &runParams{
		fs: fSys,
	})
}

type runParams struct {
	fs common.FS
}

func (c *Command) realRun(ctx context.Context, rp *runParams) error {
	manifest, err := loadManifest(ctx, rp.fs, c.flags.Manifest)
	if err != nil {
		return err
	}
	_ = manifest

	// TODO(#191): implement the upgrade feature

	return nil
}

func loadManifest(ctx context.Context, fs common.FS, path string) (*manifest.Manifest, error) {
	f, err := fs.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open manifest file at %q: %w", path, err)
	}
	defer f.Close()

	manifestI, err := decode.DecodeValidateUpgrade(ctx, f, path, decode.KindManifest)
	if err != nil {
		return nil, fmt.Errorf("error reading manifest file: %w", err)
	}

	out, ok := manifestI.(*manifest.Manifest)
	if !ok {
		return nil, fmt.Errorf("internal error: manifest file did not decode to *manifest.Manifest")
	}

	return out, nil
}
