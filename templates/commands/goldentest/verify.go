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
	"github.com/abcxyz/abc-updater/pkg/metrics"
	"github.com/abcxyz/abc/internal/wrapper"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/run"
	"github.com/abcxyz/abc/templates/common/tempdir"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/workerpool"
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
	mClient := metrics.FromContext(ctx)
	cleanup := wrapper.WriteMetric(ctx, mClient, "command_goldentest_verify", 1)
	defer cleanup()

	if err := c.Flags().Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	absLocation, err := filepath.Abs(c.flags.Location)
	if err != nil {
		return err //nolint:wrapcheck
	}
	templateLocations, err := crawlTemplatesWithGoldenTests(absLocation)
	if err != nil {
		return fmt.Errorf("failed to crawl template locations: %w", err)
	}

	pool := workerpool.New[string](&workerpool.Config{
		// Concurrency is larger than the number of CPUs because the bottleneck
		// is not CPU cycles. We just want to do I/O concurrently. The constant
		// multiplier was chosen arbitrarily.
		Concurrency: int64(runtime.NumCPU()) * 4,
	})

	var merr error
	for _, templateLocation := range templateLocations {
		merr = errors.Join(merr, verify(ctx, pool, templateLocation, c.flags.TestNames, c.Stdout()))
	}

	results, err := pool.Done(ctx)
	if errors.Is(err, context.Canceled) {
		// Any other errors besides context.Canceled will be processed in the
		// following loop.
		return err //nolint:wrapcheck
	}

	var resultReport strings.Builder
	resultReport.WriteString("\n")
	for _, result := range results {
		if result.Error != nil {
			merr = errors.Join(merr, result.Error)
		}
		resultReport.WriteString(result.Value)
		resultReport.WriteString("\n")
	}

	fmt.Println(resultReport.String())

	if merr != nil {
		return fmt.Errorf("golden test verification failure:\n %w", merr)
	}

	return merr
}

func verify(ctx context.Context, pool *workerpool.Pool[string], templateLocation string, testNames []string, stdout io.Writer) error {
	logger := logging.FromContext(ctx)
	logger.InfoContext(ctx, "verifying test for template location", "template_location", templateLocation)
	testCases, err := parseTestCases(ctx, templateLocation, testNames)
	if err != nil {
		return fmt.Errorf("failed to parse golden tests: %w", err)
	}

	fs := &common.RealFS{}

	// Highlight error message color, given diff text might be hundreds lines long.
	// Only color the text when the result is to displayed at a terminal
	var red, green func(a ...interface{}) string
	useColor := stdout == os.Stdout && isatty.IsTerminal(os.Stdout.Fd())
	if useColor {
		red = color.New(color.FgRed).SprintFunc()
		green = color.New(color.FgGreen).SprintFunc()
	} else {
		red = fmt.Sprint
		green = fmt.Sprint
	}

	for _, tc := range testCases {
		tc := tc

		workerFunc := func() (_ string, rErr error) {
			tempTracker := tempdir.NewDirTracker(fs, false)
			defer tempTracker.DeferMaybeRemoveAll(ctx, &rErr)

			tempDir, err := tempTracker.MkdirTempTracked("", "verify-testcase-"+tc.TestName+"-")
			if err != nil {
				return "", fmt.Errorf("failed creating temp directory: %w", err)
			}

			err = renderTemplateTestCases(ctx, testCases, templateLocation, tempDir)
			if err != nil {
				return "", fmt.Errorf("failed to render test cases: %w", err)
			}
			tempTracker.Track(tempDir)

			p := &diffOutputsOneTestParams{
				templateLocation: templateLocation,
				tempBase:         tempDir,
				useColor:         useColor,
				redSprintf:       red,
				greenSprintf:     green,
			}

			if err := diffOutputsOneTest(ctx, p, tc); err != nil {
				result := red(fmt.Sprintf("[x] template location [%s] golden test [%s] fails", templateLocation, tc.TestName))
				return "", fmt.Errorf("%s:\n %w", result, err)
			}
			return green(fmt.Sprintf("[✓] template location [%s] golden test [%s] succeeds", templateLocation, tc.TestName)), nil
		}

		if err := pool.Do(ctx, workerFunc); err != nil {
			// The only way pool.Do() can return error is if the context is
			// canceled.
			return err //nolint:wrapcheck
		}
	}

	return nil
}

type diffOutputsOneTestParams struct {
	templateLocation string
	tempBase         string
	useColor         bool

	// These may not actually use color if stdout is not a terminal.
	redSprintf, greenSprintf func(...any) string
}

// diffOutputsOneTest checks the got vs want outputs for a single test and
// returns an error if they differ.
func diffOutputsOneTest(ctx context.Context, p *diffOutputsOneTestParams, tc *TestCase) error {
	goldenDataDir := filepath.Join(p.templateLocation, goldenTestDir, tc.TestName, testDataDir)
	tempDataDir := filepath.Join(p.tempBase, goldenTestDir, tc.TestName, testDataDir)

	if !tc.TestConfig.Features.SkipABCRenamed {
		if err := renameGitDirsAndFiles(tempDataDir); err != nil {
			return fmt.Errorf("failed renaming git related dirs and files for test case %s: %w", tc.TestName, err)
		}
	}

	fileSet := make(map[string]struct{})

	exists, err := common.Exists(goldenDataDir)
	if err != nil {
		return err //nolint:wrapcheck
	}
	if !exists {
		return fmt.Errorf("no recorded test data in %q, "+
			"please run `record` command to record the template rendering result to golden tests", goldenDataDir)
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

	var merr error
	anyDiffErrs := false
	for _, relPath := range relPaths {
		if err := diffOneFile(ctx, p, goldenDataDir, tempDataDir, relPath); err != nil {
			merr = errors.Join(merr, err)
			anyDiffErrs = true
		}
	}

	// verify stdout only when SkipStdout flag is set to false.
	if !tc.TestConfig.Features.SkipStdout {
		goldenStdoutFile := filepath.Join(goldenDataDir, common.ABCInternalDir, common.ABCInternalStdout)
		tempStdoutFile := filepath.Join(tempDataDir, common.ABCInternalDir, common.ABCInternalStdout)

		stdoutDiff, err := run.RunDiff(ctx, p.useColor, goldenStdoutFile, goldenDataDir, tempStdoutFile, tempDataDir)
		if err != nil {
			return fmt.Errorf("failed to compare stdout:%w", err)
		}
		if len(stdoutDiff) > 0 {
			failureText := p.redSprintf("the printed messages differ between the recorded golden output and the actual output")
			err := fmt.Errorf("%s:\n%s", failureText, stdoutDiff)
			merr = errors.Join(merr, err)
			anyDiffErrs = true
		}
	}

	if anyDiffErrs {
		failureText := p.redSprintf(fmt.Sprintf("template location [%s] golden test [%s] didn't match actual output, you might "+
			"need to run 'record' command to capture it as the new expected output", p.templateLocation, tc.TestName))
		err := fmt.Errorf(failureText)
		merr = errors.Join(merr, err)
	}

	return merr
}

func diffOneFile(ctx context.Context, p *diffOutputsOneTestParams, goldenDataDir, tempDataDir, relPath string) error {
	goldenFile := filepath.Join(goldenDataDir, relPath)
	tempFile := filepath.Join(tempDataDir, relPath)
	abcRenameTrimmedGoldenFile := strings.TrimSuffix(goldenFile, abcRenameSuffix)
	abcRenameTrimmedTempFile := strings.TrimSuffix(tempFile, abcRenameSuffix)

	// TODO(revell): try rewriting RunDiff to return error if neither file
	// exists, then these can go on the "if RunDiff returns error" branch
	// instead of running every time.
	exists, err := common.Exists(goldenFile)
	if err != nil {
		return fmt.Errorf("failed to read (%s): %w", abcRenameTrimmedGoldenFile, err)
	}

	if !exists {
		failureText := p.redSprintf(fmt.Sprintf("-- [%s] generated, however not recorded in test data", abcRenameTrimmedGoldenFile))
		return fmt.Errorf(failureText)
	}

	exists, err = common.Exists(tempFile)
	if err != nil {
		return fmt.Errorf("failed to read (%s): %w", abcRenameTrimmedTempFile, err)
	}
	if !exists {
		failureText := p.redSprintf(fmt.Sprintf("-- [%s] expected, however missing", abcRenameTrimmedGoldenFile))
		return fmt.Errorf(failureText)
	}

	diff, err := run.RunDiff(ctx, p.useColor, goldenFile, goldenDataDir, tempFile, tempDataDir)
	if err != nil {
		return fmt.Errorf("error diffing actual vs expected output: %w", err)
	}

	if len(diff) > 0 {
		failureText := p.redSprintf(fmt.Sprintf("-- [%s] file content mismatch", abcRenameTrimmedGoldenFile))
		return fmt.Errorf("%s:\n%s", failureText, diff)
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
