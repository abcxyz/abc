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

// A DAG is a directed graph. It is list of nodes, identified by their slice
// index, each having 0 or more outgoing edges to other node indices. So
// [][]int{{1}, {}} is a graph with two nodes, where node 0 has an outgoing edge
// to node 1, and node 1 has no outgoing edges.
//
// The edge directions are interpreted as "node index N depends on node indices
// [X,Y,Z]". So the returned topological sort will place X, Y, and Z somewhere
// before N.
type DAG [][]int

// ErrCyclic is returned when the provided graph has a cycle (so it can't be
// topologically sorted).
var ErrCyclic = fmt.Errorf("this directed graph has a cycle")

type visitState int

const (
	unvisited visitState = iota
	visiting
	visited
)

// TopoSort is a topological (dependency-order) sort of the given node indices.
// If there are multiple valid topological orderings, the choice of which one
// you'll get back is deterministic but not controllable.
//
// The returned integers are indices into the input slice d. So a return value
// of []int{3, 7, ...} means that d[3] has no dependencies and comes first in
// the topological sort, and is followed by d[7], and so on.
//
// If any outbound edge index in any of the slices is out of range
// 0<=edgeIndex<len(d) then it will panic.
//
// Sorting an empty graph returns an empty list.
//
// Note for maintainers: why didn't we import a third-party lib for this?
// They're all either sketchy, unmaintainted, or require implementing lots of
// cumbersome interfaces. And this entire algorithm is tiny.
func TopoSort(d DAG) ([]int, error) {
	// This algorithm comes from
	// https://en.wikipedia.org/wiki/Topological_sorting#Depth-first_search.

	visitStates := make([]visitState, len(d))

	out := make([]int, 0, len(d))

	for node := 0; node < len(d); node++ {
		if err := visit(node, visitStates, d, &out); err != nil {
			return nil, err
		}
	}

	return out, nil
}

func visit(node int, visitStates []visitState, d DAG, out *[]int) error {
	switch visitStates[node] {
	case visited:
		return nil
	case visiting:
		return ErrCyclic
	case unvisited: // useless, but satisfies the linter.
	}
	visitStates[node] = visiting

	for _, neighbor := range d[node] {
		if err := visit(neighbor, visitStates, d, out); err != nil {
			return err
		}
	}

	visitStates[node] = visited

	*out = append(*out, node)
	return nil
}

// A wrapper around [TopoSort] for the case where you have graph node adjacency
// expressed as node labels rather than node indices. Like {"alice": ["bob"]}.
func TopoSortGeneric[T cmp.Ordered](m map[T][]T) ([]T, error) {
	d, inputOrder := encodeAsInts(m)

	topoSorted, err := TopoSort(d)
	if err != nil {
		return nil, err
	}

	return decodeFromInts(topoSorted, inputOrder), nil
}

// Converts a map with node labels of type T to an [][]int. The returned []T
// maps node indices to their original labels.
func encodeAsInts[T cmp.Ordered](m map[T][]T) (DAG, []T) {
	nodesSeen := map[T]struct{}{}
	for k, vs := range m {
		nodesSeen[k] = struct{}{}
		for _, v := range vs {
			nodesSeen[v] = struct{}{}
		}
	}
	sortedNodes := maps.Keys(nodesSeen)
	slices.SortFunc(sortedNodes, cmp.Compare)

	nodeToIndex := map[T]int{}
	for i, node := range sortedNodes {
		nodeToIndex[node] = i
	}

	d := make(DAG, len(sortedNodes))
	for index, node := range sortedNodes {
		for _, dependsNode := range m[node] {
			d[index] = append(d[index], nodeToIndex[dependsNode])
		}
	}

	return d, sortedNodes
}

func decodeFromInts[T cmp.Ordered](topoSorted []int, inputOrder []T) []T {
	out := make([]T, len(topoSorted))
	for i, node := range topoSorted {
		out[i] = inputOrder[node]
	}
	return out
}
