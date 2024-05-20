package graph

import (
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
			want: []int{0, 1, 2, 3}, // {0, 2, 1, 3} woudl also be valid
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
		t.Run(tc.name, func(t *testing.T) {
			got, err := TopoSort(tc.d)
			if err != tc.wantErr {
				t.Fatalf("got error %v but wanted %v", err, tc.wantErr)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("TopoSort(%v) = %v; want %v", tc.d, got, tc.want)
			}
		})
	}
}

func TestRandomGraph(t *testing.T) {
	t.Parallel()
	const numTests = 10000
	for seed := int64(0); seed < numTests; seed++ {
		// Why don't we use subtests here? Because we don't want to spam the
		// console with tens of thousands of test results.

		r := rand.New(rand.NewSource(seed)) // math rand, not crypto rand, because we want speed not security

		d := makeRandomGraph(r)
		got, err := TopoSort(d)
		if err != nil {
			continue // there was a cycle. Perhaps in the future we could verify that this is true somehow
		}

		assertSortIsTopological(t, d, got)
	}
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

func makeRandomGraph(rand *rand.Rand) DAG {
	numNodes := weightedRandom(rand, []int{1, 1, 50, 100, 100, 10}) // arbitrary, can be changed. Prefer not to have too small or too large of a graph.
	d := make(DAG, numNodes)
	for i := range d {
		// Prefer to generate a smallish number of neighbors for each node.
		// Otherwise, our randomly generated graphs would be very likely to be
		// cyclic and we wouldn't often get a valid topological sort.
		numNeighborWeights := []int{10, 10, 2, 1, 1, 1, 1, 1}[:numNodes]
		numNeighbors := weightedRandom(rand, numNeighborWeights)
		d[i] = rand.Perm(numNodes)[:numNeighbors]
	}
	return d
}

// weightedRandom generates a non-uniform random integer in the range
// 0<=len(weights). The inputs are probability mass for each index. They do not
// need to be normalized (so passing [1,1] and [999,999] means the same thing).
func weightedRandom(rand *rand.Rand, weights []int) int {
	sum := 0
	for _, w := range weights {
		sum += w
	}
	r := rand.Int() % sum
	for i, w := range weights {
		r -= w
		if r < 0 {
			return i
		}
	}
	panic("unreachable")
}
