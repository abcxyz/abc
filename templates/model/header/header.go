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

// Package header helps with marshaling and unmarshaling YAML structs together
// with header fields.
package header

import "github.com/abcxyz/abc/templates/model"

// Fields is the set of fields that are present in every "kind" of YAML file
// used in this program.
type Fields struct {
	// When marshaling, use NewStyleAPIVersion and not OldStyleAPIVersion.
	NewStyleAPIVersion model.String `yaml:"api_version,omitempty"`
	// OldStyleAPIVersion only exists to handle a legacy naming apiVersion; the
	// new, more correct name is "api_version".
	OldStyleAPIVersion model.String `yaml:"apiVersion,omitempty"`

	// One of "Template", "GoldenTest", etc.
	Kind model.String `yaml:"kind"`
}

// With wraps any type in a struct that contains header fields. The header
// fields and payload fields are inlined together in the YAML output. This is
// intended to be used when marshaling a struct for output to a file, because
// our model structs don't have the "api_version" and "kind" fields.
type With[T any] struct {
	Header  *Fields `yaml:",inline"`
	Wrapped T       `yaml:",inline"`
}
