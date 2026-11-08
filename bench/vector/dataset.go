// SPDX-License-Identifier: Apache-2.0

// Package vector holds the recall-matched vector benchmarks: a seeded dataset,
// exact versus approximate recall, and the ef_search sweep that picks the
// cheapest setting reaching a target recall.
package vector

import (
	"math"
	"math/rand"
	"strconv"
	"strings"
)

// Dataset is a seeded set of unit vectors and query vectors.
type Dataset struct {
	Dim     int
	Vectors [][]float32
	Queries [][]float32
}

// Generate returns n vectors and queries query vectors of dimension dim, all
// normalized to unit length, from a deterministic source.
func Generate(n, queries, dim int, seed int64) Dataset {
	r := rand.New(rand.NewSource(seed)) // #nosec G404 -- deterministic benchmark data
	ds := Dataset{Dim: dim, Vectors: make([][]float32, n), Queries: make([][]float32, queries)}
	for i := range ds.Vectors {
		ds.Vectors[i] = unit(r, dim)
	}
	for i := range ds.Queries {
		ds.Queries[i] = unit(r, dim)
	}
	return ds
}

func unit(r *rand.Rand, dim int) []float32 {
	v := make([]float32, dim)
	var sum float64
	for i := range v {
		x := r.NormFloat64()
		v[i] = float32(x)
		sum += x * x
	}
	norm := float32(math.Sqrt(sum))
	if norm == 0 {
		norm = 1
	}
	for i := range v {
		v[i] /= norm
	}
	return v
}

// VectorText renders v as a pgvector text literal such as [0.1,0.2].
func VectorText(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
