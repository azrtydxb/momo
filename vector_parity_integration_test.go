// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/internal/vectorparity"
	"github.com/azrtydxb/momo/vector"
)

// TestPostgresVectorParityReference holds pgvector exact search to the same
// reference ids the MariaDB parity test (driver/mysqldriver) is held to, so
// the two servers are compared through one plain-Go answer. The table has no
// index, so the cosine ordering is exact.
func TestPostgresVectorParityReference(t *testing.T) {
	vecs := vectorparity.Vectors()
	query := vecs[0]
	want := vectorparity.ExactCosineTopK(vecs, query, vectorparity.TopK)

	db := testopen.Postgres(t)
	mustExecPG(t, db, fmt.Sprintf("CREATE TABLE docs (id bigint PRIMARY KEY, emb vector(%d) NOT NULL)", vectorparity.Dim))
	for i, v := range vecs {
		mustExecPG(t, db, "INSERT INTO docs (id, emb) VALUES (?, ?)", int64(i+1), v)
	}
	col := momo.NewVectorColumn[vector.Vector]("docs", "emb", vectorparity.Dim, 0)
	c, err := momo.Select[vecDoc, *vecDoc](db).
		OrderBy(momo.CosineDistance(col, query).Asc(), momo.NewColumn[int64]("docs", "id", 0).Asc()).
		Limit(vectorparity.TopK).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if got := queryIDs(t, pgPool(t), c); !slices.Equal(got, want) {
		t.Fatalf("pgvector exact search = %v, reference = %v", got, want)
	}
}
