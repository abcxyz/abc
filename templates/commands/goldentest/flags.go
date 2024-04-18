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
	"strings"

	"github.com/abcxyz/pkg/cli"
)

// Flags describes the template location and the test case.
type Flags struct {
	// Positional arguments:

	// Location is the file system location of the templates to be tested.
	//
	// Example: t/rest_server.
	Location string

	// Flag arguments (--foo):

	// Testnames are the name of the test cases to record or verify. If no
	// test name is specified, all gold tests will be run against.
	//
	// Optional.
	TestNames []string
}

func (r *Flags) Register(set *cli.FlagSet) {
	f := set.NewSection("TEST OPTIONS")

	f.StringSliceVar(&cli.StringSliceVar{
		Name:    "test-name",
		Aliases: []string{"t"},
		Example: "test_case_1",
		Target:  &r.TestNames,
		Usage:   "The name of the test cases to record or verify.",
	})

	// Default location to the first CLI argument, if given.
	// If not given, default to current directory.
	set.AfterParse(func(existingErr error) error {
		r.Location = strings.TrimSpace(set.Arg(0))

		if r.Location == "" {
			// make current directory the default location
			r.Location = "."
		}
		return nil
	})
}
