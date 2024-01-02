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
