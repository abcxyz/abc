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

// Package commands implements the template-related subcommands.
package commands

// This file implements the "templates test record" subcommand for
// recording template tests.

import (
	"context"
	"fmt"

	"github.com/abcxyz/pkg/cli"
)

type TestRecordCommand struct {
	cli.BaseCommand
}

func (c *TestRecordCommand) Desc() string {
	return "record the template rendering result to golden tests"
}

func (c *TestRecordCommand) Help() string {
	return `
Usage: {{ COMMAND }} [options] <test_name>

The {{ COMMAND }} records the template golden tests.

The "<test_name>" is the name of the test. If no <test_name> is specified,
all tests will be recoreded.

For every test case, it is expected that
  - a testdata/golden/<test_name> folder exists to host test results.
  - a testdata/golden/<test_name>/inputs.yaml exists to define
template input params.`
}

func (c *TestRecordCommand) Run(ctx context.Context, args []string) error {
	return fmt.Errorf("Unimplemented")
}