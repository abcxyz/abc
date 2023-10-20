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

package common

import (
	"path/filepath"
	"strings"

	"github.com/abcxyz/abc/templates/model"
)

// SafeRelPath returns an error if the path contains a ".." traversal, and
// converts it to a relative path by removing any leading "/".
func SafeRelPath(pos *model.ConfigPos, p string) (string, error) {
	if strings.Contains(p, "..") {
		return "", pos.Errorf(`path %q must not contain ".."`, p)
	}
	return strings.TrimLeft(p, string(filepath.Separator)), nil
}
