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
	"time"

	"gopkg.in/yaml.v3"

	"github.com/abcxyz/abc/templates/model"
	"github.com/abcxyz/abc/templates/model/header"
)

// Manifest represents the contents of a manifest file. A manifest file is the
// set of all information that is needed to cleanly upgrade to a new template
// version in the future.
type Manifest struct {
	Pos model.ConfigPos `yaml:"-"`

	// The UTC time when the template was first rendered (it's not touched for
	// upgrades). Will be marshaled in RFC3339 format, like
	// "2006-01-02T15:04:05Z". This is only as accurate as the system clock
	// on the machine where the operation ran.
	CreationTime time.Time `yaml:"creation_time"`

	// The UTC time when the template was most recently upgraded, or if has
	// never been upgraded, the time of initial template rendering. Will be
	// marshaled in RFC3339 format, like "2006-01-02T15:04:05Z". This is only as
	// accurate as the system clock on the machine where the operation ran.
	ModificationTime time.Time `yaml:"modification_time"`

	// The canonical template location from which upgraded template versions can
	// be fetched in the future.
	TemplateLocation model.String `yaml:"template_location"`

	// How to interpret template_location, e.g. "remote_git" or "local_git".
	LocationType model.String `yaml:"location_type"`

	// The tag, branch, SHA, or other version information.
	TemplateVersion model.String `yaml:"template_version"`

	UpgradeTrack model.String `yaml:"upgrade_track"`

	// // TODO
	// //
	// // This addresses the use case where the
	// // user originally installs the template from the `main` branch and
	// // therefore we should upgrade from the same branch, rather than using the
	// // "latest" release, which might actually be a downgrade.
	// //
	// // At upgrade time when this field is read, it will only be used if the
	// // location_type is "remote_git".
	// RequestedVersion model.String `yaml:"requested_version"`

	// The dirhash (https://pkg.go.dev/golang.org/x/mod/sumdb/dirhash) of the
	// template source tree (not the output). This shows exactly what version of
	// the template was installed.
	TemplateDirhash model.String `yaml:"template_dirhash"`

	// The input values that were supplied by the user when rendering the template.
	Inputs []*Input `yaml:"inputs"`

	// The hash of each output file created by the template.
	OutputFiles []*OutputFile `yaml:"output_files"`
}

// This absurdity is a workaround for a bug github.com/go-yaml/yaml/issues/817
// in the YAML library. We want to inline a Manifest in a WithHeader when
// marshaling. But the bug prevents that, because anything that implements
// Unmarshaler cannot be inlined. As a workaround, we create a new type with the
// same fields but without the Unmarshal method.
type (
	ForMarshaling Manifest
	WithHeader    header.With[*ForMarshaling]
)

// UnmarshalYAML implements yaml.Unmarshaler.
func (m *Manifest) UnmarshalYAML(n *yaml.Node) error {
	return model.UnmarshalPlain(n, m, &m.Pos, "api_version", "apiVersion", "kind") //nolint:wrapcheck
}

// Validate() implements model.Validator.
func (m *Manifest) Validate() error {
	// Inputs and OutputHashes can legally be empty, since a template doesn't
	// necessarily have these.
	return errors.Join(
		model.NotZeroModel(&m.Pos, m.TemplateDirhash, "template_dirhash"),
		model.ValidateEach(m.Inputs),
		model.ValidateEach(m.OutputFiles),
	)
}

// Input is a YAML object representing an input value that was provided to the
// template when it was rendered.
type Input struct {
	Pos model.ConfigPos `yaml:"-"`

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

// OutputFile records a checksum of a single file as it was created during
// template rendering.
type OutputFile struct {
	Pos model.ConfigPos `yaml:"-"`

	// The path, relative to the destination directory, of this file.
	File model.String `yaml:"file"`

	// The dirhash-style hash (see https://pkg.go.dev/golang.org/x/mod/sumdb/dirhash)
	// of this file. The format looks like "h1:0a1b2c3d...".
	Hash model.String `yaml:"hash"`

	// In the (somewhat rare) case where this file is a modified version of one
	// of the user's preexisting files using the "include from destination"
	// feature, then we save a patch here that is the inverse of our change.
	// This allows our change to be un-done in the future.
	Patch *model.String `yaml:"patch,omitempty"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (f *OutputFile) UnmarshalYAML(n *yaml.Node) error {
	return model.UnmarshalPlain(n, f, &f.Pos) //nolint:wrapcheck
}

// Validate() implements model.Validator.
func (f *OutputFile) Validate() error {
	return errors.Join(
		model.NotZeroModel(&f.Pos, f.File, "file"),
		model.NotZeroModel(&f.Pos, f.Hash, "hash"),
	)
}
