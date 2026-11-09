// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/vecop"
	"github.com/azrtydxb/momo/vector"
)

// TestCosineDistanceDimensionlessColumn searches a dimensionless vector
// column that holds rows of two sizes, filtered by a dims column, the layout
// of a store whose embedding size follows the configured model. The column
// descriptor has dimension zero, so each query vector is checked against its
// own size and pgvector compares only rows of that size.
func TestCosineDistanceDimensionlessColumn(t *testing.T) {
	db := testopen.Postgres(t)
	mustExecPG(t, db, "CREATE TABLE docs (id bigint PRIMARY KEY, dims bigint NOT NULL, emb vector)")
	rows := []struct {
		id  int64
		emb vector.Vector
	}{
		{1, vector.Vector{1, 0, 0}},
		{2, vector.Vector{0, 1, 0}},
		{3, vector.Vector{1, 1, 0}},
		{4, vector.Vector{1, 0, 0, 0, 0}},
		{5, vector.Vector{0, 0, 0, 0, 1}},
		{6, vector.Vector{0, 0, 1, 0, 1}},
	}
	for _, r := range rows {
		mustExecPG(t, db, "INSERT INTO docs (id, dims, emb) VALUES (?, ?, ?)", r.id, int64(len(r.emb)), r.emb)
	}
	col := momo.NewVectorColumn[vector.Vector]("docs", "emb", 0, 0)
	dims := momo.ColumnOf[int64]("docs", "dims")
	pool := pgPool(t) // opened on the test's own schema, not a subtest's
	cases := []struct {
		name string
		q    vector.Vector
		want []int64
	}{
		// cosine distance to {1,0,0}: id 1 is 0, id 3 about 0.29, id 2 is 1.
		{"three dimensions", vector.Vector{1, 0, 0}, []int64{1, 3, 2}},
		// cosine distance to {0,0,0,0,1}: id 5 is 0, id 6 about 0.29, id 4 is 1.
		{"five dimensions", vector.Vector{0, 0, 0, 0, 1}, []int64{5, 6, 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := momo.Select[vecDoc, *vecDoc](db).
				Where(dims.Eq(int64(len(tc.q)))).
				OrderBy(vecop.Cosine(col, tc.q).Asc()).
				Limit(10).SQL()
			if err != nil {
				t.Fatal(err)
			}
			if got := queryIDs(t, pool, c); !slices.Equal(got, tc.want) {
				t.Fatalf("order = %v, want %v", got, tc.want)
			}
		})
	}
}
