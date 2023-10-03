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

package manifest

import (
	"errors"

	"github.com/abcxyz/abc/templates/model"
	"gopkg.in/yaml.v3"
)

// Manifest represents the contents of a manifest file. A manifest file is the
// set of all information that is needed to cleanly upgrade to a new template
// version in the future.
type Manifest struct {
	Pos model.ConfigPos `yaml:"-"`

	// The template source address as passed to `abc templates render`.
	TemplateLocation model.String `yaml:"template_location"`

	// The dirhash (https://pkg.go.dev/golang.org/x/mod/sumdb/dirhash) of the
	// template source tree (not the output). This shows exactly what version of
	// the template was installed.
	TemplateDirhash model.String `yaml:"template_dirhash"`

	// The input values that were supplied by the user when rendering the template.
	Inputs []*Input `yaml:"inputs"`

	// The hash of each output file created by the template.
	OutputHashes []*OutputHash `yaml:"output_hashes"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (m *Manifest) UnmarshalYAML(n *yaml.Node) error {
	return model.UnmarshalPlain(n, m, &m.Pos, "api_version", "apiVersion", "kind") //nolint:wrapcheck
}

// Validate() implements model.Validator.
func (m *Manifest) Validate() error {
	// Inputs and OutputHashes can legally be empty, since a template doesn't
	// necessarily have these.
	return errors.Join(
		model.NotZeroModel(&m.Pos, m.TemplateLocation, "template_location"),
		model.NotZeroModel(&m.Pos, m.TemplateDirhash, "template_dirhash"),
		model.ValidateEach(m.Inputs),
		model.ValidateEach(m.OutputHashes),
	)
}

// Input is a YAML object representing an input value that was provided to the
// template when it was rendered.
type Input struct {
	Pos model.ConfigPos

	// The name of the template input, e.g. "my_service_account"
	Name model.String `yaml:"name"`
	// The value of the template input, e.g. "foo@iam.gserviceaccount.com".
	Value model.String `yaml:"value"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (i *Input) UnmarshalYAML(n *yaml.Node) error {
	return model.UnmarshalPlain(n, i, &i.Pos) //nolint:wrapcheck
}

// Validate() implements model.Validator.
func (i *Input) Validate() error {
	return errors.Join(
		model.NotZeroModel(&i.Pos, i.Name, "name"),
		model.NotZeroModel(&i.Pos, i.Value, "value"),
	)
}

// OutputHash records a checksum of a single file as it was created during
// template rendering.
type OutputHash struct {
	Pos model.ConfigPos

	// The path, relative to the destination directory, of this file.
	File model.String `yaml:"file"`
	// The dirhash-style hash (see https://pkg.go.dev/golang.org/x/mod/sumdb/dirhash)
	// of this file. The format looks like "h1:0a1b2c3d...".
	Hash model.String `yaml:"hash"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (f *OutputHash) UnmarshalYAML(n *yaml.Node) error {
	return model.UnmarshalPlain(n, f, &f.Pos) //nolint:wrapcheck
}

// Validate() implements model.Validator.
func (f *OutputHash) Validate() error {
	return errors.Join(
		model.NotZeroModel(&f.Pos, f.File, "file"),
		model.NotZeroModel(&f.Pos, f.Hash, "hash"),
	)
}
