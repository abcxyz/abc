// Copyright 2024 The Authors (see AUTHORS file)
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

package features

// Features controls which code paths are enabled in abc goldentest by a given api_version.
//
// These should true for old api_versions that don't support the feature, and
// false for new api_versions that do support the feature. This will allow the
// most recent schema to have all booleans false (all features enabled) without
// undergoing any special transformation. Older schemas will have features
// disabled (booleans set to true) in their Upgrade() method.
type Features struct {
	// SkipStdout determines whether to verify stdout or not.
	// New in github.com/abcxyz/abc/templates/model/goldentest/v1beta4.
	SkipStdout bool

	// SkipABCRenamed determines whether to rename git/github related dirs/files or not.
	// New in github.com/abcxyz/abc/templates/model/goldentest/v1beta4.
	SkipABCRenamed bool
}
