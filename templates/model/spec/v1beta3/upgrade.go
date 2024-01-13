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

package v1beta3

import (
	"context"

	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/pkg/logging"
)

// Upgrade implements model.ValidatorUpgrader.
func (s *Spec) Upgrade(ctx context.Context) (model.ValidatorUpgrader, error) {
	logger := logging.FromContext(ctx).With("logger", "Upgrade")
	logger.DebugContext(ctx, "finished upgrading spec model, this is the most recent version")

	// Uncomment this when there's a version after v1beta3.
	// var out nextversion.Spec
	// if err := copier.Copy(&out, s); err != nil {
	// 	return nil, fmt.Errorf("internal error: failed upgrading spec from v1beta2 to v1beta3: %w", err)
	// }
	// // If this spec was upgraded from an older api_version, disable the features
	// // that weren't supported in its declared api_version.
	// out.Features = s.Features

	// // Features introduced in v1beta4:
	// out.Features.SkipFoo = true

	return nil, model.ErrLatestVersion
}
