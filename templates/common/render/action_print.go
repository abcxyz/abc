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

package render

import (
	"context"
	"fmt"
	"strings"

	spec "github.com/abcxyz/abc/templates/model/spec/v1beta3"
)

func actionPrint(ctx context.Context, p *spec.Print, sp *stepParams) error {
	scope := sp.scope.With(sp.extraPrintVars)

	msg, err := parseAndExecuteGoTmpl(p.Message.Pos, p.Message.Val, scope)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}

	// We can ignore the int returned from Write() because the docs promise that
	// short writes always return error.
	if _, err := sp.rp.Stdout.Write([]byte(msg)); err != nil {
		return fmt.Errorf("error writing to stdout: %w", err)
	}

	return nil
}
