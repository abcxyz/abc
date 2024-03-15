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

package manifest

import (
	manifest "github.com/abcxyz/abc/templates/model/manifest/v1alpha1"
)

// HashesAsMap transforms the list of OutputHashes into a map of path->hash.
func HashesAsMap(hs []*manifest.OutputHash) map[string]string {
	out := make(map[string]string, len(hs))
	for _, entry := range hs {
		out[entry.File.Val] = entry.Hash.Val
	}
	return out
}