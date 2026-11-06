// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/vector"
)

type encodingCase struct {
	name  string
	ddl   string
	value func(r *rand.Rand) any
}

var encodingCases = []encodingCase{
	{"vector-1536", "vector(1536)", func(r *rand.Rand) any { return vector.Vector(randomFloats(r, 1536)) }},
	{"halfvec-3072", "halfvec(3072)", func(r *rand.Rand) any { return vector.HalfVector(randomFloats(r, 3072)) }},
	{"sparsevec-1000", "sparsevec(1000)", func(r *rand.Rand) any { return randomSparse(r, 1000, 100) }},
	{"bit-1024", "bit(1024)", func(r *rand.Rand) any { return randomBits(r, 1024) }},
}

func randomFloats(r *rand.Rand, n int) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = float32(r.Float64()*2 - 1)
	}
	return out
}

// randomSparse returns nnz distinct, sorted, 0-based indexes below dim.
func randomSparse(r *rand.Rand, dim, nnz int) vector.SparseVector {
	idx := r.Perm(dim)[:nnz]
	sort.Ints(idx)
	s := vector.SparseVector{Dim: dim, Indices: make([]int32, nnz), Values: make([]float32, nnz)}
	for i, j := range idx {
		s.Indices[i] = int32(j)
		s.Values[i] = float32(r.Float64()*2 - 1)
	}
	return s
}

func randomBits(r *rand.Rand, n int) vector.Bits {
	data := make([]byte, (n+7)/8)
	r.Read(data)
	return vector.Bits{Len: n, Data: data}
}

// openEncodingDriver returns a driver on a fresh pool with a table `enc` of the given column type.
func openEncodingDriver(b *testing.B, format VectorFormat, ddl string) momo.Driver {
	b.Helper()
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(testdb.PostgresURL(b))
	if err != nil {
		b.Fatal(err)
	}
	if format == VectorBinary {
		ConfigureVectors(cfg)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(pool.Close)
	for _, stmt := range []string{
		"CREATE EXTENSION IF NOT EXISTS vector",
		"DROP TABLE IF EXISTS enc",
		"CREATE TABLE enc (id bigint, v " + ddl + ")",
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			b.Fatal(err)
		}
	}
	return New(pool, WithVectorFormat(format))
}

func formatName(f VectorFormat) string {
	if f == VectorBinary {
		return "binary"
	}
	return "text"
}

func BenchmarkVectorEncoding(b *testing.B) {
	for _, format := range []VectorFormat{VectorText, VectorBinary} {
		for _, c := range encodingCases {
			b.Run(fmt.Sprintf("%s/%s", formatName(format), c.name), func(b *testing.B) {
				drv := openEncodingDriver(b, format, c.ddl)
				ctx := context.Background()
				r := rand.New(rand.NewSource(1))
				values := make([]any, 1000)
				for i := range values {
					values[i] = c.value(r)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					for n, v := range values {
						if _, err := drv.Exec(ctx, "INSERT INTO enc (id, v) VALUES ($1, $2)", []any{int64(n), v}); err != nil {
							b.Fatal(err)
						}
					}
				}
			})
		}
	}
}
