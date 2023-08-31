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

package common

import "golang.org/x/exp/maps"

// scope binds variable names to values. It has a stack-like structure that
// allows inner scopes to inherit values from outer scopes. Variable names are
// looked up in order of innermost-to-outermost.
//
// For example, a for_each action defines a key value that is assigned to each
// of a list of values. The new variable introduced in this way "shadows" any
// variable that may previously exist of the same name. When the for_each loop
// is finished, then the outer scope's variable becomes available again.
type Scope struct {
	vars    map[string]string // never nil
	inherit *Scope            // is nil if this is the outermost scope.
}

func NewScope(m map[string]string) *Scope {
	if m == nil {
		// This isn't strictly necessary, because a lookup in a nil map just
		// returns false, but it saves complexity in the other methods.
		m = map[string]string{}
	}
	return &Scope{
		vars: m,
	}
}

// Lookup returns the current value of a given variable name, or false.
func (s *Scope) Lookup(name string) (string, bool) {
	val, ok := s.vars[name]
	if ok {
		return val, true
	}

	if s.inherit == nil {
		// This is the outermost scope, there's no more variables anywhere else.
		return "", false
	}

	return s.inherit.Lookup(name)
}

// With returns a new scope containing a new set of variable values. It forwards
// lookups to the previously existing scope if the lookup key is not found in m.
func (s *Scope) With(m map[string]string) *Scope {
	return &Scope{
		vars:    maps.Clone(m), // defensively clone to avoid bugs if the caller modifies their copy
		inherit: s,
	}
}

// All returns all variable bindings that are in scope. Inner/top-of-stack
// bindings take priority over outer bindings of the same name.
//
// The returned map is a copy that is owned by the caller; it can be changed
// safely.
//
// The return value is never nil.
func (s *Scope) All() map[string]string {
	if s.inherit == nil {
		return maps.Clone(s.vars)
	}

	out := s.inherit.All()
	maps.Copy(out, s.vars)
	return out
}
