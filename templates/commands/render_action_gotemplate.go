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

	"github.com/abcxyz/abc/templates/model"
)

func actionGoTemplate(ctx context.Context, p *model.GoTemplate, sp *stepParams) error {
	for _, p := range p.Paths {
		// Paths may contain template expressions, so render them first.
		walkRelPath, err := parseAndExecuteGoTmpl(p.Pos, p.Val, sp.inputs)
		if err != nil {
			return err
		}

		walkRelPath, err = safeRelPath(p.Pos, walkRelPath)
		if err != nil {
			return err
		}

		if err := walkAndModify(p.Pos, sp.fs, sp.scratchDir, walkRelPath, func(b []byte) ([]byte, error) {
			executed, err := parseAndExecuteGoTmpl(nil, string(b), sp.inputs)
			if err != nil {
				return nil, fmt.Errorf("failed executing file as Go template: %w", err)
			}
			return []byte(executed), nil
		}); err != nil {
			return err
		}
	}

	return nil
}
