// SPDX-License-Identifier: Apache-2.0

// Package vectorparity holds the dataset and the exact reference that the
// vector parity tests share, so that PostgreSQL pgvector and MariaDB are both
// held to one answer computed in plain Go.
package vectorparity

import (
	"math"
	"math/rand"
	"sort"

	"github.com/azrtydxb/momo/vector"
)

const (
	// Dim is the vector dimension of the dataset.
	Dim = 8
	// Count is the number of vectors; the vector with index i has id i+1.
	Count = 300
	// TopK is the size of the nearest-neighbour result under test.
	TopK = 10
	// MinIndexedOverlap is how many of the TopK exact ids an approximate
	// (indexed) search must return.
	MinIndexedOverlap = 9
	seed              = 5
)

// Vectors returns the deterministic dataset. Element i has id i+1.
func Vectors() []vector.Vector {
	r := rand.New(rand.NewSource(seed)) // #nosec G404 -- reproducible test data, not a secret
	out := make([]vector.Vector, Count)
	for i := range out {
		out[i] = make(vector.Vector, Dim)
		for j := range out[i] {
			out[i][j] = float32(r.Float64()*2 - 1)
		}
	}
	return out
}

// CosineDistance is 1 - cos(a, b) in float64.
func CosineDistance(a, b vector.Vector) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	return 1 - dot/(math.Sqrt(na)*math.Sqrt(nb))
}

// L2Distance is the Euclidean distance in float64.
func L2Distance(a, b vector.Vector) float64 {
	var sum float64
	for i := range a {
		d := float64(a[i]) - float64(b[i])
		sum += d * d
	}
	return math.Sqrt(sum)
}

// L1Distance is the taxicab distance in float64.
func L1Distance(a, b vector.Vector) float64 {
	var sum float64
	for i := range a {
		sum += math.Abs(float64(a[i]) - float64(b[i]))
	}
	return sum
}

// ExactTopK returns the ids of the k vectors nearest to q by dist, nearest
// first, ties broken by the lower id.
func ExactTopK(vecs []vector.Vector, q vector.Vector, k int, dist func(a, b vector.Vector) float64) []int64 {
	type scored struct {
		id   int64
		dist float64
	}
	all := make([]scored, len(vecs))
	for i, v := range vecs {
		all[i] = scored{id: int64(i + 1), dist: dist(v, q)}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].dist != all[j].dist {
			return all[i].dist < all[j].dist
		}
		return all[i].id < all[j].id
	})
	ids := make([]int64, 0, k)
	for _, s := range all[:k] {
		ids = append(ids, s.id)
	}
	return ids
}

// ExactCosineTopK returns the ids of the k vectors nearest to q by cosine
// distance, nearest first, ties broken by the lower id.
func ExactCosineTopK(vecs []vector.Vector, q vector.Vector, k int) []int64 {
	return ExactTopK(vecs, q, k, CosineDistance)
}
