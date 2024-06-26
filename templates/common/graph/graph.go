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
	"slices"

	"golang.org/x/exp/maps"
)

// CyclicError is returned when the input graph has a cycle.
type CyclicError[T comparable] struct {
	Cycle []T
}

func (e *CyclicError[T]) Error() string {
	return fmt.Sprintf("cycle detected: %v", e.Cycle)
}

// Graph represents a directed graph.
type Graph[T cmp.Ordered] struct {
	edges map[T][]T
}

// NewGraph creates a new graph.
func NewGraph[T cmp.Ordered]() *Graph[T] {
	return &Graph[T]{
		edges: make(map[T][]T),
	}
}

// AddEdge adds a directed edge from source to destination. This should be
// interpreted as "$source depends on $destination," not "$source comes before
// "$destination." In the topologically sorted output, $destination will come
// before $source.
func (g *Graph[T]) AddEdge(source, destination T) {
	g.edges[source] = append(g.edges[source], destination)
}

// AddNode adds a node to the graph without adding any edges. [AddEdge]
// implicitly creates nodes, so this function is only needed in the case where
// you have a node with no edges to or from it.
//
// This function is always safe to call. You can call it before adding edges to
// the node or after adding edges. It is idempotent.
func (g *Graph[T]) AddNode(n T) {
	if _, ok := g.edges[n]; ok {
		return // we already know that this node exists
	}
	g.edges[n] = nil // node exists in map, but has no outgoing edges
}

func (g *Graph[T]) EdgesFrom(n T) []T {
	return g.edges[n]
}

// TopologicalSort performs a topological sort. For all edges a->b, the output
// will have b before a.
//
// For the same graph, the same result will be returned, regardless of the order
// of Add*() calls, and regardless of Go's random map iteration order.
//
// If there is a cycle in the graph, an error message will be returned that
// names the nodes involved in the cycle.
func (g *Graph[T]) TopologicalSort() ([]T, error) {
	visited := make(map[T]struct{})
	out := make([]T, 0, len(g.edges))
	cycleDetect := make(map[T]struct{})

	// Output order must be the same across multiple CLI invocations. If we
	// care about the inefficient asymptotic runtime of this approach, we could
	// switch to a heap-based algorithm and abandon this DFS algorithm.
	nodes := maps.Keys(g.edges)
	slices.Sort(nodes)

	for _, node := range nodes {
		if _, ok := visited[node]; !ok {
			if err := g.dfs(node, visited, &out, cycleDetect); err != nil {
				return nil, err
			}
		}
	}

	return out, nil
}

// dfs is the heart of the topological sort. See
// https://en.wikipedia.org/wiki/Topological_sorting#Depth-first_search.
func (g *Graph[T]) dfs(node T, visited map[T]struct{}, stack *[]T, cycleDetect map[T]struct{}) error {
	visited[node] = struct{}{}
	cycleDetect[node] = struct{}{}

	neighbors := g.edges[node]
	slices.Sort(neighbors) // output order must be the same across multiple CLI invocations
	for _, neighbor := range neighbors {
		if _, ok := visited[neighbor]; !ok {
			if err := g.dfs(neighbor, visited, stack, cycleDetect); err != nil {
				return err
			}
		} else if _, ok := cycleDetect[neighbor]; ok {
			// Cycle detected!
			cycle := []T{node, neighbor}
			for current := neighbor; current != node; current = g.edges[current][0] {
				cycle = append(cycle, g.edges[current][0])
			}
			return &CyclicError[T]{cycle}
		}
	}

	delete(cycleDetect, node)     // Remove node from recursion stack
	*stack = append(*stack, node) // Add node to the output list

	return nil
}
