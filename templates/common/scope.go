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
	vars        map[string]string // never nil
	goTmplFuncs map[string]any    // never nil
	inherit     *Scope            // is nil if this is the outermost scope.
}

func NewScope(vars map[string]string, goTmplFuncs map[string]any) *Scope {
	return &Scope{
		vars:        cloneOrEmpty(vars),
		goTmplFuncs: cloneOrEmpty(goTmplFuncs),
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

// AllVars returns all variable bindings that are in scope. Inner/top-of-stack
// bindings take priority over outer bindings of the same name.
//
// The returned map is a copy that is owned by the caller; it can be changed
// safely.
//
// The return value is never nil.
func (s *Scope) AllVars() map[string]string {
	if s.inherit == nil {
		return maps.Clone(s.vars)
	}

	out := s.inherit.AllVars()
	maps.Copy(out, s.vars)
	return out
}

// GoTmplFuncs returns all the Go-template functions that are in-scope. The
// result is suitable for passing to text/template.Template.Funcs().
func (s *Scope) GoTmplFuncs() map[string]any {
	return maps.Clone(s.goTmplFuncs)
}

// cloneOrEmpty does two things:
//   - it makes a copy of the input map, so we can "own" the copy without
//     worrying about it being modified.
//   - if the input is nil, then an empty map is returned, guaranteeing that we
//     don't have to deal with a nil map in a Scope.
func cloneOrEmpty[T any](m map[string]T) map[string]T {
	if len(m) == 0 {
		return map[string]T{}
	}
	return maps.Clone(m)
}
