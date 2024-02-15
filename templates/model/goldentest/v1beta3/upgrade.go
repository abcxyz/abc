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

package goldentest

import (
	"context"
	"fmt"

	"github.com/jinzhu/copier"

	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/abc/templates/model/goldentest/features"
	v1beta4 "github.com/abcxyz/abc/templates/model/goldentest/v1beta4"
)

// Upgrade implements model.ValidatorUpgrader.
func (t *Test) Upgrade(ctx context.Context) (model.ValidatorUpgrader, error) {
	var out v1beta4.Test

	if err := copier.Copy(&out, t); err != nil {
		return nil, fmt.Errorf("internal error: failed upgrading spec from v1beta3 to v1beta4: %w", err)
	}
	out.Features = features.Features{
		SkipStdout:     true,
		SkipABCRenamed: true,
	}

	return &out, nil
}
