// SPDX-License-Identifier: Apache-2.0

//go:build integration && cgo && !windows

package sqlitedriver_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/driver/sqlitedriver"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/internal/vectorparity"
	"github.com/azrtydxb/momo/vector"
)

type parityDoc struct {
	ID  int64
	Emb vector.Vector
}

var parityDocMeta = momo.Meta[parityDoc]{
	Table:      "docs",
	Columns:    []momo.ColumnMeta{{Name: "id"}, {Name: "emb"}},
	Ptr:        func(m *parityDoc, i int) any { return []any{&m.ID, &m.Emb}[i] },
	Value:      func(m *parityDoc, i int) any { return []any{m.ID, m.Emb}[i] },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0},
}

func (d *parityDoc) MomoMeta() *momo.Meta[parityDoc] { return &parityDocMeta }

func parityIDs(docs []parityDoc) []int64 {
	ids := make([]int64, len(docs))
	for i, d := range docs {
		ids[i] = d.ID
	}
	return ids
}

// TestSQLiteVectorParity holds sqlite-vec, driven through the typed distance
// builders, to the exact nearest neighbours that the shared reference computes
// in plain Go. The root package test holds pgvector exact search to the same
// reference for cosine and the MariaDB test does so as well, so the three
// servers return the same ids. An approximate index is refused, never
// silently accepted.
func TestSQLiteVectorParity(t *testing.T) {
	ctx := context.Background()
	vecs := vectorparity.Vectors()
	query := vecs[0]

	sqldb, drv, err := sqlitedriver.Open(testdb.SQLitePath(t), sqlitedriver.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sqldb.Close(); err != nil {
			t.Error(err)
		}
	})
	db := momo.New(drv)
	typ, err := momo.SQLiteVectorType("emb", vectorparity.Dim)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := momo.Exec(ctx, db, fmt.Sprintf("CREATE TABLE docs (id INTEGER PRIMARY KEY, emb %s NOT NULL)", typ)); err != nil {
		t.Fatal(err)
	}
	models := make([]parityDoc, len(vecs))
	for i, v := range vecs {
		models[i] = parityDoc{ID: int64(i + 1), Emb: v}
	}
	if _, err := momo.Insert[parityDoc](db).Models(models).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	col := momo.NewVectorColumn[vector.Vector]("docs", "emb", vectorparity.Dim, 0)
	id := momo.NewColumn[int64]("docs", "id", 0)
	cases := []struct {
		name string
		dist momo.DistanceExpr
		want []int64
	}{
		{"cosine", momo.CosineDistance(col, query), vectorparity.ExactCosineTopK(vecs, query, vectorparity.TopK)},
		{"l2", momo.L2Distance(col, query), vectorparity.ExactTopK(vecs, query, vectorparity.TopK, vectorparity.L2Distance)},
		{"l1", momo.L1Distance(col, query), vectorparity.ExactTopK(vecs, query, vectorparity.TopK, vectorparity.L1Distance)},
	}
	for _, c := range cases {
		got, err := momo.Select[parityDoc](db).OrderBy(c.dist.Asc(), id.Asc()).Limit(vectorparity.TopK).All(ctx)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if ids := parityIDs(got); !slices.Equal(ids, c.want) {
			t.Fatalf("%s top-%d: sqlite-vec %v, reference %v", c.name, vectorparity.TopK, ids, c.want)
		}
		if !slices.Equal(got[0].Emb, query) {
			t.Fatalf("%s: the nearest row does not read back as the query vector", c.name)
		}
	}

	// Desc orders farthest first: the row last in the reference order is first.
	far, err := momo.Select[parityDoc](db).OrderBy(cases[1].dist.Desc(), id.Asc()).Limit(1).All(ctx)
	if err != nil || len(far) != 1 {
		t.Fatalf("farthest: %v, %v", far, err)
	}
	all := vectorparity.ExactTopK(vecs, query, len(vecs), vectorparity.L2Distance)
	if far[0].ID != all[len(all)-1] {
		t.Fatalf("farthest L2 row = %d, reference %d", far[0].ID, all[len(all)-1])
	}

	// No ANN index: the dialect lists none, the check refuses every method,
	// and SQLite itself does not take an index method.
	if idx := dialect.For(dialect.SQLite).Vector.Indexes; len(idx) != 0 {
		t.Fatalf("the SQLite dialect lists vector indexes %v", idx)
	}
	for _, method := range []string{"hnsw", "ivfflat", "vector"} {
		if err := momo.CheckVectorIndexMethod(drv.Dialect(), method); !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("CheckVectorIndexMethod(%q) = %v, want ErrUnsupported", method, err)
		}
	}
	if _, err := momo.Exec(ctx, db, "CREATE INDEX docs_emb ON docs USING hnsw (emb)"); err == nil {
		t.Fatal("SQLite accepted CREATE INDEX ... USING hnsw")
	}

	// The same data in pgvector, searched exactly (the table has no index): the
	// same builders return the same ids. Skipped when no PostgreSQL URL is set.
	t.Run("pgvector", func(t *testing.T) {
		pg := testopen.Postgres(t)
		if _, err := momo.Exec(ctx, pg, fmt.Sprintf("CREATE TABLE docs (id bigint PRIMARY KEY, emb vector(%d) NOT NULL)", vectorparity.Dim)); err != nil {
			t.Fatal(err)
		}
		for i, v := range vecs {
			if _, err := momo.Exec(ctx, pg, "INSERT INTO docs (id, emb) VALUES (?, ?)", int64(i+1), v); err != nil {
				t.Fatal(err)
			}
		}
		for _, c := range cases {
			sel, err := momo.Select[parityDoc](pg).OrderBy(c.dist.Asc(), id.Asc()).Limit(vectorparity.TopK).Columns(id).SQL()
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			rows, err := pg.Driver().Query(ctx, sel.SQL, sel.Args)
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			var ids []int64
			for rows.Next() {
				var n int64
				if err := rows.Scan(&n); err != nil {
					t.Fatalf("%s: %v", c.name, err)
				}
				ids = append(ids, n)
			}
			if err := errors.Join(rows.Err(), rows.Close()); err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if !slices.Equal(ids, c.want) {
				t.Fatalf("%s top-%d: pgvector exact %v, sqlite-vec and reference %v", c.name, vectorparity.TopK, ids, c.want)
			}
		}
	})
}
