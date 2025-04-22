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

package main

import (
	"testing"

	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/testing/passert"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/testing/ptest"
)

func TestEmitResult(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input []string
		want  []*DataModel
	}{
		{
			name:  "multiple csv records",
			input: []string{"id1", "id2", "id3"},
			want: []*DataModel{
				{ID: "id1"},
				{ID: "id2"},
				{ID: "id3"},
			},
		},
		{
			name:  "empty csv records",
			input: []string{},
			want:  []*DataModel{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			beam.Init()
			p, s := beam.NewPipelineWithRoot()
			ctx := t.Context()
			csvPCol := beam.CreateList(s, tc.input)
			dataModels := emitResult(ctx, s, csvPCol)

			passert.Equals(s, dataModels, beam.CreateList(s, tc.want))
			ptest.RunAndValidate(t, p)
		})
	}
}
