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
	"cmp"
	"fmt"
	"math"
	"math/rand"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestTopoSort(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		g       *Graph[string]
		want    [][]string
		wantErr error
	}{
		{
			name: "empty",
			g:    NewGraph[string](),
			want: [][]string{},
		},
		{
			name: "one_node_no_edges",
			g: func() *Graph[string] {
				g := NewGraph[string]()
				g.AddNode("a")
				return g
			}(),
			want: [][]string{{"a"}},
		},
		{
			name: "two_nodes_no_edges",
			g: func() *Graph[string] {
				g := NewGraph[string]()
				g.AddNode("a")
				g.AddNode("b")
				return g
			}(),
			want: [][]string{
				{"a", "b"},
				{"b", "a"},
			},
		},
		{
			name: "one_edge",
			g: func() *Graph[string] {
				g := NewGraph[string]()
				g.AddEdge("a", "b")
				return g
			}(),
			want: [][]string{{"b", "a"}},
		},
		{
			name: "two_edges",
			g: func() *Graph[string] {
				g := NewGraph[string]()
				g.AddEdge("a", "b")
				g.AddEdge("b", "c")
				return g
			}(),
			want: [][]string{{"c", "b", "a"}},
		},
		{
			name: "two_edges_superfluous_addnode_first",
			g: func() *Graph[string] {
				g := NewGraph[string]()
				g.AddNode("a")
				g.AddEdge("a", "b")
				g.AddEdge("b", "c")
				return g
			}(),
			want: [][]string{{"c", "b", "a"}},
		},
		{
			name: "two_edges_superfluous_addnode_last",
			g: func() *Graph[string] {
				g := NewGraph[string]()
				g.AddEdge("a", "b")
				g.AddEdge("b", "c")
				g.AddNode("a")
				return g
			}(),
			want: [][]string{{"c", "b", "a"}},
		},
		{
			name: "diamond",
			g: func() *Graph[string] {
				g := NewGraph[string]()
				g.AddEdge("a", "b")
				g.AddEdge("a", "c")
				g.AddEdge("b", "d")
				g.AddEdge("c", "d")
				return g
			}(),
			want: [][]string{
				{"d", "b", "c", "a"},
				{"d", "c", "b", "a"},
			},
		},
		{
			name: "3_cycle",
			g: func() *Graph[string] {
				g := NewGraph[string]()
				g.AddEdge("a", "b")
				g.AddEdge("b", "c")
				g.AddEdge("c", "a")
				return g
			}(),
			wantErr: &CyclicError[string]{Cycle: []string{"a", "b", "c"}},
		},
		{
			name: "3_cycle_with_unconnected_node",
			g: func() *Graph[string] {
				g := NewGraph[string]()
				g.AddEdge("a", "b")
				g.AddEdge("b", "c")
				g.AddEdge("c", "a")
				g.AddNode("d")
				return g
			}(),
			wantErr: &CyclicError[string]{Cycle: []string{"a", "b", "c"}},
		},
		{
			name: "self_edge",
			g: func() *Graph[string] {
				g := NewGraph[string]()
				g.AddEdge("a", "a")
				return g
			}(),
			wantErr: &CyclicError[string]{Cycle: []string{"a", "a"}},
		},
		{
			name: "cycle_with_other_edges",
			g: func() *Graph[string] {
				g := NewGraph[string]()
				g.AddEdge("a", "b")
				g.AddEdge("b", "c")
				g.AddEdge("c", "a")
				g.AddEdge("d", "e")
				return g
			}(),
			wantErr: &CyclicError[string]{Cycle: []string{"a", "b", "c"}},
		},
		{
			name: "many_edges_in_linear_order",
			g: func() *Graph[string] {
				g := NewGraph[string]()
				for i := 0; i < 10; i++ {
					g.AddEdge(fmt.Sprintf("node%d", i), fmt.Sprintf("node%d", i+1))
				}
				return g
			}(),
			want: [][]string{{"node10", "node9", "node8", "node7", "node6", "node5", "node4", "node3", "node2", "node1", "node0"}},
		},
		{
			name: "duplicate_edges",
			g: func() *Graph[string] {
				g := NewGraph[string]()
				g.AddEdge("a", "b")
				g.AddEdge("a", "b")
				g.AddEdge("a", "b")
				g.AddEdge("b", "c")
				return g
			}(),
			want: [][]string{{"c", "b", "a"}},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := tc.g.TopologicalSort()

			gocmp.Equal(err, tc.wantErr,
				// Cycles can appear in a variety of forms ({a b c a} or
				// {c a b c}), so we canonicalize by just checking the *set* of
				// nodes involved in the cycle.
				gocmp.Transformer("canonicalize_cycle", func(cycle []string) map[string]struct{} {
					out := map[string]struct{}{}
					for _, n := range cycle {
						out[n] = struct{}{}
					}
					return out
				}),
			)

			anyMatched := false
			if len(tc.want) == 0 && len(got) == 0 {
				anyMatched = true
			}
			for _, want := range tc.want {
				if gocmp.Equal(got, want, cmpopts.EquateEmpty()) {
					anyMatched = true
					break
				}
			}
			if !anyMatched {
				t.Errorf("for graph %v, got output %v, but wanted one of %v", tc.g.edges, got, tc.want)
			}
		})
	}
}

func TestTopoSortRandomGraph(t *testing.T) {
	t.Parallel()
	const numTests = 1000
	for seed := int64(0); seed < numTests; seed++ {
		// Why don't we use t.Run() here? Because we don't want to spam the
		// console with thousands of test results.

		// We use math rand, not crypto rand, because we want speed, not
		// security. We're just making a random-ish graph for testing.
		r := rand.New(rand.NewSource(seed)) //nolint:gosec

		g := makeRandomDAG(r)
		got, err := g.TopologicalSort()
		if err != nil {
			t.Fatal(err)
		}

		assertSortIsTopological(t, g, got)
	}
}

// makeRandomDAG randomly generates and returns a directed acyclic graph.
func makeRandomDAG(rand *rand.Rand) *Graph[int] {
	const maxNodes = 20

	// We generate a DAG by just iterating over a list of nodes and randomly
	// adding edges that only go "forward" in the list. By only adding forward
	// edges we guarantee that there are no cycles.
	numNodes := intMin(geometricRand(rand, 0.2), maxNodes)
	g := NewGraph[int]()
	for fromNode := 0; fromNode < numNodes; fromNode++ {
		nodesRemaining := numNodes - fromNode - 1 // how many nodes are after this one
		numNeighbors := intMin(geometricRand(rand, 0.2), nodesRemaining)
		for i := 0; i < numNeighbors; i++ {
			// This could generate multiple edges with the same (source,dest)
			// pair, but our algorithm should tolerate that.
			skipAhead := intMin(geometricRand(rand, 0.3), nodesRemaining)
			g.AddEdge(fromNode, fromNode+skipAhead)
		}
	}
	return g
}

// Generates a geometrically-distributed random number. This is good for when
// you want a random integer that's usually small but not always.
func geometricRand(rand *rand.Rand, p float64) int {
	// Check if probability is valid
	if p < 0 || p > 1 {
		panic("Probability must be between 0 and 1")
	}

	// Generate uniform random number
	u := rand.Float64()

	// Calculate geometrically distributed random number
	return int(math.Ceil(math.Log(1-u) / math.Log(1-p)))
}

func intMin(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func assertSortIsTopological[T cmp.Ordered](t *testing.T, g *Graph[T], candidateSort []T) {
	t.Helper()

	seen := make(map[T]struct{}, len(g.edges))
	for _, node := range candidateSort {
		for _, neighbor := range g.edges[node] {
			if _, ok := seen[neighbor]; !ok {
				t.Errorf("TopoSort(%v) = %v which is not topologically sorted, node %v depends on node %v which hasn't been seen yet",
					g.edges, candidateSort, node, neighbor)
			}
		}
		seen[node] = struct{}{}
	}
}
