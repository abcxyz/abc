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

// This file implements the "templates golden-test recordTestCases" subcommand.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/tempdir"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
)

type RecordCommand struct {
	flags Flags

	cli.BaseCommand
}

func (c *RecordCommand) Desc() string {
	return "record the template rendering result to golden tests " +
		"(capture the anticipated outcome akin to expected output in unit test)"
}

func (c *RecordCommand) Help() string {
	return `
Usage: {{ COMMAND }} [--test-name=<test-name-1>,<test-name-2>] [<location>]

The {{ COMMAND }} records the template golden tests (capture the
anticipated outcome akin to expected output in unit test).

The "<test_name>" is the name of the test. If no <test_name> is specified,
all tests will be recorded.

The "<location>" is the location of the templates. 
If no "<location>" is given, default to current directory.

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

func (c *RecordCommand) Run(ctx context.Context, args []string) (rErr error) {
	if err := c.Flags().Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	templateLocations, err := crawlTemplateLocations(c.flags.Location)
	if err != nil {
		return fmt.Errorf("failed to crawl template locations [%s]: %w", c.flags.Location, err)
	}
	var merr error
	for _, templateLocation := range templateLocations {
		merr = errors.Join(merr, recordTestCases(ctx, templateLocation, c.flags.TestNames))
	}
	return merr
}

func recordTestCases(ctx context.Context, templateLocation string, testNames []string) (rErr error) {
	logger := logging.FromContext(ctx)
	logger.InfoContext(ctx, "recording test for template location", "template_location", templateLocation)
	testCases, err := parseTestCases(ctx, templateLocation, testNames)
	if err != nil {
		return fmt.Errorf("failed to parse golden test for template location %v: %w", templateLocation, err)
	}

	rfs := &common.RealFS{}

	tempTracker := tempdir.NewDirTracker(rfs, false)
	defer tempTracker.DeferMaybeRemoveAll(ctx, &rErr)

	// Create a temporary directory to validate golden tests rendered with no
	// error. If any test fails, no data should be written to file system
	// for atomicity purpose.
	tempDir, err := renderTestCases(ctx, testCases, templateLocation)
	if err != nil {
		return fmt.Errorf("failed to render test cases: %w", err)
	}
	tempTracker.Track(tempDir)

	// Recursively copy files from tempDir to template golden test directory.
	for _, tc := range testCases {
		if err := recordTestCase(ctx, templateLocation, tc, tempDir, rfs); err != nil {
			rErr = errors.Join(rErr, fmt.Errorf("failed to record test case [%s] for template location [%s]: %w", tc.TestName, templateLocation, err))
		}
	}
	return rErr
}

func recordTestCase(ctx context.Context, templateLocation string, tc *TestCase, tempDir string, rfs *common.RealFS) error {
	logger := logging.FromContext(ctx)
	logger.InfoContext(ctx, "recording test for test name", "testname", tc.TestName)
	testDir := filepath.Join(templateLocation, goldenTestDir, tc.TestName, testDataDir)
	if err := os.RemoveAll(testDir); err != nil {
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
		DstRoot: testDir,
		SrcRoot: filepath.Join(tempDir, goldenTestDir, tc.TestName, testDataDir),
		FS:      rfs,
		Visitor: visitor,
	}
	if err := common.CopyRecursive(ctx, nil, params); err != nil {
		return fmt.Errorf("failed to copy recursive: %w", err)
	}

	abcInternal := filepath.Join(testDir, common.ABCInternalDir)
	if err := os.MkdirAll(abcInternal, common.OwnerRWXPerms); err != nil {
		return fmt.Errorf("failed to create dir %q: %w", abcInternal, err)
	}

	if !tc.TestConfig.Features.SkipABCRenamed {
		if err := renameGitDirsAndFiles(testDir); err != nil {
			return fmt.Errorf("failed renaming git related dirs and files for test case %q: %w", tc.TestName, err)
		}
	}

	// git won't commit an empty directory, so add a placeholder file.
	gitKeep := filepath.Join(abcInternal, ".gitkeep")
	if err := os.WriteFile(gitKeep, []byte{}, common.OwnerRWPerms); err != nil {
		return fmt.Errorf("failed creating %q: %w", gitKeep, err)
	}
	return nil
}
