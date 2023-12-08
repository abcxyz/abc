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

package describe

import (
	"fmt"
	"strings"

	"github.com/posener/complete/v2/predict"

	"github.com/abcxyz/pkg/cli"
)

// DescribeFlags describes what template to describe.
type DescribeFlags struct {
	// Source is the location of the input template to be rendered.
	//
	// Example: github.com/abcxyz/abc/t/rest_server@latest
	Source string

	// GitProtocol is not yet used.
	GitProtocol string
}

func (r *DescribeFlags) Register(set *cli.FlagSet) {
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
