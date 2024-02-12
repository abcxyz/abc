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
	"github.com/abcxyz/abc/templates/common/builtinvar"
	"github.com/abcxyz/abc/templates/common/input"
	"github.com/abcxyz/abc/templates/common/specutil"
	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/abc/templates/model/decode"
	goldentest "github.com/abcxyz/abc/templates/model/goldentest/v1beta3"
	"github.com/abcxyz/abc/templates/model/header"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
)

type NewTestCommand struct {
	cli.BaseCommand

	flags NewTestFlags

	// used in prompt UT.
	skipPromptTTYCheck bool
}

func (c *NewTestCommand) Desc() string {
	return "create a new golden test"
}

func (c *NewTestCommand) Help() string {
	return `
Usage: {{ COMMAND }} [options] <test_name> [<location>]

The {{ COMMAND }} create a new golden test.

The "<test_name>" is the name of the test.
The "<location>" is the location of the template. 
If no "<location>" is given, default to current directory.
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
	fs := &common.RealFS{}

	spec, err := specutil.Load(ctx, fs, c.flags.Location, c.flags.Location)
	if err != nil {
		return err //nolint:wrapcheck
	}
	logger.DebugContext(ctx, "resolving inputs")

	resolvedInputs, err := input.Resolve(ctx, &input.ResolveParams{
		FS:                 fs,
		Inputs:             c.flags.Inputs,
		Prompt:             c.flags.Prompt,
		Prompter:           c,
		Spec:               spec,
		SkipPromptTTYCheck: c.skipPromptTTYCheck,
	})
	if err != nil {
		return err //nolint:wrapcheck
	}

	builtinVarsKeys := make([]string, 0, len(c.flags.BuiltinVars))
	for k := range c.flags.BuiltinVars {
		builtinVarsKeys = append(builtinVarsKeys, k)
	}
	if err = builtinvar.Validate(spec.Features, builtinVarsKeys); err != nil {
		return err //nolint:wrapcheck
	}

	buf, err := marshalTestCase(resolvedInputs, c.flags.BuiltinVars)
	if err != nil {
		return fmt.Errorf("failed to marshal test config data: %w", err)
	}

	testDir := filepath.Join(c.flags.Location, goldenTestDir, c.flags.NewTestName)
	testConfigFile := filepath.Join(testDir, configName)

	if err = fs.MkdirAll(testDir, common.OwnerRWXPerms); err != nil {
		return fmt.Errorf("failed creating %s directory to contain test yaml file: %w", testDir, err)
	}
	// file overwriting is not allowed.
	fileFlag := os.O_CREATE | os.O_EXCL | os.O_WRONLY
	if c.flags.ForceOverwrite {
		// file overwriting is allowed.
		fileFlag = os.O_CREATE | os.O_TRUNC | os.O_WRONLY
	}
	fh, err := fs.OpenFile(testConfigFile, fileFlag, common.OwnerRWPerms)
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

func marshalTestCase(inputs, builtinVars map[string]string) ([]byte, error) {
	testCase := &goldentest.WithHeader{
		Header: &header.Fields{
			NewStyleAPIVersion: model.String{Val: decode.LatestAPIVersion},
			Kind:               model.String{Val: decode.KindGoldenTest},
		},
		Wrapped: &goldentest.ForMarshaling{
			Inputs:      mapToVarValues(inputs),
			BuiltinVars: mapToVarValues(builtinVars),
		},
	}
	buf, err := yaml.Marshal(testCase)
	if err != nil {
		return nil, fmt.Errorf("failed marshaling test case when writing: %w", err)
	}

	return buf, nil
}
