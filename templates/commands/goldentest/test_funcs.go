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

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/benbjohnson/clock"

	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/abc/templates/common/errs"
	"github.com/abcxyz/abc/templates/common/render"
	"github.com/abcxyz/abc/templates/model/decode"
	goldentest "github.com/abcxyz/abc/templates/model/goldentest/v1beta3"
)

// TestCase describes a template golden test case.
type TestCase struct {
	// Name of the test case.
	// Example: nextjs_with_auth0_idp.
	TestName string

	// Config of the test case.
	TestConfig *goldentest.Test
}

const (
	// The golden test directory is alwayse located in the template root dir and
	// named testdata/golden.
	goldenTestDir = "testdata/golden"

	// The subdirectory under a test case that records test data.
	// Example: testdata/golden/test-case-1/data/...
	testDataDir = "data"

	// The golden test config file is always located in the test case root dir and
	// named test.yaml.
	configName = "test.yaml"
)

// parseTestCases returns a list of test cases to record or verify.
func parseTestCases(ctx context.Context, location string, testNames []string) ([]*TestCase, error) {
	if _, err := os.Stat(location); err != nil {
		return nil, fmt.Errorf("error reading template directory (%s): %w", location, err)
	}

	testDir := filepath.Join(location, goldenTestDir)

	testCases := []*TestCase{}

	if len(testNames) > 0 {
		for _, testName := range testNames {
			testCase, err := buildTestCase(ctx, testDir, testName)
			if err != nil {
				return nil, err
			}

			testCases = append(testCases, testCase)
		}
		return testCases, nil
	}

	entries, err := os.ReadDir(testDir)
	if err != nil {
		return nil, fmt.Errorf("error reading golden test directory (%s): %w", testDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			return nil, fmt.Errorf("unexpected file entry under golden test directory: %s", entry.Name())
		}

		testCase, err := buildTestCase(ctx, testDir, entry.Name())
		if err != nil {
			return nil, err
		}

		testCases = append(testCases, testCase)
	}

	return testCases, nil
}

// buildtestCases builds the name and config of a test case.
func buildTestCase(ctx context.Context, testDir, testName string) (*TestCase, error) {
	testConfig := filepath.Join(testDir, testName, configName)
	test, err := parseTestConfig(ctx, testConfig)
	if err != nil {
		return nil, err
	}

	return &TestCase{
		TestName:   testName,
		TestConfig: test,
	}, nil
}

// parseTestConfig reads a configuration yaml and returns the result.
func parseTestConfig(ctx context.Context, path string) (*goldentest.Test, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening test config (%s): %w", path, err)
	}
	defer f.Close()

	testI, err := decode.DecodeValidateUpgrade(ctx, f, path, decode.KindGoldenTest)
	if err != nil {
		return nil, fmt.Errorf("error reading golden test config file: %w", err)
	}
	out, ok := testI.(*goldentest.Test)
	if !ok {
		return nil, fmt.Errorf("internal error: expected golden test config to be of type *goldentest.Test but got %T", testI)
	}

	return out, nil
}

// renderTestCases render all test cases in a temporary directory.
func renderTestCases(ctx context.Context, testCases []*TestCase, location string) (string, error) {
	tempDir, err := os.MkdirTemp("", "abc-test-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}

	var merr error
	for _, tc := range testCases {
		merr = errors.Join(merr, renderTestCase(ctx, location, tempDir, tc))
	}
	if merr != nil {
		return "", fmt.Errorf("failed to render golden tests: %w", merr)
	}
	return tempDir, nil
}

// renderTestCase executes the "template render" command based upon test config.
func renderTestCase(ctx context.Context, templateDir, outputDir string, tc *TestCase) error {
	testDir := filepath.Join(outputDir, goldenTestDir, tc.TestName, testDataDir)

	if err := os.RemoveAll(testDir); err != nil {
		return fmt.Errorf("failed to clear test directory: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("os.Getwd(): %w", err)
	}

	err = render.Render(ctx, &render.Params{
		OverrideBuiltinVars: varValuesToMap(tc.TestConfig.BuiltinVars),
		Clock:               clock.New(),
		Cwd:                 cwd,
		DestDir:             testDir,
		FS:                  &common.RealFS{},
		Inputs:              varValuesToMap(tc.TestConfig.Inputs),
		Source:              templateDir,
		Stdout:              io.Discard, // Mute stdout from command runs.
	})
	if err != nil {
		var uve *errs.UnknownVar
		if errors.As(err, &uve) && strings.HasPrefix(uve.VarName, "_") {
			return fmt.Errorf("you may need to provide a value for %q in the builtin_vars section of test.yaml: %w", uve.VarName, err)
		}
		return err //nolint:wrapcheck
	}

	return nil
}

func varValuesToMap(vvs []*goldentest.VarValue) map[string]string {
	out := make(map[string]string, len(vvs))
	for _, vv := range vvs {
		out[vv.Name.Val] = vv.Value.Val
	}
	return out
}
