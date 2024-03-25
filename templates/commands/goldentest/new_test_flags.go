// Copyright 2024 The Authors (see AUTHORS file)
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

package goldentest

import (
	"fmt"
	"strings"

	"github.com/abcxyz/abc/templates/common/flags"
	"github.com/abcxyz/pkg/cli"
)

// NewTestFlags describes what new golden test to render and how.
type NewTestFlags struct {
	// Positional arguments:

	// Location is the file system location of the template to be tested.
	//
	// Example: t/rest_server.
	Location string

	// NewTestName are the name of the test case to create.
	NewTestName string

	// Flag arguments (--foo):
	// See common/flags.Inputs().
	Inputs map[string]string

	// Whether to prompt the user for new-test inputs.
	Prompt bool

	// Fake builtin_vars used in goldentest.
	BuiltinVars map[string]string

	// ForceOverwrite lets existing test config file be overwritten.
	ForceOverwrite bool
}

func (r *NewTestFlags) Register(set *cli.FlagSet) {
	f := set.NewSection("NEW-TEST OPTIONS")

	f.StringMapVar(flags.Inputs(&r.Inputs))

	f.BoolVar(flags.Prompt(&r.Prompt))

	f.BoolVar(&cli.BoolVar{
		Name:    "force-overwrite",
		Target:  &r.ForceOverwrite,
		Default: false,
		Usage:   "If an test yaml file already exists, overwrite it instead of failing.",
	})

	f.StringMapVar(&cli.StringMapVar{
		Name:    "builtin-var",
		Example: "_git_tag=my-cool-tag",
		Target:  &r.BuiltinVars,
		Usage:   "The key=val pairs of builtin_vars; may be repeated.",
	})

	// Default NewTestName to the first CLI argument, if given
	set.AfterParse(func(existingErr error) error {
		r.NewTestName = set.Arg(0)

		if r.NewTestName == "" {
			return fmt.Errorf("missing template <new-test-name>")
		}

		if strings.Contains(r.NewTestName, "/") || strings.Contains(r.NewTestName, "\\") {
			return fmt.Errorf("<new-test-name> can't include any slashes or backslashes")
		}
		return nil
	})

	// Default Location to the second CLI argument, if given
	// If not given, default to current directory.
	set.AfterParse(func(existingErr error) error {
		r.Location = strings.TrimSpace(set.Arg(1))

		if r.Location == "" {
			// make current directory the default location
			r.Location = "."
		}
		return nil
	})
}
