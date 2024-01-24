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

// Package goldentest implements golden test related subcommands.
package goldentest

// This file implements the "templates golden-test new-test" subcommand.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/input"
	"github.com/abcxyz/abc/templates/common/specutil"
	"github.com/abcxyz/abc/templates/model/decode"
	goldentest "github.com/abcxyz/abc/templates/model/goldentest/v1beta3"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
)

type NewTestCommand struct {
	cli.BaseCommand

	flags NewTestFlags
}

func (c *NewTestCommand) Desc() string {
	return "create a new golden test"
}

func (c *NewTestCommand) Help() string {
	return `
Usage: {{ COMMAND }} [options] <test_name>

The {{ COMMAND }} create a new golden test.

The "<test_name>" is the name of the test.
`
}

func (c *NewTestCommand) Flags() *cli.FlagSet {
	set := c.NewFlagSet()
	c.flags.Register(set)
	return set
}

func (c *NewTestCommand) Run(ctx context.Context, args []string) (rErr error) {
	logger := logging.FromContext(ctx)

	if err := c.Flags().Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "abc-new-test-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer func() {
		rErr = errors.Join(rErr, os.RemoveAll(tempDir))
	}()

	fs := &common.RealFS{}

	if err != nil {
		return err //nolint:wrapcheck
	}

	spec, err := specutil.Load(ctx, fs, c.flags.Location, c.flags.Location)
	if err != nil {
		return err //nolint:wrapcheck
	}
	logger.DebugContext(ctx, "resolving inputs")

	resolvedInputs, err := input.Resolve(ctx, &input.ResolveParams{
		FS:       fs,
		Inputs:   c.flags.Inputs,
		Prompt:   c.flags.Prompt,
		Prompter: c,
		Spec:     spec,
	})
	if err != nil {
		return err //nolint:wrapcheck
	}

	buf, err := marshalTestCase(resolvedInputs)
	if err != nil {
		return fmt.Errorf("failed to marshal test config data: %w", err)
	}

	testDir := filepath.Join(c.flags.Location, goldenTestDir, c.flags.NewTestName)
	testConfigFile := filepath.Join(testDir, configName)

	err = fs.MkdirAll(testDir, common.OwnerRWXPerms)
	if err != nil {
		return fmt.Errorf("failed creating %s directory to contain test yaml file: %w", testDir, err)
	}
	// file overwriting is not allowed.
	fh, err := fs.OpenFile(testConfigFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, common.OwnerRWPerms)
	if err != nil {
		return fmt.Errorf("can't open file(%q): %w", testConfigFile, err)
	}
	defer func() {
		rErr = errors.Join(rErr, fh.Close())
	}()
	if _, err := fh.Write(buf); err != nil {
		return fmt.Errorf("write(%q): %w", testConfigFile, err)
	}

	return nil
}

func marshalTestCase(resolvedInputs map[string]string) ([]byte, error) {
	testCase := goldentest.Test{
		Inputs: mapToVarValues(resolvedInputs),
	}
	buf, err := yaml.Marshal(testCase)
	if err != nil {
		return nil, fmt.Errorf("failed marshaling test case when writing: %w", err)
	}
	header := map[string]string{
		"api_version": decode.LatestAPIVersion,
		"kind":        decode.KindGoldenTest,
	}
	headerYAML, err := yaml.Marshal(header)
	if err != nil {
		return nil, fmt.Errorf("failed marshaling api_version: %w", err)
	}

	buf = append(headerYAML,
		buf...)
	return buf, nil
}