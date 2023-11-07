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

	"github.com/abcxyz/pkg/cli"
	"github.com/posener/complete/v2/predict"
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

	// GitProtocol is not yet used.
	GitProtocol string

	// LogLevel is one of debug|info|warn|error|panic.
	LogLevel string

	// ForceOverwrite lets existing output files in the Dest directory be overwritten
	// with the output of the template.
	ForceOverwrite bool

	// Inputs provide values that are substituted into the template. The keys in
	// this map must match the input names in the Source template's spec.yaml
	// file.
	//
	// This is mutable, even after flag parsing. It may be updated when default
	// values are added and as the user is prompted for inputs.
	Inputs map[string]string // these are just the --input values from flags; doesn't include values from env vars

	// InputFiles are the files containing a list of inputs. See Inputs flag for details.
	InputFiles []string

	// KeepTempDirs prevents the cleanup of temporary directories after rendering is complete.
	// This can be useful for debugging a failing template.
	KeepTempDirs bool

	// Whether to prompt the user for template inputs.
	Prompt bool

	// DebugScratchContents causes the contents of the scratch directory to be
	// logged at level INFO after each step of the spec.yaml.
	DebugScratchContents bool

	// SkipInputValidation skips the execution of the input validation rules as
	// configured in the template's spec.yaml file.
	SkipInputValidation bool
}

func (r *RenderFlags) Register(set *cli.FlagSet) {
	f := set.NewSection("RENDER OPTIONS")

	f.StringVar(&cli.StringVar{
		Name:    "dest",
		Aliases: []string{"d"},
		Example: "/my/git/dir",
		Target:  &r.Dest,
		Default: ".",
		Predict: predict.Dirs("*"),
		Usage:   "Required. The target directory in which to write the output files.",
	})

	f.StringMapVar(&cli.StringMapVar{
		Name:    "input",
		Example: "foo=bar",
		Target:  &r.Inputs,
		Usage:   "The key=val pairs of template values; may be repeated.",
	})

	f.StringSliceVar(&cli.StringSliceVar{
		Name:    "input-file",
		Example: "/my/git/abc-inputs.yaml",
		Target:  &r.InputFiles,
		Usage:   "The yaml files with key: val pairs of template values; may be repeated.",
	})

	f.StringVar(&cli.StringVar{
		Name:    "log-level",
		Example: "info",
		Default: defaultLogLevel,
		Target:  &r.LogLevel,
		Usage:   "How verbose to log; any of debug|info|warn|error.",
	})

	f.BoolVar(&cli.BoolVar{
		Name:    "force-overwrite",
		Target:  &r.ForceOverwrite,
		Default: false,
		Usage:   "If an output file already exists in the destination, overwrite it instead of failing.",
	})

	f.BoolVar(&cli.BoolVar{
		Name:    "keep-temp-dirs",
		Target:  &r.KeepTempDirs,
		Default: false,
		Usage:   "Preserve the temp directories instead of deleting them normally.",
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
		Name:    "skip-input-validation",
		Target:  &r.SkipInputValidation,
		Default: false,
		Usage:   "Skip running the validation expressions for inputs that were configured in spec.yaml.",
	})

	t := set.NewSection("TEMPLATE AUTHORS")
	t.BoolVar(&cli.BoolVar{
		Name:    "debug-scratch-contents",
		Target:  &r.DebugScratchContents,
		Default: false,
		Usage:   "Print the contents of the scratch directory after each step; for debugging spec.yaml files.",
	})

	g := set.NewSection("GIT OPTIONS")
	g.StringVar(&cli.StringVar{
		Name:    "git-protocol",
		Example: "https",
		Default: "https",
		Target:  &r.GitProtocol,
		Predict: predict.Set([]string{"https", "ssh"}),
		Usage:   "Either ssh or https, the protocol for connecting to git. Only used if the template source is a git repo.",
	})

	// Default source to the first CLI argument, if given
	set.AfterParse(func(existingErr error) error {
		r.Source = strings.TrimSpace(set.Arg(0))
		if r.Source == "" {
			return fmt.Errorf("missing <source> file")
		}

		return nil
	})
}
