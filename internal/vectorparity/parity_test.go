// SPDX-License-Identifier: Apache-2.0

package vectorparity

import (
	"math"
	"slices"
	"testing"

	"github.com/azrtydxb/momo/vector"
)

func TestExactCosineTopK(t *testing.T) {
	vecs := Vectors()
	if len(vecs) != Count || len(vecs[0]) != Dim {
		t.Fatalf("dataset is %d x %d, want %d x %d", len(vecs), len(vecs[0]), Count, Dim)
	}
	if !slices.Equal(vecs[7], Vectors()[7]) {
		t.Fatal("dataset is not deterministic")
	}
	ids := ExactCosineTopK(vecs, vecs[0], TopK)
	if len(ids) != TopK || ids[0] != 1 {
		t.Fatalf("top-k = %v, want %d ids starting with the query itself (id 1)", ids, TopK)
	}
	for i := 1; i < len(ids); i++ {
		if CosineDistance(vecs[ids[i]-1], vecs[0]) < CosineDistance(vecs[ids[i-1]-1], vecs[0]) {
			t.Fatalf("top-k %v is not nearest first", ids)
		}
	}
}

// TestExactTopKMetrics holds the L2 and L1 references to the same shape as the
// cosine one: the query itself first, nearest first after that, and the
// cosine wrapper equal to the general function.
func TestExactTopKMetrics(t *testing.T) {
	vecs := Vectors()
	q := vecs[3]
	for name, dist := range map[string]func(a, b vector.Vector) float64{"l2": L2Distance, "l1": L1Distance} {
		ids := ExactTopK(vecs, q, TopK, dist)
		if len(ids) != TopK || ids[0] != 4 {
			t.Fatalf("%s top-k = %v, want %d ids starting with the query itself (id 4)", name, ids, TopK)
		}
		for i := 1; i < len(ids); i++ {
			if dist(vecs[ids[i]-1], q) < dist(vecs[ids[i-1]-1], q) {
				t.Fatalf("%s top-k %v is not nearest first", name, ids)
			}
		}
	}
	if !slices.Equal(ExactTopK(vecs, q, TopK, CosineDistance), ExactCosineTopK(vecs, q, TopK)) {
		t.Fatal("ExactCosineTopK differs from ExactTopK with the cosine distance")
	}
	if got := L2Distance(vector.Vector{1, 2, 3}, vector.Vector{4, 5, 6}); math.Abs(got-math.Sqrt(27)) > 1e-12 {
		t.Fatalf("L2Distance = %v, want sqrt(27)", got)
	}
	if got := L1Distance(vector.Vector{1, 2, 3}, vector.Vector{4, 5, 6}); got != 9 {
		t.Fatalf("L1Distance = %v, want 9", got)
	}
}
