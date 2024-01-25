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

	"github.com/posener/complete/v2/predict"

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

	f.StringVar(&cli.StringVar{
		Name:    "location",
		Example: "/my/git/dir",
		Target:  &r.Location,
		Default: ".",
		Predict: predict.Dirs("*"),
		Usage: "Location is the file system location of the template to be tested and " +
			"it must be a local directory.",
	})

	f.StringMapVar(flags.Inputs(&r.Inputs))

	f.BoolVar(&cli.BoolVar{
		Name:   "prompt",
		Target: &r.Prompt,

		// Design decision: --prompt defaults to false because we don't want to
		// confuse the user with an unexpected prompt.
		//
		// Consider this motivating use case: there's a playbook abc command
		// that the user is copy-pasting, and this command has always worked
		// with the  --input values provided in the playbook. There is no need
		// for prompting. Then the upstream template developer adds a new input
		// that is not anticipated by the playbook. In this case, we'd rather
		// just fail outright than have the CLI prompt for the missing input. If
		// we prompted the user, they'd be tempted to enter something creative
		// rather than realize that their playbook needs to be updated.
		Default: false,

		Usage: "Prompt the user for template inputs that weren't provided as flags.",
	})

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
}
