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
	"github.com/abcxyz/pkg/cli"
	"github.com/posener/complete/v2/predict"
)

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
