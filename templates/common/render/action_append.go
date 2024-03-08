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
	"strings"

	"github.com/abcxyz/abc/templates/common/render/gotmpl"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta6"
)

func actionAppend(ctx context.Context, ap *spec.Append, sp *stepParams) error {
	with, err := gotmpl.ParseExec(ap.With.Pos, ap.With.Val, sp.scope)
	if err != nil {
		return err //nolint:wrapcheck
	}

	if !ap.SkipEnsureNewline.Val {
		if !strings.HasSuffix(with, "\n") {
			with = with + "\n"
		}
	}

	if err := walkAndModify(ctx, sp, ap.Paths, func(buf []byte) ([]byte, error) {
		return append(buf, []byte(with)...), nil
	}); err != nil {
		return err
	}

	return nil
}
