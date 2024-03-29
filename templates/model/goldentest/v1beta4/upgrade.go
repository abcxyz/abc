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

	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/pkg/logging"
)

// Upgrade implements model.ValidatorUpgrader.
func (t *Test) Upgrade(ctx context.Context) (model.ValidatorUpgrader, error) {
	logger := logging.FromContext(ctx).With("logger", "Upgrade")
	logger.DebugContext(ctx, "finished upgrading goldentest model, this is the most recent version")

	return nil, model.ErrLatestVersion
}
