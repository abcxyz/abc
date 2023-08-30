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
	"io"

	"gopkg.in/yaml.v3"
)

// This file parses a YAML file that describes template input.

// InputValue represents one of the parsed "input" fields from the inputs.yaml file.
type InputValue struct {
	// Pos is the YAML file location where this object started.
	Pos ConfigPos `yaml:"-"`

	Name  String `yaml:"name"`
	Value String `yaml:"value"`
}

type Inputs struct {
	// Pos is the YAML file location where this object started.
	Pos ConfigPos `yaml:"-"`

	Inputs []*InputValue `yaml:"inputs"`
}

// DecodeInputs unmarshals the YAML Spec from r.
func DecodeInputs(r io.Reader) (*Inputs, error) {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)

	var inputs Inputs
	if err := dec.Decode(&inputs); err != nil {
		return nil, fmt.Errorf("error parsing YAML inputs file: %w", err)
	}

	return &inputs, validateEach(inputs.Inputs)
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (i *Inputs) UnmarshalYAML(n *yaml.Node) error {
	if err := unmarshalPlain(n, i, &i.Pos); err != nil {
		return err
	}
	return nil
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
