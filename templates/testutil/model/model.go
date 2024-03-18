// Copyright 2024 The Authors (see AUTHORS file)
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

import "github.com/abcxyz/abc/templates/model"

// S is a helper to easily create a model.String with less boilerplate.
func S(s string) model.String {
	return model.String{Val: s}
}

// SP is a helper to easily create a *model.String with less boilerplate.
func SP(s string) *model.String {
	out := S(s)
	return &out
}

// Strings wraps each element of the input in a model.String.
func Strings(ss ...string) []model.String {
	out := make([]model.String, len(ss))
	for i, s := range ss {
		out[i] = model.String{
			Pos: &model.ConfigPos{}, // for the purposes of testing, "location unknown" is fine.
			Val: s,
		}
	}
	return out
}
