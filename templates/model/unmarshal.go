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
// Q. Why do we need to override UnmarshalYAML()?
//    A. We want to save the location within the YAML file of each object, so
//       that we can return helpful error messages that point to the problem.
//    A. We want to reject unrecognized fields. Due to a known issue in yaml.v3
//       (https://github.com/go-yaml/yaml/issues/460), this feature doesn't
//       work in some situations. So we have to implement it ourselves.
//    A. In the case of the Step struct, we want to do polymorphic decoding
//       based on the value of the "action" field.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// UnmarshalPlain unmarshals the yaml node n into the struct pointer outPtr, as
// if it did not have an UnmarshalYAML method. This lets you still use the
// default unmarshaling logic to populate the fields of your struct, while
// adding custom logic before and after.
//
// outPtr must be a pointer to a struct which will be modified by this function.
//
// pos will be modified by this function to contain the position of this yaml
// node within the input file.
//
// The `yaml:"..."` tags of the outPtr struct are used to determine the set of
// valid fields. Unexpected fields in the yaml are treated as an error. To allow
// extra yaml fields that don't correspond to a field of outPtr, provide their
// names in extraYAMLFields. This allows some fields to be handled specially.
func UnmarshalPlain(n *yaml.Node, outPtr any, outPos *ConfigPos, extraYAMLFields ...string) error {
	fields := reflect.VisibleFields(reflect.TypeOf(outPtr).Elem())

	// Calculate the set of allowed/known field names in the YAML.
	yamlFieldNames := make([]string, 0, len(fields)+len(extraYAMLFields))
	for _, field := range fields {
		commaJoined := field.Tag.Get("yaml")
		key, _, _ := strings.Cut(commaJoined, ",")
		if key == "" || key == "-" {
			continue
		}
		yamlFieldNames = append(yamlFieldNames, key)
	}

	yamlFieldNames = append(yamlFieldNames, extraYAMLFields...)
	if err := extraFields(n, yamlFieldNames); err != nil {
		// Reject unexpected fields.
		return err
	}

	// Warning: here be dragons.
	//
	// To avoid calling the UnmarshalYAML field of outPtr, which would cause
	// infinite recursion, we'll unmarshal into a new struct. This new struct is
	// not the same type as outPtr, it is a dynamically-created type with the
	// same set of fields, but with no methods, and therefore no UnmarshalYAML
	// method.
	typeWithoutMethods := reflect.StructOf(fields)
	shadow := reflect.New(typeWithoutMethods)

	if err := n.Decode(shadow.Interface()); err != nil {
		return err //nolint:wrapcheck
	}
	// Copy the field values from the dynamically-created-type-without-methods
	// to the actual output struct.
	reflect.ValueOf(outPtr).Elem().Set(shadow.Elem())

	*outPos = *YAMLPos(n)
	return nil
}

// DecodeAndValidate unmarshals the YAML text in the given Reader into the given
// pointer-to-struct, and calls Validate() on it. Returns any unmarshaling error
// or validation error.
func DecodeAndValidate(r io.Reader, filename string, outPtr Validator) error {
	dec := yaml.NewDecoder(r)
	if err := dec.Decode(outPtr); err != nil {
		return fmt.Errorf("error parsing %s YAML file: %w", filename, err)
	}
	if err := outPtr.Validate(); err != nil {
		return fmt.Errorf("validation failed in %s YAML file: %w", filename, err)
	}
	return nil
}

// ValidatorUpgrader is the interface implemented by every kind of YAML struct
// (templates, golden tests, manifests, etc).
type ValidatorUpgrader interface {
	Validator

	// Upgrade converts an old version of a YAML struct into newer version for
	// example, from api_version v1 to v2. If the struct is already the most
	// recent version, it returns (nil,LatestVersion).
	//
	// An error other than LatestVersion means that the model cannot be converted
	// because either:
	//
	//   1. something weird happened
	//   2. the YAML struct uses features in an old version that are not supported in newer
	//      versions.
	Upgrade(context.Context) (ValidatorUpgrader, error)
}

// ErrLatestVersion is a sentinel error returned from Upgrade() meaning that there
// is no further upgrading to be done because the current version is already the
// latest version.
var ErrLatestVersion = errors.New("this is the latest version")
