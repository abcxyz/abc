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

package commands

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
	// Example: github.com/abcxyz/abc.git//t/rest_server
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
	Inputs map[string]string // these are just the --input values from flags; doesn't inclue values from config file or env vars

	// KeepTempDirs prevents the cleanup of temporary directories after rendering is complete.
	// This can be useful for debugging a failing template.
	KeepTempDirs bool

	// Spec is the relative path within the template of the YAML file that
	// specifies how the template is rendered. This will often be just
	// "spec.yaml".
	Spec string
}

func (r *RenderFlags) Register(set *cli.FlagSet) {
	f := set.NewSection("RENDER OPTIONS")
	f.StringVar(&cli.StringVar{
		Name:    "spec",
		Example: "path/to/spec.yaml",
		Target:  &r.Spec,
		Default: "./spec.yaml",
		Usage:   "The path of the .yaml file within the unpacked template directory that specifies how the template is rendered.",
	})

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
