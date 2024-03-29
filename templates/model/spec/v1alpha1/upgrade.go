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

package v1alpha1

import (
	"context"
	"fmt"

	"github.com/jinzhu/copier"

	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/abc/templates/model/spec/v1beta1"
)

// Upgrade implements model.ValidatorUpgrader.
func (s *Spec) Upgrade(context.Context) (model.ValidatorUpgrader, error) {
	var out v1beta1.Spec
	// The only difference between schema v1alpha1 and v1beta1 is the addition of the "if"
	// field in the "step" struct. So a straight-across copy is sufficient.
	if err := copier.Copy(&out, s); err != nil {
		return nil, fmt.Errorf("failed upgrading spec from v1alpha1 to v1beta1: %w", err)
	}
	return &out, nil
}
