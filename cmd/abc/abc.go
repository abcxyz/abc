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
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/abcxyz/abc-updater/sdk/go/abc-updater"
	"github.com/abcxyz/abc/internal/version"
	"github.com/abcxyz/abc/templates/commands/describe"
	"github.com/abcxyz/abc/templates/commands/goldentest"
	"github.com/abcxyz/abc/templates/commands/render"
	"github.com/abcxyz/abc/templates/commands/upgrade"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
)

const (
	defaultLogLevel  = logging.LevelWarning
	defaultLogFormat = logging.FormatText
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
						"describe": func() cli.Command {
							return &describe.Command{}
						},
						"golden-test": func() cli.Command {
							return &cli.RootCommand{
								Name:        "golden-test",
								Description: "subcommands for validating template rendering with golden tests",
								Commands: map[string]cli.CommandFactory{
									"new-test": func() cli.Command {
										return &goldentest.NewTestCommand{}
									},
									"record": func() cli.Command {
										return &goldentest.RecordCommand{}
									},
									"verify": func() cli.Command {
										return &goldentest.VerifyCommand{}
									},
								},
							}
						},
						"render": func() cli.Command {
							return &render.Command{}
						},
						"upgrade": func() cli.Command {
							return &upgrade.Command{}
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
		os.Setenv("ABC_LOG_FORMAT", string(defaultLogFormat))
	}

	if os.Getenv("ABC_LOG_LEVEL") == "" {
		os.Setenv("ABC_LOG_LEVEL", defaultLogLevel.String())
	}
}

// checkForUpdates asynchronously checks for updates and prints results to
// stderr. A sync.WaitGroup is returned so main can wait for completion before
// exiting.
func checkForUpdates(ctx context.Context) sync.WaitGroup {
	updaterParams := abcupdater.CheckVersionParams{
		AppID: version.Name,
		// Version: version.Version,
		Version: "0.4.0", // intentionally older than cur version to test
		Writer:  os.Stderr,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// todo: timeout?
		logger := logging.FromContext(ctx)
		updateTime := time.Now()
		err := abcupdater.CheckAppVersion(ctx, &updaterParams)
		logger.WarnContext(ctx, "*dev* version check time", "ms", time.Now().Sub(updateTime).Milliseconds())
		if err != nil {
			logger.WarnContext(ctx, "failed to check for new versions of abc", "error", err)
		}
	}()
	return wg
}

func realMain(ctx context.Context) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("windows os is not supported in abc cli")
	}

	wg := checkForUpdates(ctx)
	defer wg.Wait()

	return rootCmd().Run(ctx, os.Args[1:]) //nolint:wrapcheck
}
