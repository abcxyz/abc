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

// Package flags contains flags that are commonly used by several commands.
package flags

import (
	"github.com/posener/complete/v2/predict"

	"github.com/abcxyz/pkg/cli"
)

// GitProtocol is a flag that's either https or ssh. It controls how we talk to
// remote git servers like GitHub.
func GitProtocol(target *string) *cli.StringVar {
	return &cli.StringVar{
		Name:    "git-protocol",
		Example: "https",
		Default: "https",
		Predict: predict.Set([]string{"https", "ssh"}),
		Target:  target,
		EnvVar:  "ABC_GIT_PROTOCOL",
		Usage:   "Either ssh or https, the protocol for connecting to git. Only used if the template source is a git repo.",
	}
}

// Inputs provide values that are substituted into the template. The keys in
// this map must match the input names in the Source template's spec.yaml
// file.
//
// These are just the --input values from flags. It doesn't include inputs
// from config files, defaults, or prompts.
func Inputs(inputs *map[string]string) *cli.StringMapVar {
	return &cli.StringMapVar{
		Name:    "input",
		Example: "foo=bar",
		Target:  inputs,
		Usage:   "The key=val pairs of template values; may be repeated.",
	}
}

// InputFiles are the files containing a YAML template inputs, similar to --input.
func InputFiles(inputFiles *[]string) *cli.StringSliceVar {
	return &cli.StringSliceVar{
		Name:    "input-file",
		Example: "/my/git/abc-inputs.yaml",
		Predict: predict.Files(""),
		Target:  inputFiles,
		Usage:   "The yaml files with key: val pairs of template values; may be repeated.",
	}
}

// KeepTempDirs prevents the cleanup of temporary directories after rendering is
// complete. This can be useful for debugging a failing template.
func KeepTempDirs(k *bool) *cli.BoolVar {
	return &cli.BoolVar{
		Name:    "keep-temp-dirs",
		Target:  k,
		Default: false,
		Usage:   "Preserve the temp directories instead of deleting them normally.",
	}
}

// SkipInputValidation skips the execution of the input validation rules as
// configured in the template's spec.yaml file.
func SkipInputValidation(s *bool) *cli.BoolVar {
	return &cli.BoolVar{
		Name:    "skip-input-validation",
		Target:  s,
		Default: false,
		Usage:   "Skip running the validation expressions for inputs that were configured in spec.yaml.",
	}
}

// DebugScratchContents causes the contents of the scratch directory to be
// logged at level INFO after each step of the spec.yaml.
func DebugScratchContents(d *bool) *cli.BoolVar {
	return &cli.BoolVar{
		Name:    "debug-scratch-contents",
		Target:  d,
		Default: false,
		Usage:   "Print the contents of the scratch directory after each step; for debugging spec.yaml files.",
	}
}

// DebugStepDiffs causes the diffs between steps to be logged as git commits.
func DebugStepDiffs(d *bool) *cli.BoolVar {
	return &cli.BoolVar{
		Name:    "debug-step-diffs",
		Target:  d,
		Default: false,
		Usage:   "Commit the diffs between steps for debugging.",
	}
}

// Prompt causes the user to be prompted for any needed input values.
func Prompt(p *bool) *cli.BoolVar {
	return &cli.BoolVar{
		Name:   "prompt",
		Target: p,

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

		EnvVar: "ABC_PROMPT",

		Usage: "Prompt the user for template inputs that weren't provided as flags.",
	}
}

// Verbose causes more output to be produced.
func Verbose(v *bool) *cli.BoolVar {
	return &cli.BoolVar{
		Name:    "verbose",
		Aliases: []string{"v"},
		Target:  v,
		Default: false,
		EnvVar:  "ABC_VERBOSE",
		Usage:   "include more output; intended for power users",
	}
}

// AcceptDefaults causes template defaults to be accepted automatically when
// prompting is disabled.
func AcceptDefaults(a *bool) *cli.BoolVar {
	return &cli.BoolVar{
		Name:    "accept-defaults",
		Target:  a,
		Default: false,
		EnvVar:  "ABC_ACCEPT_DEFAULTS",
		Usage:   "when a template input has a default value, and the user didn't provide a value for that input, and prompting is disabled, this will cause the default value to be silently used.",
	}
}

func UpgradeChannel(u *string) *cli.StringVar {
	return &cli.StringVar{
		Name:    "upgrade-channel",
		Target:  u,
		Default: "",
		EnvVar:  "ABC_UPGRADE_CHANNEL",
		Usage:   `overrides the "upgrade_channel" field in the output manifest, which controls where upgraded template versions will be pulled from in the future by "abc uprade". Can be either a branch name or the special string "latest". The default is to upgrade from the branch that the template was originally rendered from if rendered from a branch, or in any other case to use the value "latest" to upgrade to the latest release tag by semver order.`,
	}
}
