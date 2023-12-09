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

// This file implements the "templates golden-test verify" subcommand.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
	"github.com/sergi/go-diff/diffmatchpatch"
)

type VerifyCommand struct {
	flags Flags

	cli.BaseCommand
}

func (c *VerifyCommand) Desc() string {
	return "verify the template rendering result against golden tests"
}

func (c *VerifyCommand) Help() string {
	return `
Usage: {{ COMMAND }} [options] <test_name>

The {{ COMMAND }} verify the template golden test.

The "<test_name>" is the name of the test. If no <test_name> is specified,
all tests will be run against.

For every test case, it is expected that
  - a testdata/golden/<test_name> folder exists to host test results.
  - a testdata/golden/<test_name>/inputs.yaml exists to define
template input params.`
}

func (c *VerifyCommand) Flags() *cli.FlagSet {
	set := c.NewFlagSet()
	c.flags.Register(set)
	return set
}

func (c *VerifyCommand) Run(ctx context.Context, args []string) error {
	if err := c.Flags().Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	testCases, err := parseTestCases(ctx, c.flags.Location, c.flags.TestName)
	if err != nil {
		return fmt.Errorf("failed to parse golden test: %w", err)
	}

	// Create a temporary directory to render golden tests
	tempDir, err := renderTestCases(testCases, c.flags.Location)
	defer os.RemoveAll(tempDir)
	if err != nil {
		return fmt.Errorf("failed to render test cases: %w", err)
	}

	var merr error
	logger := logging.FromContext(ctx)

	for _, tc := range testCases {
		goldenDataDir := filepath.Join(c.flags.Location, goldenTestDir, tc.TestName, testDataDir)
		tempDataDir := filepath.Join(tempDir, goldenTestDir, tc.TestName, testDataDir)

		fileSet := make(map[string]struct{})
		addTestFile(&fileSet, goldenDataDir)
		addTestFile(&fileSet, tempDataDir)

		dmp := diffmatchpatch.New()

		for relPath := range fileSet {
			goldenFile := filepath.Join(goldenDataDir, relPath)
			tempFile := filepath.Join(tempDataDir, relPath)

			goldenContent, err := os.ReadFile(goldenFile)
			if err != nil {
				fmt.Printf("failed to read (%s):", goldenFile, err)
				continue
			}

			tempContent, err := os.ReadFile(tempFile)
			if err != nil {
				fmt.Printf("failed to read (%s):", tempFile, err)
				continue
			}

			diff := dmp.DiffMain(string(goldenContent), string(tempContent), false)
			diffString := dmp.DiffPrettyText(diff)

			if strings.TrimSpace(diffString) != "" {
				merr = errors.Join(merr, fmt.Errorf("golden test failed in %s:\n%s\n", relPath, diffString))
			} else {
				logger.InfoContext(ctx, "verified golden test", "testname", tc.TestName)
			}
		}
	}
	if merr != nil {
		return fmt.Errorf("failed to verify golden test: %w", merr)
	}

	logger.InfoContext(ctx, "golden tests verification succeed")
	return nil
}

func addTestFile(fileSet *map[string]struct{}, testDataDir string) error {
	fs.WalkDir(&common.RealFS{}, testDataDir, func(path string, de fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("fs.WalkDir(%s): %w", path, err)
		}
		if de.IsDir() {
			return nil
		}

		relToSrc, err := filepath.Rel(testDataDir, path)
		if err != nil {
			return fmt.Errorf("filepath.Rel(%s,%s): %w", testDataDir, path, err)
		}
		(*fileSet)[relToSrc] = struct{}{}
		return nil
	})
	return nil
}
