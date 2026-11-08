// SPDX-License-Identifier: Apache-2.0

package vectorparity

import (
	"slices"
	"testing"
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
