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

package model

// This file contains boxed primitive data types that are part of a data model struct.
// It's theoretically extensible beyond YAML.

import "gopkg.in/yaml.v3"

// String represents a string field in a model, together with its location in the input file.
type String = valWithPos[string]

// Bool represents a boolean field in a model, together with its location in the input file.
type Bool = valWithPos[bool]

// valWithPos unmarshals a type T from YAML, and adds on the location in the YAML doc that it came
// from. This allows helpful error messages.
type valWithPos[T any] struct {
	Val T
	Pos ConfigPos
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (v *valWithPos[T]) UnmarshalYAML(n *yaml.Node) error {
	if err := n.Decode(&v.Val); err != nil { 
		return err //nolint:wrapcheck
	}
	v.Pos = yamlPos(n)
	return nil
}
