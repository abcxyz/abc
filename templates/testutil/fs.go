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

// Package testutil contains common util functions to facilitate tests.
package testutil

import (
	"path/filepath"
	"testing"
)

func TestMustGlob(t *testing.T, glob string) (string, bool) {
	t.Helper()

	matches, err := filepath.Glob(glob)
	if err != nil {
		t.Fatalf("couldn't find template directory: %v", err)
	}
	switch len(matches) {
	case 0:
		return "", false
	case 1:
		return matches[0], true
	}
	t.Fatalf("got %d matches for glob %q, wanted 1: %s", len(matches), glob, matches)
	panic("unreachable") // silence compiler warning for "missing return"
}
