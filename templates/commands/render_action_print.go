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
	"context"
	"fmt"
	"strings"

	"github.com/abcxyz/abc/templates/model"
)

func actionPrint(ctx context.Context, p *model.Print, sp *stepParams) error {
	inputsAndFlags := map[string]any{}
	for k, v := range sp.inputs {
		inputsAndFlags[k] = v
	}
	inputsAndFlags["flags"] = flagsForTemplate(sp.flags)
	msg, err := parseAndExecuteGoTmpl(p.Message.Pos, p.Message.Val, inputsAndFlags)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}

	// We can ignore the int returned from Write() because the docs promise that
	// short writes always return error.
	if _, err := sp.stdout.Write([]byte(msg)); err != nil {
		return fmt.Errorf("error writing to stdout: %w", err)
	}

	return nil
}

func flagsForTemplate(r *renderFlags) map[string]any {
	// We only expose certain fields the print action; these are the ones that
	// we have beneficial use cases for and that don't encourage bad API use.
	return map[string]any{
		"dest":   r.dest,
		"source": r.source,
		"spec":   r.spec,
	}
}
