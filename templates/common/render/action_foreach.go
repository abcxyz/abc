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

	"github.com/abcxyz/abc/templates/common"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta4"
)

func actionForEach(ctx context.Context, fe *spec.ForEach, sp *stepParams) error {
	key, err := parseAndExecuteGoTmpl(fe.Iterator.Key.Pos, fe.Iterator.Key.Val, sp.scope)
	if err != nil {
		return err
	}

	var values []string
	if len(fe.Iterator.Values) > 0 {
		var err error
		values, err = parseAndExecuteGoTmplAll(fe.Iterator.Values, sp.scope)
		if err != nil {
			return err
		}
	} else {
		if err := common.CelCompileAndEval(ctx, sp.scope, *fe.Iterator.ValuesFrom, &values); err != nil {
			return err //nolint:wrapcheck
		}
	}

	for _, keyVal := range values {
		subStepParams := sp.WithScope(map[string]string{key: keyVal})
		if err := executeSteps(ctx, fe.Steps, subStepParams); err != nil {
			return err
		}
	}

	return nil
}
