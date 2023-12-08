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

// Package goldentest implements golden test related subcommands.
package goldentest

// This file implements the "templates golden-test record" subcommand.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
)

type RecordCommand struct {
	flags Flags

	cli.BaseCommand
}

func (c *RecordCommand) Desc() string {
	return "record the template rendering result to golden tests"
}

func (c *RecordCommand) Help() string {
	return `
Usage: {{ COMMAND }} [options] <test_name>

The {{ COMMAND }} records the template golden tests.

The "<test_name>" is the name of the test. If no <test_name> is specified,
all tests will be recoreded.

For every test case, it is expected that
  - a testdata/golden/<test_name> folder exists to host test results.
  - a testdata/golden/<test_name>/test.yaml exists to define
template input params.`
}

func (c *RecordCommand) Flags() *cli.FlagSet {
	set := c.NewFlagSet()
	c.flags.Register(set)
	return set
}

func (c *RecordCommand) Run(ctx context.Context, args []string) error {
	if err := c.Flags().Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	testCases, err := parseTestCases(ctx, c.flags.Location, c.flags.TestName)
	if err != nil {
		return fmt.Errorf("failed to parse golden test: %w", err)
	}

	// Create a temporary directory to validate golden tests rendered with no
	// error. If any test fails, no data should be written to file system
	// for atomicity purpose.
	tempDir, err := os.MkdirTemp("", "abc-test-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	var merr error
	for _, tc := range testCases {
		merr = errors.Join(merr, renderTestCase(c.flags.Location, tempDir, tc))
	}
	if merr != nil {
		return fmt.Errorf("failed to render golden tests: %w", merr)
	}

	logger := logging.FromContext(ctx)

	// Recursively copy files from tempDir to template golden test directory.
	for _, tc := range testCases {
		testDir := filepath.Join(c.flags.Location, goldenTestDir, tc.TestName)
		if err := clearTestDir(testDir); err != nil {
			return fmt.Errorf("failed to clear test directory: %w", err)
		}

		visitor := func(relToAbsSrc string, de fs.DirEntry) (common.CopyHint, error) {
			if !de.IsDir() {
				logger.InfoContext(ctx, "recording",
					"testname", tc.TestName,
					"testdata", relToAbsSrc)
			}
			return common.CopyHint{
				Overwrite: true,
			}, nil
		}
		params := &common.CopyParams{
			DstRoot: filepath.Join(testDir, testDataDir),
			SrcRoot: filepath.Join(tempDir, goldenTestDir, tc.TestName, testDataDir),
			RFS:     &common.RealFS{},
			Visitor: visitor,
		}
		merr = errors.Join(merr, common.CopyRecursive(ctx, nil, params))
	}
	if merr != nil {
		return fmt.Errorf("failed to write golden test data: %w", merr)
	}

	return nil
}
