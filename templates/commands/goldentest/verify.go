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

	"github.com/fatih/color"
	"github.com/sergi/go-diff/diffmatchpatch"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/pkg/cli"
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
		return fmt.Errorf("failed to parse golden tests: %w", err)
	}

	// Create a temporary directory to render golden tests
	tempDir, err := renderTestCases(testCases, c.flags.Location)
	defer os.RemoveAll(tempDir)
	if err != nil {
		return fmt.Errorf("failed to render test cases: %w", err)
	}

	var merr error
	// Highlight error message color, given diff text might be hundreds lines long.
	red := color.New(color.FgRed).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	resultReport := "\nTest Report:\n"

	for _, tc := range testCases {
		goldenDataDir := filepath.Join(c.flags.Location, goldenTestDir, tc.TestName, testDataDir)
		tempDataDir := filepath.Join(tempDir, goldenTestDir, tc.TestName, testDataDir)

		fileSet := make(map[string]struct{})
		if err := addTestFile(&fileSet, goldenDataDir); err != nil {
			return err
		}
		if err := addTestFile(&fileSet, tempDataDir); err != nil {
			return err
		}

		dmp := diffmatchpatch.New()

		var tcErr error
		for relPath := range fileSet {
			goldenFile := filepath.Join(goldenDataDir, relPath)
			tempFile := filepath.Join(tempDataDir, relPath)

			goldenContent, err := os.ReadFile(goldenFile)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					failureText := red(fmt.Sprintf("-- [%s] generated, however not recorded in test data", goldenFile))
					err := fmt.Errorf(failureText)
					tcErr = errors.Join(tcErr, err)
					continue
				} else {
					return fmt.Errorf("failed to read (%s): %w", goldenFile, err)
				}
			}

			tempContent, err := os.ReadFile(tempFile)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					failureText := red(fmt.Sprintf("-- [%s] expected, however missing", goldenFile))
					err := fmt.Errorf(failureText)
					tcErr = errors.Join(tcErr, err)
					continue
				} else {
					return fmt.Errorf("failed to read (%s): %w", tempFile, err)
				}
			}

			diffs := dmp.DiffMain(string(tempContent), string(goldenContent), false)

			if hasDiff(diffs) {
				failureText := red(fmt.Sprintf("-- [%s] file content mismatch", goldenFile))
				err := fmt.Errorf("%s:\n%s", failureText, dmp.DiffPrettyText(diffs))
				tcErr = errors.Join(tcErr, err)
			}
		}

		if tcErr != nil {
			result := red(fmt.Sprintf("[x] golden test %s fails", tc.TestName))
			tcErr := fmt.Errorf("%s:\n %w", result, tcErr)
			merr = errors.Join(merr, tcErr)
			resultReport += result
		} else {
			resultReport += green(fmt.Sprintf("[✓] golden test %s succeeds", tc.TestName))
		}

		resultReport += "\n"
	}

	// Print test result report.
	fmt.Println(resultReport)

	if merr != nil {
		return fmt.Errorf("golden test verification failure:\n %w", merr)
	}

	return nil
}

// addTestFile collects file paths generated in a golden test.
func addTestFile(fileSet *map[string]struct{}, testDataDir string) error {
	err := fs.WalkDir(&common.RealFS{}, testDataDir, func(path string, de fs.DirEntry, err error) error {
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
	if err != nil {
		return fmt.Errorf("fs.WalkDir: %w", err)
	}
	return nil
}

// hasDiff returns whether file content mismatch exits.
func hasDiff(diffs []diffmatchpatch.Diff) bool {
	for _, diff := range diffs {
		if diff.Type != diffmatchpatch.DiffEqual {
			return true
		}
	}
	return false
}
