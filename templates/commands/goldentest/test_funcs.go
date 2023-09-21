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
	"fmt"
	"os"
	"path/filepath"

	"github.com/abcxyz/abc/templates/model/goldentest"
)

// TestCase describes a template golden test case.
type TestCase struct {
	// Name of the test case.
	TestName string

	// Config of the test case.
	TestConfig *goldentest.Test
}

const (
	// The golden test directory is alwayse located in the template root dir and
	// named testdata/golden.
	goldenTestDir = "testdata/golden"

	// The golden test config file is always located in the test case root dir and
	// named test.yaml.
	configName = "test.yaml"
)

// ParseTestCases returns a list of test cases to record or verify.
func ParseTestCases(location, testName string) ([]*TestCase, error) {
	if _, err := os.Stat(location); err != nil {
		return nil, fmt.Errorf("error reading template directory (%s): %w", location, err)
	}

	testDir := filepath.Join(location, goldenTestDir)

	if testName != "" {
		testConfig := filepath.Join(testDir, testName, configName)
		test, err := parseTestConfig(testConfig)
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
			return nil, fmt.Errorf("unexpeted file entry under golden test directory: %s", entry.Name())
		}

		testConfig := filepath.Join(testDir, entry.Name(), configName)
		test, err := parseTestConfig(testConfig)
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
func parseTestConfig(path string) (*goldentest.Test, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening test config (%s): %w", path, err)
	}
	defer f.Close()

	test, err := goldentest.DecodeTest(f)
	if err != nil {
		return nil, fmt.Errorf("error reading golden test config file: %w", err)
	}
	return test, nil
}