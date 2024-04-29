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
	"fmt"
	"strings"

	"github.com/abcxyz/abc/templates/common/flags"
	"github.com/abcxyz/abc/templates/common/templatesource"
	"github.com/abcxyz/pkg/cli"
)

type Flags struct {
	Manifest string

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

	// See common/flags.Prompt().
	Prompt bool

	// See common/flags.SkipInputValidation().
	SkipInputValidation bool

	// The template version to upgrade to; defaults to "latest".
	Version string
}

func (f *Flags) Register(set *cli.FlagSet) {
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

	// Manifest is the first CLI argument.
	set.AfterParse(func(existingErr error) error {
		f.Manifest = strings.TrimSpace(set.Arg(0))
		if f.Manifest == "" {
			return fmt.Errorf("missing <manifest> file argument")
		}

		return nil
	})
}
