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
package spec

import (
	"errors"
	"io"
	"strings"

	"github.com/abcxyz/abc/templates/model"
	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"
)

// Decode unmarshals the YAML Spec from r. This function exists so we can
// validate the Spec model before providing it to the caller; we don't want the
// caller to forget, and thereby introduce bugs.
//
// If the Spec parses successfully but then fails validation, the spec will be
// returned along with the validation error.
func Decode(r io.Reader) (*Spec, error) {
	out := &Spec{}
	if err := model.DecodeAndValidate(r, "spec", out); err != nil {
		return nil, err
	}
	return out, nil
}

// Spec represents a parsed spec.yaml file describing a template.
type Spec struct {
	// Pos is the YAML file location where this object started.
	Pos model.ConfigPos `yaml:"-"`

	APIVersion model.String // this field is unmarshalled specially, see Spec.UnmarshalYAML
	Kind       model.String `yaml:"kind"`

	Desc   model.String `yaml:"desc"`
	Inputs []*Input     `yaml:"inputs"`
	Steps  []*Step      `yaml:"steps"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *Spec) UnmarshalYAML(n *yaml.Node) error {
	// The api_version field was mistakenly named apiVersion in the past, so accept both.
	if err := model.UnmarshalPlain(n, s, &s.Pos, "api_version", "apiVersion"); err != nil {
		return err
	}

	avShim := struct {
		OldStyle model.String `yaml:"apiVersion"`
		NewStyle model.String `yaml:"api_version"`
	}{}
	if err := n.Decode(&avShim); err != nil {
		return err
	}

	if avShim.NewStyle.Val != "" && avShim.OldStyle.Val != "" {
		return avShim.OldStyle.Pos.Errorf("must not set both apiVersion and api_version, please use api_version only")
	}
	if avShim.NewStyle.Val != "" {
		s.APIVersion = avShim.NewStyle
		return nil
	}
	if avShim.OldStyle.Val != "" {
		s.APIVersion = avShim.OldStyle
	}
	return nil
}

// Validate implements Validator.
func (s *Spec) Validate() error {
	return errors.Join(
		model.IsKnownSchemaVersion(&s.Pos, s.APIVersion, "api_version"),
		model.OneOf(&s.Pos, s.Kind, []string{"Template"}, "kind"),
		model.NotZeroModel(&s.Pos, s.Desc, "desc"),
		model.NonEmptySlice(&s.Pos, s.Steps, "steps"),
		model.ValidateEach(s.Inputs),
		model.ValidateEach(s.Steps),
	)
}

// Input represents one of the parsed "input" fields from the spec.yaml file.
type Input struct {
	// Pos is the YAML file location where this object started.
	Pos model.ConfigPos `yaml:"-"`

	Name    model.String  `yaml:"name"`
	Desc    model.String  `yaml:"desc"`
	Default *model.String `yaml:"default,omitempty"`
	Rules   []*InputRule  `yaml:"rules"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (i *Input) UnmarshalYAML(n *yaml.Node) error {
	return model.UnmarshalPlain(n, i, &i.Pos)
}

// Validate implements Validator.
func (i *Input) Validate() error {
	var reservedNameErr error
	if strings.HasPrefix(i.Name.Val, "_") {
		reservedNameErr = i.Name.Pos.Errorf("input names beginning with _ are reserved")
	}

	return errors.Join(
		model.NotZeroModel(&i.Pos, i.Name, "name"),
		model.NotZeroModel(&i.Pos, i.Desc, "desc"),
		reservedNameErr,
		model.ValidateEach(i.Rules),
	)
}

// InputRule represents a validation rule attached to an input.
type InputRule struct {
	Pos model.ConfigPos `yaml:"-"`

	Rule    model.String `yaml:"rule"`
	Message model.String `yaml:"message"` // optional
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (i *InputRule) UnmarshalYAML(n *yaml.Node) error {
	return model.UnmarshalPlain(n, i, &i.Pos)
}

// Validate implements Validator.
func (i *InputRule) Validate() error {
	return model.NotZeroModel(&i.Pos, i.Rule, "rule")
}

// Step represents one of the work steps involved in rendering a template.
type Step struct {
	// Pos is the YAML file location where this object started.
	Pos model.ConfigPos `yaml:"-"`

	Desc   model.String `yaml:"desc"`
	Action model.String `yaml:"action"`

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
	if err := model.UnmarshalPlain(n, s, &s.Pos, "params"); err != nil {
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
		return s.Pos.Errorf(`missing "action" field in this step`)
	default:
		return s.Pos.Errorf("unknown action type %q", s.Action.Val)
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
		model.NotZeroModel(&s.Pos, s.Desc, "desc"),
		model.ValidateUnlessNil(s.Append),
		model.ValidateUnlessNil(s.ForEach),
		model.ValidateUnlessNil(s.GoTemplate),
		model.ValidateUnlessNil(s.Include),
		model.ValidateUnlessNil(s.Print),
		model.ValidateUnlessNil(s.RegexNameLookup),
		model.ValidateUnlessNil(s.RegexReplace),
		model.ValidateUnlessNil(s.StringReplace),
	)
}

// Print is an action that prints a message to standard output.
type Print struct {
	// Pos is the YAML file location where this object started.
	Pos model.ConfigPos `yaml:"-"`

	Message model.String `yaml:"message"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (p *Print) UnmarshalYAML(n *yaml.Node) error {
	return model.UnmarshalPlain(n, p, &p.Pos)
}

// Validate implements Validator.
func (p *Print) Validate() error {
	return errors.Join(
		model.NotZeroModel(&p.Pos, p.Message, "message"),
	)
}

// Include is an action that places files into the output directory.
type Include struct {
	// Pos is the YAML file location where this object started.
	Pos model.ConfigPos `yaml:"-"`

	Paths []*IncludePath `yaml:"paths"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (i *Include) UnmarshalYAML(n *yaml.Node) error {
	// There are two cases for an "include":
	//  1. "paths" is a list of strings (old-style)
	//  2. "paths" is a list of objects (new-style)
	//
	// We do this by unmarshaling into a map, then checking the "kind" of the
	// YAML objects in the map values. If "paths" is a list of scalars, then we
	// assume we're dealing with case 1. Otherwise we assume we're dealing with
	// case 2.
	//
	// The shape of the Include struct looks the same either way, so downstream
	// code inside this program doesn't have to know that there are two cases.

	nodesMap := map[string]yaml.Node{}
	if err := n.Decode(nodesMap); err != nil {
		return model.YAMLPos(n).Errorf("%w", err)
	}

	pathsNode, ok := nodesMap["paths"]
	if !ok {
		return model.YAMLPos(n).Errorf(`field "paths" is required`)
	}
	if pathsNode.Kind != yaml.SequenceNode {
		return model.YAMLPos(&pathsNode).Errorf("paths must be a YAML list")
	}
	var listElemKind, zeroKind yaml.Kind
	for _, elemNode := range pathsNode.Content {
		if listElemKind != zeroKind && elemNode.Kind != listElemKind {
			return model.YAMLPos(&pathsNode).Errorf("Lists of paths must be homogeneous, either all strings or all objects")
		}
		listElemKind = elemNode.Kind
	}

	if listElemKind == yaml.ScalarNode { // Detect old-style case 1 input
		ip := &IncludePath{}
		i.Paths = []*IncludePath{ip}
		// Subtle point: in case 1 ("old-style"), we unmarshal the incoming YAML object as an "IncludePath" struct.
		return model.UnmarshalPlain(n, ip, &ip.Pos)
	}

	// Otherwise we're in case 2, we just unmarshal the incoming YAML object as an "Include: struct.
	return model.UnmarshalPlain(n, i, &i.Pos)
}

// Validate implements Validator.
func (i *Include) Validate() error {
	return errors.Join(
		model.ValidateEach(i.Paths),
		model.NonEmptySlice(&i.Pos, i.Paths, "paths"),
	)
}

// IncludePath represents an object for controlling the behavior of included files.
type IncludePath struct {
	Pos model.ConfigPos `yaml:"-"`

	AddPrefix   model.String   `yaml:"add_prefix"`
	As          []model.String `yaml:"as"`
	From        model.String   `yaml:"from"`
	OnConflict  model.String   `yaml:"on_conflict"`
	Paths       []model.String `yaml:"paths"`
	Skip        []model.String `yaml:"skip"`
	StripPrefix model.String   `yaml:"strip_prefix"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (i *IncludePath) UnmarshalYAML(n *yaml.Node) error {
	return model.UnmarshalPlain(n, i, &i.Pos)
}

// Validate implements Validator.
func (i *IncludePath) Validate() error {
	var exclusivityErr error
	if len(i.As) != 0 {
		if i.StripPrefix.Val != "" || i.AddPrefix.Val != "" {
			exclusivityErr = i.As[0].Pos.Errorf(`"as" may not be used with "strip_prefix" or "add_prefix"`)
		} else if len(i.Paths) != len(i.As) {
			exclusivityErr = i.As[0].Pos.Errorf(`when using "as", the size of "as" (%d) must be the same as the size of "paths" (%d)`,
				len(i.As), len(i.Paths))
		}
	}

	var fromErr error
	validFrom := []string{"destination"}
	if i.From.Val != "" && !slices.Contains(validFrom, i.From.Val) {
		fromErr = i.From.Pos.Errorf(`"from" must be one of %v`, validFrom)
	}

	return errors.Join(
		model.NonEmptySlice(&i.Pos, i.Paths, "paths"),
		exclusivityErr,
		fromErr,
	)
}

// RegexReplace is an action that replaces a regex match (or a subgroup of it) with a
// template expression.
type RegexReplace struct {
	// Pos is the YAML file location where this object started.
	Pos model.ConfigPos `yaml:"-"`

	Paths        []model.String       `yaml:"paths"`
	Replacements []*RegexReplaceEntry `yaml:"replacements"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (r *RegexReplace) UnmarshalYAML(n *yaml.Node) error {
	return model.UnmarshalPlain(n, r, &r.Pos)
}

// Validate implements Validator.
func (r *RegexReplace) Validate() error {
	return errors.Join(
		model.NonEmptySlice(&r.Pos, r.Paths, "paths"),
		model.NonEmptySlice(&r.Pos, r.Replacements, "replacements"),
		model.ValidateEach(r.Replacements),
	)
}

// RegexReplaceEntry is one of potentially many regex replacements to be applied.
type RegexReplaceEntry struct {
	Pos               model.ConfigPos `yaml:"-"`
	Regex             model.String    `yaml:"regex"`
	SubgroupToReplace model.String    `yaml:"subgroup_to_replace"`
	With              model.String    `yaml:"with"`
}

// Validate implements Validator.
func (r *RegexReplaceEntry) Validate() error {
	// Some validation happens later during execution:
	//  - Compiling the regular expression
	//  - Compiling the "with" template
	//  - Validating that the subgroup number is actually a valid subgroup in the regex

	var subgroupErr error
	if r.SubgroupToReplace.Val != "" {
		subgroupErr = model.IsValidRegexGroupName(r.SubgroupToReplace, "subgroup")
	}

	return errors.Join(
		model.NotZeroModel(&r.Pos, r.Regex, "regex"),
		model.NotZeroModel(&r.Pos, r.With, "with"),
		subgroupErr,
	)
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (r *RegexReplaceEntry) UnmarshalYAML(n *yaml.Node) error {
	return model.UnmarshalPlain(n, r, &r.Pos)
}

// RegexNameLookup is an action that replaces named regex capturing groups with
// the template variable of the same name.
type RegexNameLookup struct {
	// Pos is the YAML file location where this object started.
	Pos model.ConfigPos `yaml:"-"`

	Paths        []model.String          `yaml:"paths"`
	Replacements []*RegexNameLookupEntry `yaml:"replacements"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (r *RegexNameLookup) UnmarshalYAML(n *yaml.Node) error {
	return model.UnmarshalPlain(n, r, &r.Pos)
}

// Validate implements Validator.
func (r *RegexNameLookup) Validate() error {
	return errors.Join(
		model.NonEmptySlice(&r.Pos, r.Paths, "paths"),
		model.NonEmptySlice(&r.Pos, r.Replacements, "replacements"),
		model.ValidateEach(r.Replacements),
	)
}

// RegexNameLookupEntry is one of potentially many regex replacements to be applied.
type RegexNameLookupEntry struct {
	Pos   model.ConfigPos `yaml:"-"`
	Regex model.String    `yaml:"regex"`
}

// Validate implements Validator.
func (r *RegexNameLookupEntry) Validate() error {
	return errors.Join(
		model.NotZeroModel(&r.Pos, r.Regex, "regex"),
	)
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (r *RegexNameLookupEntry) UnmarshalYAML(n *yaml.Node) error {
	return model.UnmarshalPlain(n, r, &r.Pos)
}

// StringReplace is an action that replaces a string with a template expression.
type StringReplace struct {
	// Pos is the YAML file location where this object started.
	Pos model.ConfigPos `yaml:"-"`

	Paths        []model.String       `yaml:"paths"`
	Replacements []*StringReplacement `yaml:"replacements"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *StringReplace) UnmarshalYAML(n *yaml.Node) error {
	return model.UnmarshalPlain(n, s, &s.Pos)
}

// Validate implements Validator.
func (s *StringReplace) Validate() error {
	// Some validation doesn't happen here, it happens later during execution:
	//  - Compiling the regular expression
	//  - Compiling the "with" template
	//  - Validating that the subgroup number is actually a valid subgroup in
	//    the regex
	return errors.Join(
		model.NonEmptySlice(&s.Pos, s.Paths, "paths"),
		model.NonEmptySlice(&s.Pos, s.Replacements, "replacements"),
		model.ValidateEach(s.Replacements),
	)
}

type StringReplacement struct {
	Pos model.ConfigPos `yaml:"-"`

	ToReplace model.String `yaml:"to_replace"`
	With      model.String `yaml:"with"`
}

func (s *StringReplacement) Validate() error {
	return errors.Join(
		model.NotZeroModel(&s.Pos, s.ToReplace, "to_replace"),
		model.NotZeroModel(&s.Pos, s.With, "with"),
	)
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *StringReplacement) UnmarshalYAML(n *yaml.Node) error {
	return model.UnmarshalPlain(n, s, &s.Pos)
}

// Append is an action that appends some output to the end of the file.
type Append struct {
	// Pos is the YAML file location where this object started.
	Pos model.ConfigPos `yaml:"-"`

	Paths             []model.String `yaml:"paths"`
	With              model.String   `yaml:"with"`
	SkipEnsureNewline model.Bool     `yaml:"skip_ensure_newline"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (a *Append) UnmarshalYAML(n *yaml.Node) error {
	return model.UnmarshalPlain(n, a, &a.Pos)
}

// Validate implements Validator.
func (a *Append) Validate() error {
	return errors.Join(
		model.NonEmptySlice(&a.Pos, a.Paths, "paths"),
		model.NotZeroModel(&a.Pos, a.With, "with"),
	)
}

// GoTemplate is an action that executes one more files as a Go template,
// replacing each one with its template output.
type GoTemplate struct {
	// Pos is the YAML file location where this object started.
	Pos model.ConfigPos `yaml:"-"`

	Paths []model.String `yaml:"paths"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (g *GoTemplate) UnmarshalYAML(n *yaml.Node) error {
	return model.UnmarshalPlain(n, g, &g.Pos)
}

// Validate implements Validator.
func (g *GoTemplate) Validate() error {
	// Checking that the input paths are valid will happen later.
	return errors.Join(model.NonEmptySlice(&g.Pos, g.Paths, "paths"))
}

type ForEach struct {
	// Pos is the YAML file location where this object started.
	Pos model.ConfigPos `yaml:"-"`

	Iterator *ForEachIterator `yaml:"iterator"`
	Steps    []*Step          `yaml:"steps"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (f *ForEach) UnmarshalYAML(n *yaml.Node) error {
	return model.UnmarshalPlain(n, f, &f.Pos)
}

func (f *ForEach) Validate() error {
	return errors.Join(
		model.NotZero(&f.Pos, f.Iterator, "iterator"),
		model.NonEmptySlice(&f.Pos, f.Steps, "steps"),
		model.ValidateUnlessNil(f.Iterator),
		model.ValidateEach(f.Steps),
	)
}

type ForEachIterator struct {
	// Pos is the YAML file location where this object started.
	Pos model.ConfigPos `yaml:"-"`

	// The name by which the range value is accessed.
	Key model.String `yaml:"key"`

	// Exactly one of the following fields must be set.

	// Values is a list to range over, e.g. ["dev", "prod"]
	Values []model.String `yaml:"values"`
	// ValuesFrom is a CEL expression returning a list of strings to range over.
	ValuesFrom *model.String `yaml:"values_from"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (f *ForEachIterator) UnmarshalYAML(n *yaml.Node) error {
	return model.UnmarshalPlain(n, f, &f.Pos)
}

func (f *ForEachIterator) Validate() error {
	var exclusivityErr error
	if (len(f.Values) > 0 && f.ValuesFrom != nil) || (len(f.Values) == 0 && f.ValuesFrom == nil) {
		exclusivityErr = errors.New(`exactly one of the fields "values" or "values_from" must be set`)
	}

	return errors.Join(
		model.NotZeroModel(&f.Pos, f.Key, "key"),
		exclusivityErr,
	)
}
