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

// Notes for maintainers explaining the confusing stuff in this file:
//
// Q. Why is validation done as a separate pass instead of in UnmarshalYAML()?
// A. Because there's a very specific edge case that we need to avoid.
//    UnmarshalYAML() is only called for YAML objects that have at least one
//    field that's specified in the input YAML. This can happen if an object
//    relies on default values or has no parameters. But we still want to
//    validate every object. So we need to run validation separately from
//    unmarshaling.

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"

	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"
)

// NotZeroModel returns error if the given model's value is equal to the zero value for type T.
func NotZeroModel[T comparable](pos *ConfigPos, x valWithPos[T], fieldName string) error {
	return NotZero(pos, x.Val, fieldName)
}

// NotZero returns error if the given value is equal to the zero value for type T.
func NotZero[T comparable](pos *ConfigPos, t T, fieldName string) error {
	var zero T
	if t == zero {
		return pos.Errorf("field %q is required", fieldName)
	}
	return nil
}

// NonEmptySlice returns error if the given slice is empty.
func NonEmptySlice[T any](pos *ConfigPos, s []T, fieldName string) error {
	if len(s) == 0 {
		return pos.Errorf("field %q is required", fieldName)
	}
	return nil
}

// In a regex, the groupname in (?P<groupname>re) must be a letter followed by zero
// or more alphanumerics.
var validRegexGroupName = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9]*`)

// IsValidRegexGroupName returns whether the given string will be accepted by
// the regexp library as an RE2 group name.
func IsValidRegexGroupName(s String, fieldName string) error {
	if !validRegexGroupName.MatchString(s.Val) {
		return s.Pos.Errorf("subgroup name must be a letter followed by zero or more alphanumerics")
	}
	return nil
}

// OneOf returns error if x.Val is not one of the given allowed choices.
func OneOf[T comparable](pos *ConfigPos, x valWithPos[T], allowed []T, fieldName string) error {
	if slices.Contains(allowed, x.Val) {
		return nil
	}
	return pos.Errorf("field %q value must be one of %v", fieldName, allowed)
}

// extrafields returns error if any unexpected fields are seen. The input must
// be a mapping/object; anything else will result in an error. This is a
// workaround for a bug in KnownFields in the upstream yaml lib
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
	pos := YAMLPos(n) // Fallback is to report position of parent node
	// This can return a false location if some other token in the token stream
	// happens to be equal to unknownField, but that should be rare. The
	// consequences are not serious, just a mis-reported position.
	for _, c := range n.Content {
		if c.Value == unknownField {
			pos = YAMLPos(c)
		}
	}

	return pos.Errorf("unknown field name %q; valid choices are %v", unknownField, knownFields)
}

// Validator is any model struct that has a validate method. It's useful for
// "higher order" validation functions like "validate each entry in a list".
type Validator interface {
	Validate() error
}

// ValidateUnlessNil is intended to be used in a model Validate() method.
// Semantically it means "if this model field is present (non-nil), then
// validate it. If not present, then skip validation." This is useful for
// polymorphic models like Step that have many possible child types, only one
// of which will be set.
func ValidateUnlessNil(v Validator) error {
	if v == nil || reflect.ValueOf(v).IsNil() {
		return nil
	}
	return v.Validate() //nolint:wrapcheck
}

// ValidateEach calls Validate() on each element of the input and returns all
// errors encountered.
func ValidateEach[T Validator](s []T) error {
	var merr error
	for _, v := range s {
		merr = errors.Join(merr, v.Validate())
	}

	return merr
}
