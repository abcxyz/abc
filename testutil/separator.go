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

package testutil

import "path/filepath"

// KeysToPlatformPaths converts the keys from slash-separated paths to the local
// OS file separator. The maps are modified in place.
func KeysToPlatformPaths[T any](maps ...map[string]T) {
	for _, m := range maps {
		for k, v := range m {
			if newKey := filepath.FromSlash(k); newKey != k {
				m[newKey] = v
				delete(m, k)
			}
		}
	}
}

// ToPlatformPaths converts each input from slash-separated paths to the local
// OS file separator. The slices are modified in place.
func ToPlatformPaths(slices ...[]string) {
	for _, s := range slices {
		for i := range s {
			s[i] = filepath.FromSlash(s[i])
		}
	}
}
