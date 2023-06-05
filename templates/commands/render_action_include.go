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
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/abcxyz/abc/templates/model"
)

func actionInclude(ctx context.Context, i *model.Include, sp *stepParams) error {
	for _, p := range i.Paths {
		// Paths may contain template expressions, so render them first.
		walkRelPath, err := parseAndExecuteGoTmpl(p, sp.inputs)
		if err != nil {
			return model.ErrWithPos(p.Pos, `error compiling go-template: %w`, err)
		}

		if err := safeRelPath(p.Val); err != nil {
			return model.ErrWithPos(p.Pos, "invalid path: %w", err)
		}

		absSrc := filepath.Join(sp.templateDir, walkRelPath)
		absDst := filepath.Join(sp.scratchDir, walkRelPath)

		if _, err := sp.fs.Stat(absSrc); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return model.ErrWithPos(p.Pos, "include path doesn't exist: %q", walkRelPath)
			}
			return fmt.Errorf("Stat(): %w", err)
		}

		if err := copyRecursive(p.Pos, absSrc, absDst, sp.fs); err != nil {
			return model.ErrWithPos(p.Pos, "copying failed: %w", err)
		}
	}
	return nil
}
