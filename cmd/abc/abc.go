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
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/abcxyz/abc-updater/pkg/updater"
	"github.com/abcxyz/abc/internal/version"
	"github.com/abcxyz/abc/templates/commands/describe"
	"github.com/abcxyz/abc/templates/commands/goldentest"
	"github.com/abcxyz/abc/templates/commands/render"
	"github.com/abcxyz/abc/templates/commands/upgrade"
	"github.com/abcxyz/abc/templates/common"
	"github.com/abcxyz/pkg/cli"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/sets"
)

const (
	defaultLogLevel  = logging.LevelWarning
	defaultLogFormat = logging.FormatText
)

var templateCommands = map[string]cli.CommandFactory{
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
}

// In the past, all template-related commands were under the "abc"
// subcommand because we anticipated adding more subcommands in the future. This
// never happened, and there were only template commands, so they've now been
// moved to the root. We keep the old `templates` subcommand for backward
// compatibility.
var rootCommands = sets.UnionMapKeys(templateCommands, map[string]cli.CommandFactory{
	"templates": func() cli.Command {
		return &cli.RootCommand{
			Name:        "templates",
			Description: "subcommands for rendering templates and related things",
			Commands:    templateCommands,
		}
	},
})

var rootCmd = func() *cli.RootCommand {
	return &cli.RootCommand{
		Name:     version.Name,
		Version:  version.HumanVersion,
		Commands: rootCommands,
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

		// On error, the exit code is 1 unless otherwise requested.
		exitCode := 1

		// In the special case where there's an ExitCodeErr, use that code.
		var exitErr *common.ExitCodeError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.Code
			err = exitErr.Unwrap()
		}

		if err != nil { // Could be nil if the ExitCodeErr wasn't wrapping anything
			fmt.Fprintln(os.Stderr, err.Error())
		}

		os.Exit(exitCode)
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

func realMain(ctx context.Context) error {
	if err := checkSupportedOS(); err != nil {
		return err
	}

	// Only check for updates if not built from HEAD.
	if version.Version != "source" {
		// Timeout updater after 1 second.
		updaterCtx, updaterDone := context.WithTimeout(ctx, time.Second)
		defer updaterDone()
		results := updater.CheckAppVersionAsync(updaterCtx, &updater.CheckVersionParams{
			AppID:   version.Name,
			Version: version.Version,
		})

		defer func() {
			message, err := results()
			if err != nil {
				logger := logging.FromContext(ctx)
				logger.InfoContext(ctx, "failed to check for new versions", "error", err)
				return
			}
			fmt.Fprintf(os.Stderr, "\n%s\n", message)
		}()
	}

	return rootCmd().Run(ctx, os.Args[1:]) //nolint:wrapcheck
}

func checkSupportedOS() error {
	switch runtime.GOOS {
	case "windows":
		return fmt.Errorf("windows is not supported")
	case "darwin":
		var uts unix.Utsname
		if err := unix.Uname(&uts); err != nil {
			return fmt.Errorf("unix.Uname(): %w", err)
		}
		release := unix.ByteSliceToString(uts.Release[:])
		return checkDarwinVersion(release)
	default:
		return nil
	}
}

func checkDarwinVersion(utsRelease string) error {
	// We support Mac OS 13 and newer, which corresponds to Darwin kernel
	// version 22 and newer. The mappings from macOS version to Darwin
	// version are taken from
	// https://en.wikipedia.org/wiki/Darwin_(operating_system)#Release_history.
	// Regrettably, the unix.Uname() function only gives darwin version, not
	// macos version.
	const (
		// These two must match. Whenever one is changed, the other must
		// also be changed to match.
		minDarwinVersion = 22
		minMacOSVersion  = 13 // Used only in error messages.
	)

	splits := strings.Split(utsRelease, ".")
	if len(splits) != 3 {
		return fmt.Errorf("internal error splitting macos version, got version %q", utsRelease)
	}
	majorVersion, err := strconv.Atoi(splits[0])
	if err != nil {
		return fmt.Errorf("internal error parsing macos version, got version %q", utsRelease)
	}
	if majorVersion < minDarwinVersion {
		return fmt.Errorf("your macOS version is not supported, use macOS version %d or newer (darwin kernel version %d)", minMacOSVersion, minDarwinVersion)
	}
	return nil
}
