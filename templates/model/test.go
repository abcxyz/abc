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

//nolint:wrapcheck // We don't want to excessively wrap errors, like "yaml error: yaml error: ..."
package model

import (
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// This file parses a YAML file that describes test configs.

// InputValue represents one of the parsed "input" fields from the inputs.yaml file.
type InputValue struct {
	// Pos is the YAML file location where this object started.
	Pos ConfigPos `yaml:"-"`

	Name  String `yaml:"name"`
	Value String `yaml:"value"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (i *InputValue) UnmarshalYAML(n *yaml.Node) error {
	if err := unmarshalPlain(n, i, &i.Pos); err != nil {
		return err
	}
	return nil
}

func (i *InputValue) Validate() error {
	return errors.Join(
		notZeroModel(&i.Pos, i.Name, "name"),
		notZeroModel(&i.Pos, i.Value, "value"),
	)
}

// Test represents a parsed test.yaml describing test configs.
type Test struct {
	// Pos is the YAML file location where this object started.
	Pos ConfigPos `yaml:"-"`

	APIVersion String `yaml:"apiVersion"`

	Inputs []*InputValue `yaml:"inputs"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (i *Test) UnmarshalYAML(n *yaml.Node) error {
	if err := unmarshalPlain(n, i, &i.Pos); err != nil {
		return err
	}
	return nil
}

// DecodeTest unmarshals the YAML Spec from r.
func DecodeTest(r io.Reader) (*Test, error) {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)

	var test Test
	if err := dec.Decode(&test); err != nil {
		return nil, fmt.Errorf("error parsing test YAML file: %w", err)
	}

	return &test, errors.Join(
		oneOf(&test.Pos, test.APIVersion, []string{"cli.abcxyz.dev/v1alpha1"}, "apiVersion"),
		validateEach(test.Inputs),
	)
}
