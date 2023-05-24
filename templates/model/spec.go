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

// Package model contains the structs for unmarshaled YAML files.
//
//nolint:wrapcheck // We don't want to excessively wrap errors, like "yaml error: yaml error: ..."
package model

// Notes for maintainers explaining the confusing stuff in this file:
//
// Q1. Why do we need to override UnmarshalYAML()?
//    A. We want to save the location within the YAML file of each object, so
//       that we can return helpful error messages that point to the problem.
//    A. We want to reject unrecognized fields. Due to a known issue in yaml.v3
//       (https://github.com/go-yaml/yaml/issues/460), this feature doesn't
//       work in some situations. So we have to implement it ourselves.
//    A. In the case of the Step struct, we want to do polymorphic decoding
//       based on the value of the "action" field.
//
// Q2. What's up with the weird "shadow" unmarshalling pattern (ctrl-f "shadow")
// A. We often want to "unmarshal all the fields of my struct, but also run
//    our own logic before and after unmarshaling." To do this, we have to
//    create a separate type that *doesn't* implement yaml.Unmarshaler, and
//    unmarshal into that first. If we didn't do this, we'd get infinite
//    recursion where UnmarshalYAML invokes Decode which invokes UnmarshalYAML
//    which invokes Decode ...infinitely. See e.g.
//    https://github.com/go-yaml/yaml/issues/107#issuecomment-524681153.
//
// Q3. Why is validation done as a separate pass instead of in UnmarshalYAML()?
// A. Because there's a very specific edge case that we need to avoid.
//    UnmarshalYAML() is only called for YAML objects that have at least one
//    field that's specified in the input YAML. This can happen if an object
//    relies on default values or has no parameters. But we still want to
//    validate every object. So we need to run validation separately from
//    unmarshaling.

import (
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// NewDecoder returns a yaml Decoder with the desired options.
func NewDecoder(r io.Reader) *yaml.Decoder {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true) // Fail if any unexpected fields are seen. Often doesn't work: https://github.com/go-yaml/yaml/issues/460
	return dec
}

// Spec represents a parsed spec.yaml file describing a template.
type Spec struct {
	// Pos is the YAML file location where this object started.
	Pos *ConfigPos `yaml:"-"`

	APIVersion String `yaml:"apiVersion"`
	Kind       String `yaml:"kind"`

	Desc   String   `yaml:"desc"`
	Inputs []*Input `yaml:"inputs"`
	Steps  []*Step  `yaml:"steps"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *Spec) UnmarshalYAML(n *yaml.Node) error {
	knownYAMLFields := []string{"apiVersion", "kind", "desc", "inputs", "steps"}
	if err := extraFields(n, knownYAMLFields); err != nil {
		return err
	}

	type shadowType Spec
	shadow := &shadowType{} // see "Q2" in file comment above
	if err := n.Decode(shadow); err != nil {
		return err
	}

	*s = Spec(*shadow)
	s.Pos = yamlPos(n)
	return nil
}

// Validate implements Validator.
func (s *Spec) Validate() error {
	return errors.Join(
		notZero(s.Pos, s.APIVersion, "apiVersion"),
		notZero(s.Pos, s.Kind, "kind"),
		notZero(s.Pos, s.Desc, "desc"),
		nonEmptySlice(s.Pos, s.Steps, "steps"),
		validateEach(s.Inputs),
		validateEach(s.Steps),
	)
}

// Input represents one of the parsed "input" fields from the spec.yaml file.
type Input struct {
	// Pos is the YAML file location where this object started.
	Pos *ConfigPos `yaml:"-"`

	Name     String `yaml:"name"`
	Desc     String `yaml:"desc"`
	Required Bool   `yaml:"required"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (i *Input) UnmarshalYAML(n *yaml.Node) error {
	knownYAMLFields := []string{"name", "desc", "required"}
	if err := extraFields(n, knownYAMLFields); err != nil {
		return err
	}

	// Unmarshal with default values
	type shadowType Input
	shadow := &shadowType{ // unmarshal into a type that doesn't have UnmarshalYAML
		Required: Bool{Val: true},
	}

	if err := n.Decode(shadow); err != nil {
		return err
	}

	*i = Input(*shadow)
	i.Pos = yamlPos(n)

	return nil
}

// Validate implements Validator.
func (i *Input) Validate() error {
	return errors.Join(
		notZero(i.Pos, i.Name, "name"),
		notZero(i.Pos, i.Desc, "desc"),
	)
}

// Step represents one of the work steps involved in rendering a template.
type Step struct {
	// Pos is the YAML file location where this object started.
	Pos *ConfigPos `yaml:"-"`

	Desc   String `yaml:"desc"`
	Action String `yaml:"action"`

	// Each action type has a field below. Only one of these will be set.
	Print   *Print   `yaml:"-"`
	Include *Include `yaml:"-"`

	// TODO: add more action types:
	// RegexReplace  *RegexReplace  `yaml:"-"`
	// StringReplace *StringReplace `yaml:"-"`
	// GoTemplate    *GoTemplate    `yaml:"-"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *Step) UnmarshalYAML(n *yaml.Node) error {
	knownYAMLFields := []string{"desc", "action", "params"}
	if err := extraFields(n, knownYAMLFields); err != nil {
		return err
	}

	type shadowType Step
	shadow := &shadowType{} // see "Q2" in file comment above
	if err := n.Decode(shadow); err != nil {
		return err
	}

	*s = Step(*shadow)
	s.Pos = yamlPos(n)

	// The rest of this function just unmarshals the "params" field into the correct struct type depending
	// on the value of "action".
	var unmarshalInto any
	switch s.Action.Val {
	case "print":
		s.Print = new(Print)
		unmarshalInto = s.Print
		s.Print.Pos = s.Pos // Set an approximate position in case yaml unmarshaling fails later
	case "include":
		s.Include = new(Include)
		unmarshalInto = s.Include
		s.Include.Pos = s.Pos
	case "":
		return s.Pos.AnnotateErr(fmt.Errorf(`missing "action" field in this step`))
	default:
		return s.Pos.AnnotateErr(fmt.Errorf("unknown action type %q", s.Action.Val))
	}

	params := struct {
		Params yaml.Node `yaml:"params"`
	}{}
	if err := n.Decode(&params); err != nil {
		return err
	}
	if err := params.Params.Decode(unmarshalInto); err != nil {
		return err
	}
	return nil
}

// Validate implements Validator.
func (s *Step) Validate() error {
	// The "action" field is implicitly validated by UnmarshalYAML, so not included here.
	return errors.Join(
		notZero(s.Pos, s.Desc, "desc"),
		validateUnlessNil(s.Print),
		validateUnlessNil(s.Include),
		// TODO: add more action types:
		// validateIfNotNil(s.RegexReplace),
		// validateIfNotNil(s.StringReplace),
		// validateIfNotNil(s.GoTemplate),
	)
}

// Print is an action that prints a message to standard output.
type Print struct {
	// Pos is the YAML file location where this object started.
	Pos *ConfigPos `yaml:"-"`

	Message String `yaml:"message"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (p *Print) UnmarshalYAML(n *yaml.Node) error {
	knownYAMLFields := []string{"message"}
	if err := extraFields(n, knownYAMLFields); err != nil {
		return err
	}

	type shadowType Print
	shadow := &shadowType{} // see "Q2" in file comment above

	if err := n.Decode(shadow); err != nil {
		return err
	}

	*p = Print(*shadow)
	p.Pos = yamlPos(n)
	return nil
}

// Validate implements Validator.
func (p *Print) Validate() error {
	return errors.Join(
		notZero(p.Pos, p.Message, "message"),
	)
}

// Include is an action that places files into the output directory.
type Include struct {
	// Pos is the YAML file location where this object started.
	Pos *ConfigPos `yaml:"-"`

	Paths []String `yaml:"paths"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (i *Include) UnmarshalYAML(n *yaml.Node) error {
	knownYAMLFields := []string{"paths"}
	if err := extraFields(n, knownYAMLFields); err != nil {
		return err
	}
	type shadowType Include
	shadow := &shadowType{} // see "Q2" in file comment above

	if err := n.Decode(shadow); err != nil {
		return err
	}
	*i = Include(*shadow)
	i.Pos = yamlPos(n)

	return nil
}

// Validate implements Validator.
func (i *Include) Validate() error {
	return errors.Join(
		nonEmptySlice(i.Pos, i.Paths, "paths"),
	)
}
