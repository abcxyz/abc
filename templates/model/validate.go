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

import (
	"errors"
	"fmt"
	"reflect"

	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"
)

func notZero[T comparable](pos *ConfigPos, x valWithPos[T], fieldName string) error {
	var zero T
	if x.Val == zero {
		return pos.AnnotateErr(fmt.Errorf("field %q is required", fieldName))
	}
	return nil
}

func nonEmptySlice[T any](pos *ConfigPos, s []T, fieldName string) error {
	if len(s) == 0 {
		return pos.AnnotateErr(fmt.Errorf("field %q is required", fieldName))
	}
	return nil
}

// Fail if any unexpected fields are seen. The input must be a mapping/object; anything else will
// result in an error.
// This is a workaround for the brokenness of KnownFields in the upstream yaml lib
// (https://github.com/go-yaml/yaml/issues/460).
func extraFields(n *yaml.Node, knownFields []string) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("got yaml node of kind %d, expected %d", n.Kind, yaml.MappingNode)
	}
	m := map[string]any{}
	if err := n.Decode(m); err != nil {
		return err //nolint:wrapcheck
	}

	var unknownField string
	for k := range m {
		if !slices.Contains(knownFields, k) {
			unknownField = k
			break
		}
	}

	if unknownField == "" {
		return nil
	}

	// Now we have to find the position within the YAML of the unknown field.
	pos := yamlPos(n) // Fallback is to report position of parent node
	// This can return a false location if some other token in the token stream
	// happens to be equal to unknownField, but that should be rare. The
	// consequences are not serious, just a mis-reported position.
	for _, c := range n.Content {
		if c.Value == unknownField {
			pos = yamlPos(c)
		}
	}

	return pos.AnnotateErr(fmt.Errorf("unknown field name %q", unknownField))
}

type validator interface {
	Validate() error
}

// validateIfNotNil is intended to be used in a model Validate() method.
// Semantically it means "if this model field is present (non-nil), then
// validate it. If not present, then skip validation. This is useful for
// polymorphic models like Step that have many possible child types, only one
// of which will be set.
func validateIfNotNil(v validator) error {
	if v == nil || reflect.ValueOf(v).IsNil() {
		return nil
	}
	return v.Validate() //nolint:wrapcheck
}

func validateEach[T validator](s []T) error {
	var merr error
	for _, v := range s {
		merr = errors.Join(merr, v.Validate())
	}

	return merr
}
