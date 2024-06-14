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

package upgrade

import (
	"strings"

	"github.com/posener/complete/v2/predict"

	"github.com/abcxyz/abc/templates/common/flags"
	"github.com/abcxyz/abc/templates/common/templatesource"
	"github.com/abcxyz/pkg/cli"
)

type Flags struct {
	Location string

	// A list of files that were...
	//   - changed in place by a previous render operation...
	//   - then an upgrade operation was attempted, which attempted to undo the
	//     change by applying the reversal patch in the manifest...
	//   - but the patch failed to apply cleanly...
	//   - and the user was asked to manually resolve the conflict by manually
	//     applying the .rej rejected patch file...
	//   - which the user did, and removed the patch file...
	//   - which means that we can resume the upgrade operation, without needing
	//     to patch these files, because they've already been patched.
	//
	// So basically it's a set of included-from-destination files that will be
	// skipped when doing the phase of the upgrade operation that tries to
	// reverse changes that were previously made to modifed-in-place files.
	AlreadyResolved []string

	// See common/flags.DebugScratchContents().
	DebugScratchContents bool

	// See common/flags.DebugStepDiffs().
	DebugStepDiffs bool

	// See common/flags.GitProtocol().
	GitProtocol string

	// See common/flags.Inputs().
	Inputs map[string]string

	// See common/flags.InputFiles().
	InputFiles []string

	// See common/flags.KeepTempDirs().
	KeepTempDirs bool

	// The manifest to start with, when upgrading multiple manifests. This is
	// used when a previous upgrade operation required manual intervention, and
	// the manual intervention is done, and the user wants to resume.
	// TODO(upgrade): implement this feature.
	ResumeFrom string

	// See common/flags.Prompt().
	Prompt bool

	// See common/flags.SkipInputValidation().
	SkipInputValidation bool

	Verbose bool

	// The template version to upgrade to; defaults to "latest".
	Version string
}

func (f *Flags) Register(set *cli.FlagSet) {
	u := set.NewSection("UPGRADE OPTIONS")
	u.StringSliceVar(&cli.StringSliceVar{
		Name:    "already-resolved",
		Example: "my_file.txt,my_dir/my_other_file.txt",
		Predict: predict.Files(""),
		Target:  &f.AlreadyResolved,
		Usage:   "a list of files where a patch failed to apply during the upgrade process, generating a .patch.rej file that was manually resolved by the user; usually combined with --resume-from",
	})
	u.StringVar(&cli.StringVar{
		Name:    "resume-from",
		Example: "./some/dir/.abc/manifest.foo.yaml",
		Predict: predict.Files("*.yaml"),
		Target:  &f.ResumeFrom,
		Usage:   "begin or resume the upgrade starting at this manifest file",
	})
	u.BoolVar(flags.Verbose(&f.Verbose))

	r := set.NewSection("RENDER OPTIONS")

	r.StringMapVar(flags.Inputs(&f.Inputs))
	r.StringSliceVar(flags.InputFiles(&f.InputFiles))
	r.BoolVar(flags.SkipInputValidation(&f.SkipInputValidation))
	r.BoolVar(flags.DebugStepDiffs(&f.DebugStepDiffs))
	r.BoolVar(flags.KeepTempDirs(&f.KeepTempDirs))
	r.BoolVar(flags.Prompt(&f.Prompt))
	r.StringVar(&cli.StringVar{
		Name:    "version",
		Usage:   "for remote templates, the version to upgrade to; may be git tag, branch, or SHA",
		Example: "main",
		Default: templatesource.Latest,
		EnvVar:  "ABC_UPGRADE_TO_VERSION",
		Target:  &f.Version,
	})
	t := set.NewSection("TEMPLATE AUTHORS")
	t.BoolVar(flags.DebugScratchContents(&f.DebugScratchContents))

	g := set.NewSection("GIT OPTIONS")
	g.StringVar(flags.GitProtocol(&f.GitProtocol))

	set.AfterParse(func(existingErr error) error {
		// Default location to the first CLI argument, if given.
		// If not given, default to current directory.
		f.Location = strings.TrimSpace(set.Arg(0))
		return nil
	})
}
