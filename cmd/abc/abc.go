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

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/abcxyz/abc/internal/templates/render"
	"github.com/abcxyz/pkg/cli"
)

var rootCmd = func() *cli.RootCommand {
	return &cli.RootCommand{
		Name:    "abc",
		Version: "0.0.1",
		Commands: map[string]cli.CommandFactory{
			"templates": func() cli.Command {
				return &cli.RootCommand{
					Name:        "templates",
					Description: "subcommands for rendering templates and related things",
					Commands: map[string]cli.CommandFactory{
						"render": func() cli.Command {
							return &render.Command{}
						},
					},
				}
			},
		},
	}
}

func main() {
	ctx := context.Background()
	if err := rootCmd().Run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
