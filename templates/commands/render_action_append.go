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

	"github.com/abcxyz/abc/templates/model"
)

func actionAppend(ctx context.Context, ap *model.Append, sp *stepParams) error {
	path, err := parseAndExecuteGoTmpl(ap.Path.Pos, ap.Path.Val, sp.inputs)
	if err != nil {
		return err
	}

	with, err := parseAndExecuteGoTmpl(ap.With.Pos, ap.With.Val, sp.inputs)
	if err != nil {
		return err
	}

	if err := walkAndModify(ctx, ap.Path.Pos, sp.fs, sp.scratchDir, path, func(buf []byte) ([]byte, error) {
		// todo: should we add a newline before/after appending or leave that to user?
		return append(buf, []byte(with)...), nil
	}); err != nil {
		return err
	}

	return nil
}
