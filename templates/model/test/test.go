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

package test

import (
	"errors"
	"io"

	"github.com/abcxyz/abc/templates/model"
	"gopkg.in/yaml.v3"
)

// This file parses a YAML file that describes test configs.

// InputValue represents one of the parsed "input" fields from the inputs.yaml file.
type InputValue struct {
	// Pos is the YAML file location where this object started.
	Pos model.ConfigPos `yaml:"-"`

	Name  model.String `yaml:"name"`
	Value model.String `yaml:"value"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (i *InputValue) UnmarshalYAML(n *yaml.Node) error {
	return model.UnmarshalPlain(n, i, &i.Pos) //nolint:wrapcheck
}

func (i *InputValue) Validate() error {
	return errors.Join(
		model.NotZeroModel(&i.Pos, i.Name, "name"),
		model.NotZeroModel(&i.Pos, i.Value, "value"),
	)
}

// Test represents a parsed test.yaml describing test configs.
type Test struct {
	// Pos is the YAML file location where this object started.
	Pos model.ConfigPos `yaml:"-"`

	APIVersion model.String  `yaml:"api_version"`
	Inputs     []*InputValue `yaml:"inputs"`
}

// Validate implements model.Validator.
func (t *Test) Validate() error {
	return errors.Join(
		model.IsKnownSchemaVersion(&t.Pos, t.APIVersion, "api_version"),
		model.ValidateEach(t.Inputs),
	)
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (t *Test) UnmarshalYAML(n *yaml.Node) error {
	return model.UnmarshalPlain(n, t, &t.Pos) //nolint:wrapcheck
}

// DecodeTest unmarshals the YAML Spec from r.
func DecodeTest(r io.Reader) (*Test, error) {
	var test Test
	if err := model.DecodeAndValidate(r, "test", &test); err != nil {
		return &test, err //nolint:wrapcheck
	}
	return &test, nil
}