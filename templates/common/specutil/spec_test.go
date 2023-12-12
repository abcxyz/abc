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

package specutil

import (
	"testing"

	"github.com/abcxyz/abc/templates/model"
	spec "github.com/abcxyz/abc/templates/model/spec/v1beta1"
	"github.com/google/go-cmp/cmp"
)

func TestSpecDescriptionForDescribe(t *testing.T) {
	t.Parallel()
	spec := &spec.Spec{
		Desc: model.String{Val: "Test Description"},
		Inputs: []*spec.Input{
			{
				Name: model.String{Val: "name1"},
				Desc: model.String{Val: "desc1"},
			},
		},
	}
	want := [][]string{
		{OutputDescriptionKey, "Test Description"},
	}

	if diff := cmp.Diff(SpecDescriptionForDescribe(spec), want); diff != "" {
		t.Errorf("got unexpected spec description (-got +want): %v", diff)
	}
}

func TestAllSpecInputVarForDescribe(t *testing.T) {
	t.Parallel()
	spec := &spec.Spec{
		Desc: model.String{Val: "Test Description"},
		Inputs: []*spec.Input{
			{
				Name:    model.String{Val: "name1"},
				Desc:    model.String{Val: "desc1"},
				Default: &model.String{Val: "."},
				Rules: []*spec.InputRule{
					{
						Rule:    model.String{Val: "test rule 0"},
						Message: model.String{Val: "test rule 0 message"},
					},
					{
						Rule: model.String{Val: "test rule 1"},
					},
				},
			},
			{
				Name: model.String{Val: "name2"},
				Desc: model.String{Val: "desc2"},
			},
		},
	}

	want := [][]string{
		{"Input name", "name1"},
		{"Description", "desc1"},
		{"Default", "."},
		{"Rule 0", "test rule 0"},
		{"Rule 0 msg", "test rule 0 message"},
		{"Rule 1", "test rule 1"},
		{"Input name", "name2"},
		{"Description", "desc2"},
	}

	if diff := cmp.Diff(AllSpecInputVarForDescribe(spec), want); diff != "" {
		t.Errorf("got unexpected spec description (-got +want): %v", diff)
	}
}

func TestSingleSpecInputVarForDescribe(t *testing.T) {
	t.Parallel()
	spec := &spec.Spec{
		Desc: model.String{Val: "Test Description"},
		Inputs: []*spec.Input{
			{
				Name:    model.String{Val: "name1"},
				Desc:    model.String{Val: "desc1"},
				Default: &model.String{Val: "."},
				Rules: []*spec.InputRule{
					{
						Rule:    model.String{Val: "test rule 0"},
						Message: model.String{Val: "test rule 0 message"},
					},
					{
						Rule: model.String{Val: "test rule 1"},
					},
				},
			},
		},
	}
	want := [][]string{
		{"Input name", "name1"},
		{"Description", "desc1"},
		{"Default", "."},
		{"Rule 0", "test rule 0"},
		{"Rule 0 msg", "test rule 0 message"},
		{"Rule 1", "test rule 1"},
	}

	if diff := cmp.Diff(SingleSpecInputVarForDescribe(spec.Inputs[0]), want); diff != "" {
		t.Errorf("got unexpected spec description (-got +want): %v", diff)
	}
}
