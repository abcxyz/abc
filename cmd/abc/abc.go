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
	"os/signal"
	"syscall"

	"github.com/abcxyz/abc/internal/version"
	"github.com/abcxyz/abc/templates/commands/describe"
	"github.com/abcxyz/abc/templates/commands/goldentest"
	"github.com/abcxyz/abc/templates/commands/render"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
)

const (
	defaultLogLevel = "warn"
	defaultLogMode  = "text"
)

var rootCmd = func() *cli.RootCommand {
	return &cli.RootCommand{
		Name:    version.Name,
		Version: version.HumanVersion,
		Commands: map[string]cli.CommandFactory{
			"templates": func() cli.Command {
				return &cli.RootCommand{
					Name:        "templates",
					Description: "subcommands for rendering templates and related things",
					Commands: map[string]cli.CommandFactory{
						"render": func() cli.Command {
							return &render.Command{}
						},
						"golden-test": func() cli.Command {
							return &cli.RootCommand{
								Name:        "golden-test",
								Description: "subcommands for validating template rendering with golden tests",
								Commands: map[string]cli.CommandFactory{
									"record": func() cli.Command {
										return &goldentest.RecordCommand{}
									},
									"verify": func() cli.Command {
										return &goldentest.VerifyCommand{}
									},
								},
							}
						},
						"describe": func() cli.Command {
							return &describe.Command{}
						},
					},
				}
			},
		},
	}
}

func main() {
	ctx, done := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer done()

	setLogEnvVars()
	ctx = logging.WithLogger(ctx, logging.NewFromEnv("ABC_"))

	if err := realMain(ctx); err != nil {
		done()
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func setLogEnvVars() {
	if os.Getenv("ABC_LOG_FORMAT") == "" {
		os.Setenv("ABC_LOG_FORMAT", defaultLogMode)
	}

	if os.Getenv("ABC_LOG_LEVEL") == "" {
		os.Setenv("ABC_LOG_LEVEL", defaultLogLevel)
	}
}

func realMain(ctx context.Context) error {
	return rootCmd().Run(ctx, os.Args[1:]) //nolint:wrapcheck
}
