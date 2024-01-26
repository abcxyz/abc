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

package header

import "github.com/abcxyz/abc/templates/model"

// Fields is the set of fields that are present in every "kind" of YAML file
// used in this program.
type Fields struct {
	OldStyleAPIVersion model.String `yaml:"apiVersion,omitempty"`
	NewStyleAPIVersion model.String `yaml:"api_version,omitempty"`
	Kind               model.String `yaml:"kind"`
}
