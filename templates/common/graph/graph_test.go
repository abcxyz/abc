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

package graph

import (
	"fmt"
	"math"
	"math/rand"
	"slices"
	"testing"
)

func TestTopoSort(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		d       DAG
		want    []int
		wantErr error
	}{
		{
			name: "empty",
			d:    DAG{},
			want: []int{},
		},
		{
			name: "single_node",
			d:    DAG{{}},
			want: []int{0},
		},
		{
			name:    "single_node_self_edge",
			d:       DAG{{0}},
			wantErr: ErrCyclic,
		},
		{
			name:    "multi_node_self_edge",
			d:       DAG{{0}, {1}},
			wantErr: ErrCyclic,
		},
		{
			name: "2_linear",
			d:    DAG{{1}, {}},
			want: []int{1, 0},
		},
		{
			name: "3_linear",
			d:    DAG{{1}, {2}, {}},
			want: []int{2, 1, 0},
		},
		{
			name: "3_linear_reverse",
			d:    DAG{{}, {0}, {1}},
			want: []int{0, 1, 2},
		},
		{
			name: "diamond",
			d:    DAG{{1, 2}, {3}, {3}, {}},
			want: []int{3, 1, 2, 0}, // {3,2,1,0} would also be valid
		},
		{
			name: "diamond_reverse",
			d:    DAG{{}, {0}, {0}, {1, 2}},
			want: []int{0, 1, 2, 3}, // {0, 2, 1, 3} would also be valid
		},
		{
			name: "diamond_with_tail",
			d:    DAG{{1, 2}, {3}, {3}, {4}, {}},
			want: []int{4, 3, 1, 2, 0}, // {4,3,2,1,0} would also be valid
		},
		{
			name:    "2_cycle",
			d:       DAG{{1}, {0}},
			wantErr: ErrCyclic,
		},
		{
			name:    "3_cycle",
			d:       DAG{{1}, {2}, {0}},
			wantErr: ErrCyclic,
		},
		{
			name:    "cycle_with_other_edges",
			d:       DAG{{1}, {}, {3}, {2}},
			wantErr: ErrCyclic,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := TopoSort(tc.d)
			if err != tc.wantErr { //nolint:errorlint // the error should always be returned unwrapped, so we don't use errors.Is().
				t.Fatalf("got error %v but wanted %v", err, tc.wantErr)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("TopoSort(%v) = %v; want %v", tc.d, got, tc.want)
			}
		})
	}
}

func TestTopoSortGeneric(t *testing.T) {
	// Maintainers be advised: Gemini wrote this test.
	t.Parallel()

	cases := []struct {
		name    string
		m       map[string][]string
		want    []string
		wantErr error
	}{
		{
			name: "empty",
			m:    map[string][]string{},
			want: []string{},
		},
		{
			name: "single_node",
			m:    map[string][]string{"alice": {}},
			want: []string{"alice"},
		},
		{
			name:    "single_node_self_edge",
			m:       map[string][]string{"alice": {"alice"}},
			wantErr: ErrCyclic,
		},
		{
			name:    "multi_node_self_edge",
			m:       map[string][]string{"alice": {"bob"}, "bob": {"bob"}},
			wantErr: ErrCyclic,
		},
		{
			name: "2_linear",
			m:    map[string][]string{"alice": {"bob"}, "bob": {}},
			want: []string{"bob", "alice"},
		},
		{
			name: "3_linear",
			m:    map[string][]string{"alice": {"bob"}, "bob": {"charlie"}, "charlie": {}},
			want: []string{"charlie", "bob", "alice"},
		},
		{
			name: "3_linear_reverse",
			m:    map[string][]string{"alice": {}, "bob": {"alice"}, "charlie": {"bob"}},
			want: []string{"alice", "bob", "charlie"},
		},
		{
			name: "diamond",
			m:    map[string][]string{"alice": {"bob", "charlie"}, "bob": {"david"}, "charlie": {"david"}, "david": {}},
			want: []string{"david", "bob", "charlie", "alice"}, // {"david", "charlie", "bob", "alice"} would also be valid
		},
		{
			name: "diamond_reverse",
			m:    map[string][]string{"alice": {}, "bob": {"alice"}, "charlie": {"alice"}, "david": {"bob", "charlie"}},
			want: []string{"alice", "bob", "charlie", "david"}, // {"alice", "charlie", "bob", "david"} would also be valid
		},
		{
			name: "diamond_with_tail",
			m:    map[string][]string{"alice": {"bob", "charlie"}, "bob": {"david"}, "charlie": {"david"}, "david": {"eve"}, "eve": {}},
			want: []string{"eve", "david", "bob", "charlie", "alice"}, // {"eve", "david", "charlie", "bob", "alice"} would also be valid
		},
		{
			name:    "2_cycle",
			m:       map[string][]string{"alice": {"bob"}, "bob": {"alice"}},
			wantErr: ErrCyclic,
		},
		{
			name:    "3_cycle",
			m:       map[string][]string{"alice": {"bob"}, "bob": {"charlie"}, "charlie": {"alice"}},
			wantErr: ErrCyclic,
		},
		{
			name:    "cycle_with_other_edges",
			m:       map[string][]string{"alice": {"bob"}, "bob": {}, "charlie": {"david"}, "david": {"charlie"}},
			wantErr: ErrCyclic,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := TopoSortGeneric(tc.m)
			if err != tc.wantErr { //nolint:errorlint // the error should always be returned unwrapped, so we don't use errors.Is().
				t.Fatalf("got error %v but wanted %v", err, tc.wantErr)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("TopoSortGeneric(%v) = %v; want %v", tc.m, got, tc.want)
			}
		})
	}
}

func TestTopoSort_InvalidEdge(t *testing.T) {
	t.Parallel()

	d := DAG{{1}, {2}, {3}}
	d[0] = append(d[0], 4) // out of bounds edge

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic, got none")
		}
	}()
	TopoSort(d)
}

func TestRandomDAGs(t *testing.T) {
	t.Parallel()
	const numTests = 1000
	for seed := int64(0); seed < numTests; seed++ {
		// Why don't we use t.Run() here? Because we don't want to spam the
		// console with tens of thousands of test results.

		// We use math rand, not crypto rand, because we want speed, not
		// security. We're just making a random-ish graph for testing.
		r := rand.New(rand.NewSource(seed)) //nolint:gosec

		d := makeRandomDAG(r)
		got, err := TopoSort(d)
		if err != nil {
			t.Fatal(err)
		}

		assertSortIsTopological(t, d, got)
		if seed == 0 {
			fmt.Printf("*** %v\n", d)
		}
	}
}

// makeRandomDAG randomly generates and returns a directed acyclic graph.
func makeRandomDAG(rand *rand.Rand) DAG {
	const (
		maxNodes           = 20
		maxOutEdgesPerNode = 5
	)
	// We generate a DAG by just iterating over a slice of nodes and randomly
	// adding edges that only go "forward" in the slice. By only adding forward
	// edges we guarantee that there are no cycles.
	numNodes := intMin(geometricRand(rand, 0.2), maxNodes)
	dag := make(DAG, numNodes)
	for fromNode := range dag {
		nodesRemaining := numNodes - fromNode - 1 // how many nodes are after this one
		numNeighbors := intMin(geometricRand(rand, 0.2), nodesRemaining)
		for i := 0; i < numNeighbors; i++ {
			// This could generate multiple edges with the same (source,dest)
			// pair, but our algorithm should tolerate that.
			skipAhead := intMin(geometricRand(rand, 0.3), nodesRemaining)
			dag[fromNode] = append(dag[fromNode], fromNode+skipAhead)
		}
	}

	return renumberNodes(dag)
}

// Reassigns random node indices, while keeping the graph structire, so we're
// not always handing an already-sorted input to the sort algorithm.
func renumberNodes(in DAG) DAG {
	numNodes := len(in)
	oldToNew := rand.Perm(numNodes) // index is old node index, oldToNew[i] is the new index for that node
	out := make(DAG, numNodes)
	for origSourceNode, destNodes := range in {
		for i, destNode := range destNodes {
			destNodes[i] = oldToNew[destNode]
		}
		out[oldToNew[origSourceNode]] = destNodes
	}
	return out
}

// Generates a geometrically-distributed random number. This is good for when
// you want a random integer that's usually small but not always.
func geometricRand(rand *rand.Rand, p float64) int {
	// Thanks to Gemini for writing this function!

	// Check if probability is valid
	if p < 0 || p > 1 {
		panic("Probability must be between 0 and 1")
	}

	// Generate uniform random number
	u := rand.Float64() //nolint:gosec

	// Calculate geometrically distributed random number
	return int(math.Ceil(math.Log(1-u) / math.Log(1-p)))
}

func intMin(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func assertSortIsTopological(t *testing.T, d DAG, candidateSort []int) {
	t.Helper()

	seen := make([]bool, len(d))
	for _, node := range candidateSort {
		for _, neighbor := range d[node] {
			if !seen[neighbor] {
				t.Errorf("TopoSort(%v) = %v which is not topologically sorted, node %d depends on %d which isn't among the preceding nodes",
					d, candidateSort, node, neighbor)
			}
		}
		seen[node] = true
	}
}
