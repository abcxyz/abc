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
	"bytes"
	"context"

	"github.com/abcxyz/abc/templates/model"
)

func actionStringReplace(ctx context.Context, sr *model.StringReplace, sp *stepParams) error {
	replaceWith, err := parseAndExecuteGoTmpl(sr.With, sp.inputs)
	if err != nil {
		return err
	}

	toReplace, err := parseAndExecuteGoTmpl(sr.ToReplace, sp.inputs)
	if err != nil {
		return err
	}

	toReplaceBuf := []byte(toReplace)
	replaceWithBuf := []byte(replaceWith)

	for _, p := range sr.Paths {
		relPath, err := parseAndExecuteGoTmpl(p, sp.inputs)
		if err != nil {
			return err
		}
		if err := walkAndModify(p.Pos, sp.fs, sp.scratchDir, relPath, func(buf []byte) ([]byte, error) {
			return bytes.ReplaceAll(buf, toReplaceBuf, replaceWithBuf), nil
		}); err != nil {
			return err
		}
	}

	return nil
}
