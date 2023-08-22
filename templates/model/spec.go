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
	"reflect"
	"strings"

	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"
)

// DecodeSpec unmarshals the YAML Spec from r. This function exists so we can
// validate the Spec model before providing it to the caller; we don't want the
// caller to forget, and thereby introduce bugs.
//
// If the Spec parses successfully but then fails validation, the spec will be
// returned along with the validation error.
func DecodeSpec(r io.Reader) (*Spec, error) {
	dec := newDecoder(r)
	var spec Spec
	if err := dec.Decode(&spec); err != nil {
		return nil, fmt.Errorf("error parsing YAML spec file: %w", err)
	}
	return &spec, spec.Validate()
}

// newDecoder returns a yaml Decoder with the desired options.
func newDecoder(r io.Reader) *yaml.Decoder {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true) // Fail if any unexpected fields are seen. Often doesn't work: https://github.com/go-yaml/yaml/issues/460
	return dec
}

// Spec represents a parsed spec.yaml file describing a template.
type Spec struct {
	// Pos is the YAML file location where this object started.
	Pos ConfigPos `yaml:"-"`

	APIVersion String `yaml:"apiVersion"`
	Kind       String `yaml:"kind"`

	Desc   String   `yaml:"desc"`
	Inputs []*Input `yaml:"inputs"`
	Steps  []*Step  `yaml:"steps"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *Spec) UnmarshalYAML(n *yaml.Node) error {
	if err := unmarshalPlain(n, s, &s.Pos); err != nil {
		return err
	}
	return nil
}

// Validate implements Validator.
func (s *Spec) Validate() error {
	return errors.Join(
		oneOf(&s.Pos, s.APIVersion, []string{"cli.abcxyz.dev/v1alpha1"}, "apiVersion"),
		oneOf(&s.Pos, s.Kind, []string{"Template"}, "kind"),
		notZeroModel(&s.Pos, s.Desc, "desc"),
		nonEmptySlice(&s.Pos, s.Steps, "steps"),
		validateEach(s.Inputs),
		validateEach(s.Steps),
	)
}

// Input represents one of the parsed "input" fields from the spec.yaml file.
type Input struct {
	// Pos is the YAML file location where this object started.
	Pos ConfigPos `yaml:"-"`

	Name    String      `yaml:"name"`
	Desc    String      `yaml:"desc"`
	Default *String     `yaml:"default,omitempty"`
	Rules   []InputRule `yaml:"rules"`
}

// unmarshalPlain unmarshals the yaml node n into the struct pointer outPtr, as
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
func unmarshalPlain(n *yaml.Node, outPtr any, outPos *ConfigPos, extraYAMLFields ...string) error {
	fields := reflect.VisibleFields(reflect.TypeOf(outPtr).Elem())

	// Calculate the set of allowed/known field names in the YAML.
	var yamlFieldNames []string
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
		return err
	}
	// Copy the field values from the dynamically-created-type-without-methods
	// to the actual output struct.
	reflect.ValueOf(outPtr).Elem().Set(shadow.Elem())

	*outPos = *yamlPos(n)
	return nil
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (i *Input) UnmarshalYAML(n *yaml.Node) error {
	if err := unmarshalPlain(n, i, &i.Pos); err != nil {
		return err
	}
	return nil
}

// Validate implements Validator.
func (i *Input) Validate() error {
	var reservedNameErr error
	if strings.HasPrefix(i.Name.Val, "_") {
		reservedNameErr = i.Name.Pos.AnnotateErr(fmt.Errorf("input names beginning with _ are reserved"))
	}

	return errors.Join(
		notZeroModel(&i.Pos, i.Name, "name"),
		notZeroModel(&i.Pos, i.Desc, "desc"),
		reservedNameErr,
	)
}

type InputRule struct {
	Rule    String
	Message String
}

// Step represents one of the work steps involved in rendering a template.
type Step struct {
	// Pos is the YAML file location where this object started.
	Pos ConfigPos `yaml:"-"`

	Desc   String `yaml:"desc"`
	Action String `yaml:"action"`

	// Each action type has a field below. Only one of these will be set.
	Append          *Append          `yaml:"-"`
	ForEach         *ForEach         `yaml:"-"`
	GoTemplate      *GoTemplate      `yaml:"-"`
	Include         *Include         `yaml:"-"`
	Print           *Print           `yaml:"-"`
	RegexNameLookup *RegexNameLookup `yaml:"-"`
	RegexReplace    *RegexReplace    `yaml:"-"`
	StringReplace   *StringReplace   `yaml:"-"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *Step) UnmarshalYAML(n *yaml.Node) error {
	if err := unmarshalPlain(n, s, &s.Pos, "params"); err != nil {
		return nil
	}

	// The rest of this function just unmarshals the "params" field into the correct struct type depending
	// on the value of "action".
	var unmarshalInto any
	switch s.Action.Val {
	case "append":
		s.Append = new(Append)
		unmarshalInto = s.Append
		s.Append.Pos = s.Pos
	case "for_each":
		s.ForEach = new(ForEach)
		unmarshalInto = s.ForEach
		s.ForEach.Pos = s.Pos
	case "go_template":
		s.GoTemplate = new(GoTemplate)
		unmarshalInto = s.GoTemplate
		s.GoTemplate.Pos = s.Pos
	case "include":
		s.Include = new(Include)
		unmarshalInto = s.Include
		s.Include.Pos = s.Pos
	case "print":
		s.Print = new(Print)
		unmarshalInto = s.Print
		s.Print.Pos = s.Pos // Set an approximate position in case yaml unmarshaling fails later
	case "regex_name_lookup":
		s.RegexNameLookup = new(RegexNameLookup)
		unmarshalInto = s.RegexNameLookup
		s.RegexNameLookup.Pos = s.Pos
	case "regex_replace":
		s.RegexReplace = new(RegexReplace)
		unmarshalInto = s.RegexReplace
		s.RegexReplace.Pos = s.Pos
	case "string_replace":
		s.StringReplace = new(StringReplace)
		unmarshalInto = s.StringReplace
		s.StringReplace.Pos = s.Pos
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
		notZeroModel(&s.Pos, s.Desc, "desc"),
		validateUnlessNil(s.Append),
		validateUnlessNil(s.ForEach),
		validateUnlessNil(s.GoTemplate),
		validateUnlessNil(s.Include),
		validateUnlessNil(s.Print),
		validateUnlessNil(s.RegexNameLookup),
		validateUnlessNil(s.RegexReplace),
		validateUnlessNil(s.StringReplace),
	)
}

// Print is an action that prints a message to standard output.
type Print struct {
	// Pos is the YAML file location where this object started.
	Pos ConfigPos `yaml:"-"`

	Message String `yaml:"message"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (p *Print) UnmarshalYAML(n *yaml.Node) error {
	if err := unmarshalPlain(n, p, &p.Pos); err != nil {
		return err
	}
	return nil
}

// Validate implements Validator.
func (p *Print) Validate() error {
	return errors.Join(
		notZeroModel(&p.Pos, p.Message, "message"),
	)
}

// Include is an action that places files into the output directory.
type Include struct {
	// Pos is the YAML file location where this object started.
	Pos ConfigPos `yaml:"-"`

	Paths       []String `yaml:"paths"`
	From        String   `yaml:"from"`
	As          []String `yaml:"as"`
	StripPrefix String   `yaml:"strip_prefix"`
	AddPrefix   String   `yaml:"add_prefix"`
	Skip        []String `yaml:"skip"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (i *Include) UnmarshalYAML(n *yaml.Node) error {
	if err := unmarshalPlain(n, i, &i.Pos); err != nil {
		return err
	}
	return nil
}

// Validate implements Validator.
func (i *Include) Validate() error {
	var exclusivityErr error
	if len(i.As) != 0 {
		if i.StripPrefix.Val != "" || i.AddPrefix.Val != "" {
			exclusivityErr = i.As[0].Pos.AnnotateErr(fmt.Errorf(`"as" may not be used with "strip_prefix" or "add_prefix"`))
		} else if len(i.Paths) != len(i.As) {
			exclusivityErr = i.As[0].Pos.AnnotateErr(fmt.Errorf(`when using "as", the size of "as" (%d) must be the same as the size of "paths" (%d)`,
				len(i.As), len(i.Paths)))
		}
	}

	var fromErr error
	validFrom := []string{"destination"}
	if i.From.Val != "" && !slices.Contains(validFrom, i.From.Val) {
		fromErr = i.From.Pos.AnnotateErr(fmt.Errorf(`"from" must be one of %v`, validFrom))
	}

	return errors.Join(
		nonEmptySlice(&i.Pos, i.Paths, "paths"),
		exclusivityErr,
		fromErr,
	)
}

// RegexReplace is an action that replaces a regex match (or a subgroup of it) with a
// template expression.
type RegexReplace struct {
	// Pos is the YAML file location where this object started.
	Pos ConfigPos `yaml:"-"`

	Paths        []String             `yaml:"paths"`
	Replacements []*RegexReplaceEntry `yaml:"replacements"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (r *RegexReplace) UnmarshalYAML(n *yaml.Node) error {
	if err := unmarshalPlain(n, r, &r.Pos); err != nil {
		return err
	}
	return nil
}

// Validate implements Validator.
func (r *RegexReplace) Validate() error {
	return errors.Join(
		nonEmptySlice(&r.Pos, r.Paths, "paths"),
		nonEmptySlice(&r.Pos, r.Replacements, "replacements"),
		validateEach(r.Replacements),
	)
}

// RegexReplaceEntry is one of potentially many regex replacements to be applied.
type RegexReplaceEntry struct {
	Pos               ConfigPos `yaml:"-"`
	Regex             String    `yaml:"regex"`
	SubgroupToReplace String    `yaml:"subgroup_to_replace"`
	With              String    `yaml:"with"`
}

// Validate implements Validator.
func (r *RegexReplaceEntry) Validate() error {
	// Some validation happens later during execution:
	//  - Compiling the regular expression
	//  - Compiling the "with" template
	//  - Validating that the subgroup number is actually a valid subgroup in the regex

	var subgroupErr error
	if r.SubgroupToReplace.Val != "" {
		subgroupErr = isValidRegexGroupName(r.SubgroupToReplace, "subgroup")
	}

	return errors.Join(
		notZeroModel(&r.Pos, r.Regex, "regex"),
		notZeroModel(&r.Pos, r.With, "with"),
		subgroupErr,
	)
}

func (r *RegexReplaceEntry) UnmarshalYAML(n *yaml.Node) error {
	if err := unmarshalPlain(n, r, &r.Pos); err != nil {
		return err
	}
	return nil
}

// RegexNameLookup is an action that replaces named regex capturing groups with
// the template variable of the same name.
type RegexNameLookup struct {
	// Pos is the YAML file location where this object started.
	Pos ConfigPos `yaml:"-"`

	Paths        []String                `yaml:"paths"`
	Replacements []*RegexNameLookupEntry `yaml:"replacements"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (r *RegexNameLookup) UnmarshalYAML(n *yaml.Node) error {
	if err := unmarshalPlain(n, r, &r.Pos); err != nil {
		return err
	}
	return nil
}

// Validate implements Validator.
func (r *RegexNameLookup) Validate() error {
	return errors.Join(
		nonEmptySlice(&r.Pos, r.Paths, "paths"),
		nonEmptySlice(&r.Pos, r.Replacements, "replacements"),
		validateEach(r.Replacements),
	)
}

// RegexNameLookupEntry is one of potentially many regex replacements to be applied.
type RegexNameLookupEntry struct {
	Pos   ConfigPos `yaml:"-"`
	Regex String    `yaml:"regex"`
}

// Validate implements Validator.
func (r *RegexNameLookupEntry) Validate() error {
	return errors.Join(
		notZeroModel(&r.Pos, r.Regex, "regex"),
	)
}

func (r *RegexNameLookupEntry) UnmarshalYAML(n *yaml.Node) error {
	if err := unmarshalPlain(n, r, &r.Pos); err != nil {
		return err
	}
	return nil
}

// StringReplace is an action that replaces a string with a template expression.
type StringReplace struct {
	// Pos is the YAML file location where this object started.
	Pos ConfigPos `yaml:"-"`

	Paths        []String             `yaml:"paths"`
	Replacements []*StringReplacement `yaml:"replacements"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *StringReplace) UnmarshalYAML(n *yaml.Node) error {
	if err := unmarshalPlain(n, s, &s.Pos); err != nil {
		return err
	}
	return nil
}

// Validate implements Validator.
func (s *StringReplace) Validate() error {
	// Some validation doesn't happen here, it happens later during execution:
	//  - Compiling the regular expression
	//  - Compiling the "with" template
	//  - Validating that the subgroup number is actually a valid subgroup in
	//    the regex
	return errors.Join(
		nonEmptySlice(&s.Pos, s.Paths, "paths"),
		nonEmptySlice(&s.Pos, s.Replacements, "replacements"),
		validateEach(s.Replacements),
	)
}

type StringReplacement struct {
	Pos ConfigPos `yaml:"-"`

	ToReplace String `yaml:"to_replace"`
	With      String `yaml:"with"`
}

func (s *StringReplacement) Validate() error {
	return errors.Join(
		notZeroModel(&s.Pos, s.ToReplace, "to_replace"),
		notZeroModel(&s.Pos, s.With, "with"),
	)
}

func (s *StringReplacement) UnmarshalYAML(n *yaml.Node) error {
	if err := unmarshalPlain(n, s, &s.Pos); err != nil {
		return err
	}
	return nil
}

// Append is an action that appends some output to the end of the file.
type Append struct {
	// Pos is the YAML file location where this object started.
	Pos ConfigPos `yaml:"-"`

	Paths             []String `yaml:"paths"`
	With              String   `yaml:"with"`
	SkipEnsureNewline Bool     `yaml:"skip_ensure_newline"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (a *Append) UnmarshalYAML(n *yaml.Node) error {
	if err := unmarshalPlain(n, a, &a.Pos); err != nil {
		return err
	}
	return nil
}

// Validate implements Validator.
func (s *Append) Validate() error {
	return errors.Join(
		nonEmptySlice(&s.Pos, s.Paths, "paths"),
		notZeroModel(&s.Pos, s.With, "with"),
	)
}

// GoTemplate is an action that executes one more files as a Go template,
// replacing each one with its template output.
type GoTemplate struct {
	// Pos is the YAML file location where this object started.
	Pos ConfigPos `yaml:"-"`

	Paths []String `yaml:"paths"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (g *GoTemplate) UnmarshalYAML(n *yaml.Node) error {
	if err := unmarshalPlain(n, g, &g.Pos); err != nil {
		return err
	}
	return nil
}

// Validate implements Validator.
func (g *GoTemplate) Validate() error {
	// Checking that the input paths are valid will happen later.
	return errors.Join(nonEmptySlice(&g.Pos, g.Paths, "paths"))
}

type ForEach struct {
	// Pos is the YAML file location where this object started.
	Pos ConfigPos `yaml:"-"`

	Iterator *ForEachIterator `yaml:"iterator"`
	Steps    []*Step          `yaml:"steps"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (f *ForEach) UnmarshalYAML(n *yaml.Node) error {
	if err := unmarshalPlain(n, f, &f.Pos); err != nil {
		return err
	}
	return nil
}

func (f *ForEach) Validate() error {
	return errors.Join(
		notZero(&f.Pos, f.Iterator, "iterator"),
		nonEmptySlice(&f.Pos, f.Steps, "steps"),
		validateUnlessNil(f.Iterator),
		validateEach(f.Steps),
	)
}

type ForEachIterator struct {
	// Pos is the YAML file location where this object started.
	Pos ConfigPos `yaml:"-"`

	// The name by which the range value is accessed.
	Key String `yaml:"key"`

	// Exactly one of the following fields must be set.

	// Values is a list to range over, e.g. ["dev", "prod"]
	Values []String `yaml:"values"`
	// ValuesFrom is a CEL expression returning a list of strings to range over.
	ValuesFrom *String `yaml:"values_from"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (f *ForEachIterator) UnmarshalYAML(n *yaml.Node) error {
	if err := unmarshalPlain(n, f, &f.Pos); err != nil {
		return err
	}
	return nil
}

func (f *ForEachIterator) Validate() error {
	var exclusivityErr error
	if (len(f.Values) > 0 && f.ValuesFrom != nil) || (len(f.Values) == 0 && f.ValuesFrom == nil) {
		exclusivityErr = errors.New(`exactly one of the fields "values" or "values_from" must be set`)
	}

	return errors.Join(
		notZeroModel(&f.Pos, f.Key, "key"),
		exclusivityErr,
	)
}
