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
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/run"
	"github.com/abcxyz/abc/templates/common/tempdir"
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
Usage: {{ COMMAND }} [--test-name=<test-name-1>,<test-name-2>] [<location>]

The {{ COMMAND }} verifies the template golden tests.

The "<test_name>" is the name of the test. If no <test_name> is specified,
all tests will be run against.

The "<location>" is the location of the template.
If no "<location>" is given, default to current directory.

For every test case, it is expected that
  - a testdata/golden/<test_name> folder exists to host test results.
  - a testdata/golden/<test_name>/test.yaml exists to define
template input params.`
}

func (c *VerifyCommand) Flags() *cli.FlagSet {
	set := c.NewFlagSet()
	c.flags.Register(set)
	return set
}

func (c *VerifyCommand) Run(ctx context.Context, args []string) (rErr error) {
	if err := c.Flags().Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	testCases, err := parseTestCases(ctx, c.flags.Location, c.flags.TestNames)
	if err != nil {
		return fmt.Errorf("failed to parse golden tests: %w", err)
	}

	fs := &common.RealFS{}

	tempTracker := tempdir.NewDirTracker(fs, false)
	defer tempTracker.DeferMaybeRemoveAll(ctx, &rErr)

	// Create a temporary directory to render golden tests
	tempDir, err := renderTestCases(ctx, testCases, c.flags.Location)
	if err != nil {
		return fmt.Errorf("failed to render test cases: %w", err)
	}
	tempTracker.Track(tempDir)

	var merr error

	// Highlight error message color, given diff text might be hundreds lines long.
	// Only color the text when the result is to displayed at a terminal
	var red, green func(a ...interface{}) string
	useColor := c.Stdout() == os.Stdout && isatty.IsTerminal(os.Stdout.Fd())
	if useColor {
		red = color.New(color.FgRed).SprintFunc()
		green = color.New(color.FgGreen).SprintFunc()
	} else {
		red = fmt.Sprint
		green = fmt.Sprint
	}

	resultReport := "\nTest Report:\n"

	for _, tc := range testCases {
		goldenDataDir := filepath.Join(c.flags.Location, goldenTestDir, tc.TestName, testDataDir)
		tempDataDir := filepath.Join(tempDir, goldenTestDir, tc.TestName, testDataDir)

		if !tc.TestConfig.Features.SkipABCRenamed {
			if err := renameGitDirsAndFiles(tempDataDir); err != nil {
				return fmt.Errorf("failed renaming git related dirs and files for test case %s: %w", tc.TestName, err)
			}
		}

		fileSet := make(map[string]struct{})

		_, err = os.Stat(goldenDataDir)
		if err != nil {
			if common.IsStatNotExistErr(err) {
				return fmt.Errorf("no recorded test data in %q, "+
					"please run `record` command to record the template rendering result to golden tests", goldenDataDir)
			}
			return fmt.Errorf("failed to stat %q, %w", goldenDataDir, err)
		}

		if err := addTestFiles(fileSet, goldenDataDir); err != nil {
			return err
		}
		if err := addTestFiles(fileSet, tempDataDir); err != nil {
			return err
		}

		// Sort the relPaths in alphebetical order.
		relPaths := make([]string, 0, len(fileSet))
		for k := range fileSet {
			relPaths = append(relPaths, k)
		}
		sort.Strings(relPaths)

		var tcErr error
		outputMismatch := false
		for _, relPath := range relPaths {
			goldenFile := filepath.Join(goldenDataDir, relPath)
			tempFile := filepath.Join(tempDataDir, relPath)
			abcRenameTrimmedGoldenFile := strings.TrimSuffix(goldenFile, abcRenameSuffix)
			abcRenameTrimmedTempFile := strings.TrimSuffix(tempFile, abcRenameSuffix)

			if _, err := os.Stat(goldenFile); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					failureText := red(fmt.Sprintf("-- [%s] generated, however not recorded in test data", abcRenameTrimmedGoldenFile))
					err := fmt.Errorf(failureText)
					tcErr = errors.Join(tcErr, err)
					outputMismatch = true
					continue
				}
				return fmt.Errorf("failed to read (%s): %w", abcRenameTrimmedGoldenFile, err)
			}

			if _, err := os.Stat(tempFile); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					failureText := red(fmt.Sprintf("-- [%s] expected, however missing", abcRenameTrimmedGoldenFile))
					err := fmt.Errorf(failureText)
					tcErr = errors.Join(tcErr, err)
					continue
				}
				return fmt.Errorf("failed to read (%s): %w", abcRenameTrimmedTempFile, err)
			}

			diff, err := run.RunDiff(ctx, useColor, goldenFile, goldenDataDir, tempFile, tempDataDir)
			if err != nil {
				return fmt.Errorf("error diffing actual vs expected output: %w", err)
			}

			if len(diff) > 0 {
				failureText := red(fmt.Sprintf("-- [%s] file content mismatch", abcRenameTrimmedGoldenFile))
				err := fmt.Errorf("%s:\n%s", failureText, diff)
				tcErr = errors.Join(tcErr, err)
				outputMismatch = true
			}
		}

		// verify stdout only when SkipStdout flag is set to false.
		if !tc.TestConfig.Features.SkipStdout {
			goldenStdoutFile := filepath.Join(goldenDataDir, common.ABCInternalDir, common.ABCInternalStdout)
			tempStdoutFile := filepath.Join(tempDataDir, common.ABCInternalDir, common.ABCInternalStdout)

			stdoutDiff, err := run.RunDiff(ctx, useColor, goldenStdoutFile, goldenDataDir, tempStdoutFile, tempDataDir)
			if err != nil {
				return fmt.Errorf("failed to compare stdout:%w", err)
			}
			if len(stdoutDiff) > 0 {
				failureText := red("the printed messages differ between the recorded golden output and the actual output")
				err := fmt.Errorf("%s:\n%s", failureText, stdoutDiff)
				tcErr = errors.Join(tcErr, err)
				outputMismatch = true
			}
		}

		if outputMismatch {
			failureText := red(fmt.Sprintf("golden test [%s] didn't match actual output, you might "+
				"need to run 'record' command to capture it as the new expected output", tc.TestName))
			err := fmt.Errorf(failureText)
			tcErr = errors.Join(tcErr, err)
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

// addTestFiles collects file paths generated in a golden test.
func addTestFiles(fileSet map[string]struct{}, testDataDir string) error {
	err := fs.WalkDir(&common.RealFS{}, testDataDir, func(path string, de fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("fs.WalkDir(%s): %w", path, err)
		}

		relToSrc, err := filepath.Rel(testDataDir, path)
		if err != nil {
			return fmt.Errorf("filepath.Rel(%s,%s): %w", testDataDir, path, err)
		}

		// Don't assert the contents of ".abc". As of this writing, the .abc
		// dir contains things that are specific to recorded tests and not part
		// of the expected template output.
		if common.IsReservedInDest(relToSrc) {
			if de.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if de.IsDir() {
			return nil
		}

		fileSet[relToSrc] = struct{}{}
		return nil
	})
	if err != nil {
		return fmt.Errorf("fs.WalkDir: %w", err)
	}
	return nil
}
