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
	"os"
	"path/filepath"

	"github.com/abcxyz/abc/templates/commands/render"
	"github.com/abcxyz/abc/templates/model/decode"
	goldentest "github.com/abcxyz/abc/templates/model/goldentest/v1alpha1"
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
func parseTestCases(ctx context.Context, location, testName string) ([]*TestCase, error) {
	if _, err := os.Stat(location); err != nil {
		return nil, fmt.Errorf("error reading template directory (%s): %w", location, err)
	}

	testDir := filepath.Join(location, goldenTestDir)

	if testName != "" {
		testConfig := filepath.Join(testDir, testName, configName)
		test, err := parseTestConfig(ctx, testConfig)
		if err != nil {
			return nil, err
		}
		return []*TestCase{
			{
				TestName:   testName,
				TestConfig: test,
			},
		}, nil
	}

	entries, err := os.ReadDir(testDir)
	if err != nil {
		return nil, fmt.Errorf("error reading golden test directory (%s): %w", testDir, err)
	}

	testCases := []*TestCase{}
	for _, entry := range entries {
		if !entry.IsDir() {
			return nil, fmt.Errorf("unexpected file entry under golden test directory: %s", entry.Name())
		}

		testConfig := filepath.Join(testDir, entry.Name(), configName)
		test, err := parseTestConfig(ctx, testConfig)
		if err != nil {
			return nil, err
		}

		testCases = append(testCases, &TestCase{
			TestName:   entry.Name(),
			TestConfig: test,
		})
	}

	return testCases, nil
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

	args := []string{"--dest", testDir, "--force-overwrite"}
	for _, input := range tc.TestConfig.Inputs {
		args = append(args, "--input")
		args = append(args, fmt.Sprintf("%s=%s", input.Name.Val, input.Value.Val))
	}
	args = append(args, templateDir)

	r := &render.Command{}
	// Mute stdout from command runs.
	r.Pipe()

	// TODO(chloechien): Use rendering library instead of calling cmd directly.
	if err := r.Run(ctx, args); err != nil {
		return fmt.Errorf("error running `templates render` command: %w", err)
	}
	return nil
}
