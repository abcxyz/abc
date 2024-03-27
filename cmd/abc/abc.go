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
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/abcxyz/abc-updater/pkg/abcupdater"
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
// returned buffer.
// A sync.WaitGroup is returned so main can wait for completion before exiting.
func checkForUpdates(ctx context.Context) func() {
	updatesCh := make(chan string, 1)

	go func() {
		defer close(updatesCh)

		var b bytes.Buffer
		if err := abcupdater.CheckAppVersion(ctx, &abcupdater.CheckVersionParams{
			AppID: version.Name,
			// Version: version.Version,
			Version: "0.4.0", // intentionally older than cur version to test
			Writer:  &b,
		}); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "failed to check for new versions",
				"error", err)
		}

		select {
		case updatesCh <- b.String():
		default:
		}
	}()

	return func() {
		select {
		case <-ctx.Done():
			// Context was cancelled
		case msg := <-updatesCh:
			logging.FromContext(ctx).WarnContext(ctx, msg)
		case <-time.After(time.Second):
			// Give up after some time. This is how long to wait after termination has
			// finished. The command has most likely be running for a few seconds, and
			// we don't want to block the control flow for an exceedingly long time.
			//
			// This technicall leaks both the timer and the channel, but we're about
			// to terminate, so the memory will free anyway.
		}
	}
}

func realMain(ctx context.Context) error {
	report := checkForUpdates(ctx)
	defer report()

	if runtime.GOOS == "windows" {
		return fmt.Errorf("windows os is not supported in abc cli")
	}

	return rootCmd().Run(ctx, os.Args[1:]) //nolint:wrapcheck
}
