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

package common

// FirstNonZero returns the first element of s that is not the zero value for
// its type. It returns the zero value if all elements of s are zero or if s is
// empty.
func FirstNonZero[T comparable](s ...T) T {
	var zero T
	for _, e := range s {
		if e != zero {
			return e
		}
	}
	return zero
}
