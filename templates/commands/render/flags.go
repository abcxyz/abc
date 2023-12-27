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

package render

import (
	"fmt"
	"strings"

	"github.com/posener/complete/v2/predict"

	"github.com/abcxyz/abc/templates/common/flags"
	"github.com/abcxyz/pkg/cli"
)

// RenderFlags describes what template to render and how.
type RenderFlags struct {
	// Positional arguments:

	// Source is the location of the input template to be rendered.
	//
	// Example: github.com/abcxyz/abc/t/rest_server@latest
	Source string

	// Flag arguments (--foo):

	// Dest is the local directory where the template output will be written.
	// It's OK for it to already exist or not.
	Dest string

	// See common/flags.GitProtocol().
	GitProtocol string

	// ForceOverwrite lets existing output files in the Dest directory be overwritten
	// with the output of the template.
	ForceOverwrite bool

	// See common/flags.Inputs().
	Inputs map[string]string

	// See common/flags.InputFiles().
	InputFiles []string

	// See common/flags.KeepTempDirs().
	KeepTempDirs bool

	// Whether to prompt the user for template inputs.
	Prompt bool

	// See common/flags.DebugStepDiffs().
	DebugStepDiffs bool

	// See common/flags.DebugScratchContents().
	DebugScratchContents bool

	// See common/flags.SkipInputValidation().
	SkipInputValidation bool

	// Manifest enables the writing of manifest files, which are an experimental
	// feature related to template upgrades.
	Manifest bool
}

func (r *RenderFlags) Register(set *cli.FlagSet) {
	f := set.NewSection("RENDER OPTIONS")

	f.StringMapVar(flags.Inputs(&r.Inputs))
	f.StringSliceVar(flags.InputFiles(&r.InputFiles))
	f.BoolVar(flags.KeepTempDirs(&r.KeepTempDirs))
	f.BoolVar(flags.SkipInputValidation(&r.SkipInputValidation))

	f.StringVar(&cli.StringVar{
		Name:    "dest",
		Aliases: []string{"d"},
		Example: "/my/git/dir",
		Target:  &r.Dest,
		Default: ".",
		Predict: predict.Dirs("*"),
		Usage:   "Required. The target directory in which to write the output files.",
	})

	f.BoolVar(&cli.BoolVar{
		Name:    "force-overwrite",
		Target:  &r.ForceOverwrite,
		Default: false,
		Usage:   "If an output file already exists in the destination, overwrite it instead of failing.",
	})

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
		Name:    "manifest",
		Target:  &r.Manifest,
		Default: false,
		Usage:   "(experimental) write a manifest file containing metadata that will allow future template upgrades.",
	})

	t := set.NewSection("TEMPLATE AUTHORS")
	t.BoolVar(flags.DebugScratchContents(&r.DebugScratchContents))

	t.BoolVar(&cli.BoolVar{
		Name:    "debug-step-diffs",
		Target:  &r.DebugStepDiffs,
		Default: false,
		Usage:   "Commit the diffs between steps for debugging.",
	})

	g := set.NewSection("GIT OPTIONS")

	g.StringVar(flags.GitProtocol(&r.GitProtocol))

	// Default source to the first CLI argument, if given
	set.AfterParse(func(existingErr error) error {
		r.Source = strings.TrimSpace(set.Arg(0))
		if r.Source == "" {
			return fmt.Errorf("missing <source> file")
		}

		return nil
	})
}
