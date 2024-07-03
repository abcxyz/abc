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

	// Ignore any values in the Inputs map that aren't valid template inputs,
	// rather than returning error.
	IgnoreUnknownInputs bool

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

	// Whether to *only* create a manifest file without outputting any other
	// files from the template.
	ManifestOnly bool

	// Overrides the `upgrade_channel` field in the output manifest. Can be
	// either a branch name or the special string "latest".
	UpgradeChannel string
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
		Name:    "ignore-unknown-inputs",
		Target:  &r.IgnoreUnknownInputs,
		Default: false,
		Usage:   "If a user-provided input name isn't recognized by the template, ignore that input value instead of failing.",
	})

	f.BoolVar(flags.Prompt(&r.Prompt))

	f.BoolVar(&cli.BoolVar{
		Name:    "manifest",
		Target:  &r.Manifest,
		Default: false,
		EnvVar:  "ABC_MANIFEST",
		// TODO(upgrade): remove "(experimental)"
		Usage: "(experimental) write a manifest file containing metadata that will allow future template upgrades.",
	})

	f.StringVar(&cli.StringVar{
		Name:    "upgrade-channel",
		Target:  &r.UpgradeChannel,
		Default: "",
		EnvVar:  "ABC_UPGRADE_CHANNEL",
		Usage:   `overrides the "upgrade_channel" field in the output manifest, which controls where upgraded template versions will be pulled from in the future by "abc uprade". Can be either a branch name or the special string "latest". The default is to upgrade from the branch that the template was originally rendered from if rendered from a branch, or in any other case to use the value "latest" to upgrade to the latest release tag by semver order.`,
	})

	f.BoolVar(&cli.BoolVar{
		Name:    "manifest-only",
		Target:  &r.ManifestOnly,
		Default: false,
		EnvVar:  "ABC_MANIFEST_ONLY",
		// TODO(upgrade): remove "(experimental)"
		Usage: "(experimental) write only a manifest file and no other files; implicitly sets --manifest=true",
	})

	t := set.NewSection("TEMPLATE AUTHORS")
	t.BoolVar(flags.DebugScratchContents(&r.DebugScratchContents))
	t.BoolVar(flags.DebugStepDiffs(&r.DebugStepDiffs))

	g := set.NewSection("GIT OPTIONS")

	g.StringVar(flags.GitProtocol(&r.GitProtocol))

	// Default source to the first CLI argument, if given
	set.AfterParse(func(existingErr error) error {
		r.Source = strings.TrimSpace(set.Arg(0))
		if r.Source == "" {
			return fmt.Errorf("missing <source> file")
		}

		if r.ManifestOnly {
			// --manifest-only implies the user wants a manifest.
			r.Manifest = true
		}

		return nil
	})
}
