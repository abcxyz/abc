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
		oneOf(s.Pos, s.APIVersion, []string{"cli.abcxyz.dev/v1alpha1"}, "apiVersion"),
		oneOf(s.Pos, s.Kind, []string{"Template"}, "kind"),
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

	Name    String  `yaml:"name"`
	Desc    String  `yaml:"desc"`
	Default *String `yaml:"default,omitempty"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (i *Input) UnmarshalYAML(n *yaml.Node) error {
	knownYAMLFields := []string{"name", "desc", "default"}
	if err := extraFields(n, knownYAMLFields); err != nil {
		return err
	}

	// Unmarshal with default values
	type shadowType Input
	shadow := &shadowType{} // unmarshal into a type that doesn't have UnmarshalYAML

	if err := n.Decode(shadow); err != nil {
		return err
	}

	*i = Input(*shadow)
	i.Pos = yamlPos(n)

	return nil
}

// Validate implements Validator.
func (i *Input) Validate() error {
	var reservedNameErr error
	// Reasons for reserved input names:
	//  - "flags" is used as expose the CLI flags to the print action. If it was also
	//    an input name, there would be a collision when trying to do {{.flags.foo}}
	if slices.Contains([]string{"flags"}, i.Name.Val) {
		reservedNameErr = i.Name.Pos.AnnotateErr(fmt.Errorf("input name %q is reserved, please pick a different name", i.Name.Val))
	}

	return errors.Join(
		notZero(i.Pos, i.Name, "name"),
		notZero(i.Pos, i.Desc, "desc"),
		reservedNameErr,
	)
}

// Step represents one of the work steps involved in rendering a template.
type Step struct {
	// Pos is the YAML file location where this object started.
	Pos *ConfigPos `yaml:"-"`

	Desc   String `yaml:"desc"`
	Action String `yaml:"action"`

	// Each action type has a field below. Only one of these will be set.
	Append          *Append          `yaml:"-"`
	GoTemplate      *GoTemplate      `yaml:"-"`
	Include         *Include         `yaml:"-"`
	Print           *Print           `yaml:"-"`
	RegexNameLookup *RegexNameLookup `yaml:"-"`
	RegexReplace    *RegexReplace    `yaml:"-"`
	StringReplace   *StringReplace   `yaml:"-"`
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
	case "regex_replace":
		s.RegexReplace = new(RegexReplace)
		unmarshalInto = s.RegexReplace
		s.RegexReplace.Pos = s.Pos
	case "regex_name_lookup":
		s.RegexNameLookup = new(RegexNameLookup)
		unmarshalInto = s.RegexNameLookup
		s.RegexNameLookup.Pos = s.Pos
	case "string_replace":
		s.StringReplace = new(StringReplace)
		unmarshalInto = s.StringReplace
		s.StringReplace.Pos = s.Pos
	case "append":
		s.Append = new(Append)
		unmarshalInto = s.Append
		s.Append.Pos = s.Pos
	case "go_template":
		s.GoTemplate = new(GoTemplate)
		unmarshalInto = s.GoTemplate
		s.GoTemplate.Pos = s.Pos
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
		validateUnlessNil(s.Append),
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

	Paths       []String `yaml:"paths"`
	From        String   `yaml:"from"`
	As          []String `yaml:"as"`
	StripPrefix String   `yaml:"strip_prefix"`
	AddPrefix   String   `yaml:"add_prefix"`
	Skip        []String `yaml:"skip"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (i *Include) UnmarshalYAML(n *yaml.Node) error {
	knownYAMLFields := []string{"add_prefix", "as", "from", "paths", "skip", "strip_prefix"}
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
		nonEmptySlice(i.Pos, i.Paths, "paths"),
		exclusivityErr,
		fromErr,
	)
}

// RegexReplace is an action that replaces a regex match (or a subgroup of it) with a
// template expression.
type RegexReplace struct {
	// Pos is the YAML file location where this object started.
	Pos *ConfigPos `yaml:"-"`

	Paths        []String             `yaml:"paths"`
	Replacements []*RegexReplaceEntry `yaml:"replacements"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (r *RegexReplace) UnmarshalYAML(n *yaml.Node) error {
	knownYAMLFields := []string{"paths", "replacements"}
	if err := extraFields(n, knownYAMLFields); err != nil {
		return err
	}
	type shadowType RegexReplace
	shadow := &shadowType{} // see "Q2" in file comment above

	if err := n.Decode(shadow); err != nil {
		return err
	}
	*r = RegexReplace(*shadow)
	r.Pos = yamlPos(n)

	return nil
}

// Validate implements Validator.
func (r *RegexReplace) Validate() error {
	return errors.Join(
		nonEmptySlice(r.Pos, r.Paths, "paths"),
		nonEmptySlice(r.Pos, r.Replacements, "replacements"),
		validateEach(r.Replacements),
	)
}

// RegexReplaceEntry is one of potentially many regex replacements to be applied.
type RegexReplaceEntry struct {
	Pos               *ConfigPos `yaml:"-"`
	Regex             String     `yaml:"regex"`
	SubgroupToReplace String     `yaml:"subgroup_to_replace"`
	With              String     `yaml:"with"`
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
		notZero(r.Pos, r.Regex, "regex"),
		notZero(r.Pos, r.With, "with"),
		subgroupErr,
	)
}

func (r *RegexReplaceEntry) UnmarshalYAML(n *yaml.Node) error {
	knownYAMLFields := []string{"regex", "subgroup_to_replace", "with"}
	if err := extraFields(n, knownYAMLFields); err != nil {
		return err
	}
	type shadowType RegexReplaceEntry
	shadow := &shadowType{} // see "Q2" in file comment above

	if err := n.Decode(shadow); err != nil {
		return err
	}
	*r = RegexReplaceEntry(*shadow)
	r.Pos = yamlPos(n)

	return nil
}

// RegexNameLookup is an action that replaces named regex capturing groups with
// the template variable of the same name.
type RegexNameLookup struct {
	// Pos is the YAML file location where this object started.
	Pos *ConfigPos `yaml:"-"`

	Paths        []String                `yaml:"paths"`
	Replacements []*RegexNameLookupEntry `yaml:"replacements"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (r *RegexNameLookup) UnmarshalYAML(n *yaml.Node) error {
	knownYAMLFields := []string{"paths", "replacements"}
	if err := extraFields(n, knownYAMLFields); err != nil {
		return err
	}
	type shadowType RegexNameLookup
	shadow := &shadowType{} // see "Q2" in file comment above

	if err := n.Decode(shadow); err != nil {
		return err
	}
	*r = RegexNameLookup(*shadow)
	r.Pos = yamlPos(n)

	return nil
}

// Validate implements Validator.
func (r *RegexNameLookup) Validate() error {
	return errors.Join(
		nonEmptySlice(r.Pos, r.Paths, "paths"),
		nonEmptySlice(r.Pos, r.Replacements, "replacements"),
		validateEach(r.Replacements),
	)
}

// RegexNameLookupEntry is one of potentially many regex replacements to be applied.
type RegexNameLookupEntry struct {
	Pos   *ConfigPos `yaml:"-"`
	Regex String     `yaml:"regex"`
}

// Validate implements Validator.
func (r *RegexNameLookupEntry) Validate() error {
	return errors.Join(

		notZero(r.Pos, r.Regex, "regex"),
	)
}

func (r *RegexNameLookupEntry) UnmarshalYAML(n *yaml.Node) error {
	knownYAMLFields := []string{"regex"}
	if err := extraFields(n, knownYAMLFields); err != nil {
		return err
	}
	type shadowType RegexNameLookupEntry
	shadow := &shadowType{} // see "Q2" in file comment above

	if err := n.Decode(shadow); err != nil {
		return err
	}
	*r = RegexNameLookupEntry(*shadow)
	r.Pos = yamlPos(n)

	return nil
}

// StringReplace is an action that replaces a string with a template expression.
type StringReplace struct {
	// Pos is the YAML file location where this object started.
	Pos *ConfigPos `yaml:"-"`

	Paths        []String             `yaml:"paths"`
	Replacements []*StringReplacement `yaml:"replacements"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *StringReplace) UnmarshalYAML(n *yaml.Node) error {
	knownYAMLFields := []string{"paths", "replacements", "params"}
	if err := extraFields(n, knownYAMLFields); err != nil {
		return err
	}
	type shadowType StringReplace
	shadow := &shadowType{} // see "Q2" in file comment above

	if err := n.Decode(shadow); err != nil {
		return err
	}
	*s = StringReplace(*shadow)
	s.Pos = yamlPos(n)

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
		nonEmptySlice(s.Pos, s.Paths, "paths"),
		nonEmptySlice(s.Pos, s.Replacements, "replacements"),
		validateEach(s.Replacements),
	)
}

type StringReplacement struct {
	Pos *ConfigPos `yaml:"-"`

	ToReplace String `yaml:"to_replace"`
	With      String `yaml:"with"`
}

func (s *StringReplacement) Validate() error {
	return errors.Join(
		notZero(s.Pos, s.ToReplace, "to_replace"),
		notZero(s.Pos, s.With, "with"),
	)
}

func (s *StringReplacement) UnmarshalYAML(n *yaml.Node) error {
	knownYAMLFields := []string{"to_replace", "with"}
	if err := extraFields(n, knownYAMLFields); err != nil {
		return err
	}
	type shadowType StringReplacement
	shadow := &shadowType{} // see "Q2" in file comment above

	if err := n.Decode(shadow); err != nil {
		return err
	}
	*s = StringReplacement(*shadow)
	s.Pos = yamlPos(n)

	return nil
}

// Append is an action that appends some output to the end of the file.
type Append struct {
	// Pos is the YAML file location where this object started.
	Pos *ConfigPos `yaml:"-"`

	Paths             []String `yaml:"paths"`
	With              String   `yaml:"with"`
	SkipEnsureNewline Bool     `yaml:"skip_ensure_newline"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *Append) UnmarshalYAML(n *yaml.Node) error {
	knownYAMLFields := []string{"paths", "with", "skip_ensure_newline"}
	if err := extraFields(n, knownYAMLFields); err != nil {
		return err
	}
	type shadowType Append
	shadow := &shadowType{} // see "Q2" in file comment above

	if err := n.Decode(shadow); err != nil {
		return err
	}
	*s = Append(*shadow)
	s.Pos = yamlPos(n)

	return nil
}

// Validate implements Validator.
func (s *Append) Validate() error {
	return errors.Join(
		nonEmptySlice(s.Pos, s.Paths, "paths"),
		notZero(s.Pos, s.With, "with"),
	)
}

// GoTemplate is an action that executes one more files as a Go template,
// replacing each one with its template output.
type GoTemplate struct {
	// Pos is the YAML file location where this object started.
	Pos *ConfigPos `yaml:"-"`

	Paths []String `yaml:"paths"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (g *GoTemplate) UnmarshalYAML(n *yaml.Node) error {
	knownYAMLFields := []string{"paths"}
	if err := extraFields(n, knownYAMLFields); err != nil {
		return err
	}
	type shadowType GoTemplate
	shadow := &shadowType{} // see "Q2" in file comment above

	if err := n.Decode(shadow); err != nil {
		return err
	}
	*g = GoTemplate(*shadow)
	g.Pos = yamlPos(n)

	return nil
}

// Validate implements Validator.
func (g *GoTemplate) Validate() error {
	// Checking that the input paths are valid will happen later.
	return errors.Join(nonEmptySlice(g.Pos, g.Paths, "paths"))
}
