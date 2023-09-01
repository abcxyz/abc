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
// serialization formats besides YAML in the future.
type ConfigPos struct {
	Line   int
	Column int
}

// yamlPos constructs a position struct based on a YAML parse cursor.
func yamlPos(n *yaml.Node) *ConfigPos {
	return &ConfigPos{
		Line:   n.Line,
		Column: n.Column,
	}
}

// Errorf returns a error prepended with spec.yaml position information, if
// available.
//
// Examples:
//
//	Wrapping an error: c.Errorf("foo(): %w", err)
//
//	Creating an error: c.Errorf("something went wrong doing action %s", action)
func (c *ConfigPos) Errorf(fmtStr string, args ...any) error {
	err := fmt.Errorf(fmtStr, args...)
	if c == nil || *c == (ConfigPos{}) {
		return err
	}

	return fmt.Errorf("at line %d column %d: %w", c.Line, c.Column, err)
}
