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

// This file deals with tracking the line/column information in YAML files
// to support helpful error messages.
import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ConfigPos stores the position of an config value so error messages post-validation can point
// to problems. The zero value means "position unknown or there is no position."
//
// This is theoretically agnostic to input format; we could decide to support alternative
// serialization formats in the future.
type ConfigPos struct {
	Line   int
	Column int
}

// yamlPos constructs a position struct based on a YAML parse cursor.
func yamlPos(n *yaml.Node) ConfigPos {
	return ConfigPos{
		Line:   n.Line,
		Column: n.Column,
	}
}

// AnnotateErr prepends the config file location of a parsed value to an error. If the input err is
// nil, then nil is returned.
func (c ConfigPos) AnnotateErr(err error) error {
	if err == nil {
		return nil
	}

	pos := "(position unknown)" // This can happen when field values are defaults, rather that coming from the config file
	if c != (ConfigPos{}) {
		pos = fmt.Sprintf("line %d column %d", c.Line, c.Column)
	}

	return fmt.Errorf("invalid config near %s: %w", pos, err)
}
