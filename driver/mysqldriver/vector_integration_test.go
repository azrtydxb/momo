// SPDX-License-Identifier: Apache-2.0

//go:build integration

package mysqldriver_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/internal/vectorparity"
	"github.com/azrtydxb/momo/vector"
)

type vecDoc struct {
	ID  int64
	Emb vector.Vector
}

var vecDocMeta = momo.Meta[vecDoc]{
	Table:      "docs",
	Columns:    []momo.ColumnMeta{{Name: "id"}, {Name: "emb"}},
	Ptr:        func(m *vecDoc, i int) any { return []any{&m.ID, &m.Emb}[i] },
	Value:      func(m *vecDoc, i int) any { return []any{m.ID, m.Emb}[i] },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0},
}

func (d *vecDoc) MomoMeta() *momo.Meta[vecDoc] { return &vecDocMeta }

// serverVersion returns SELECT VERSION().
func serverVersion(t *testing.T, q momo.Querier) string {
	t.Helper()
	rows, err := momo.MapRows(context.Background(), q, "SELECT VERSION() AS v")
	if err != nil || len(rows) != 1 {
		t.Fatalf("VERSION(): %v, %d rows", err, len(rows))
	}
	switch v := rows[0]["v"].(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		t.Fatalf("VERSION() is %T", v)
	}
	return ""
}

func vecText(t *testing.T, v vector.Vector) string {
	t.Helper()
	s, err := v.Value()
	if err != nil {
		t.Fatal(err)
	}
	return s.(string)
}

func rowIDs(t *testing.T, rows []map[string]any) []int64 {
	t.Helper()
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		id, ok := r["id"].(int64)
		if !ok {
			t.Fatalf("id is %T, want int64", r["id"])
		}
		ids = append(ids, id)
	}
	return ids
}

// TestMariaDBVectorParity holds MariaDB to the exact nearest neighbours that
// the shared reference computes in Go; the root package test holds pgvector
// to the same reference, so both servers return the same ids.
func TestMariaDBVectorParity(t *testing.T) {
	ctx := context.Background()
	vecs := vectorparity.Vectors()
	query := vecs[0]
	want := vectorparity.ExactCosineTopK(vecs, query, vectorparity.TopK)

	db := testopen.MariaDB(t)
	if !suite.CapsOf(db).Vectors {
		t.Fatal("MariaDB driver must report Capabilities().Vectors")
	}
	mustExecMySQL(t, db, fmt.Sprintf("CREATE TABLE docs (id bigint PRIMARY KEY, emb VECTOR(%d) NOT NULL, VECTOR INDEX (emb) M=8 DISTANCE=cosine)", vectorparity.Dim))
	// Load through the typed insert path, which must wrap the vector value.
	models := make([]vecDoc, len(vecs))
	for i, v := range vecs {
		models[i] = vecDoc{ID: int64(i + 1), Emb: v}
	}
	if _, err := momo.Insert[vecDoc](db).Models(models).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	// Round trip: a stored vector reads back unchanged.
	back, err := momo.Select[vecDoc](db).Where(momo.NewColumn[int64]("docs", "id", 0).Eq(1)).All(ctx)
	if err != nil || len(back) != 1 || !slices.Equal(back[0].Emb, vecs[0]) {
		t.Fatalf("round trip: %v, %v", back, err)
	}

	// Exact path: "+ 0" keeps the server from using the index.
	rows, err := momo.MapRows(ctx, db, "SELECT id FROM docs ORDER BY VEC_DISTANCE_COSINE(emb, VEC_FromText(?)) + 0, id LIMIT 10", vecText(t, query))
	if err != nil {
		t.Fatal(err)
	}
	if got := rowIDs(t, rows); !slices.Equal(got, want) {
		t.Fatalf("exact path: mariadb %v, reference %v", got, want)
	}

	// Indexed path: the momo query, served from the VECTOR INDEX.
	col := momo.NewVectorColumn[vector.Vector]("docs", "emb", vectorparity.Dim, 0)
	sel := momo.Select[vecDoc](db).OrderBy(momo.CosineDistance(col, query).Asc()).Limit(vectorparity.TopK)
	c, err := sel.SQL()
	if err != nil {
		t.Fatal(err)
	}
	if plan, err := momo.MapRows(ctx, db, "EXPLAIN "+c.SQL, c.Args...); err == nil {
		t.Logf("EXPLAIN: %v", plan)
	}
	got, err := sel.All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	overlap := 0
	for _, d := range got {
		if slices.Contains(want, d.ID) {
			overlap++
		}
	}
	if len(got) != vectorparity.TopK || overlap < vectorparity.MinIndexedOverlap {
		t.Fatalf("indexed path returned %d rows sharing %d of %d exact ids, want %d sharing at least %d",
			len(got), overlap, vectorparity.TopK, vectorparity.TopK, vectorparity.MinIndexedOverlap)
	}

	// Euclidean distance is available too; the query vector itself is nearest.
	l2, err := momo.Select[vecDoc](db).OrderBy(momo.L2Distance(col, query).Asc()).Limit(1).All(ctx)
	if err != nil || len(l2) != 1 || l2[0].ID != 1 {
		t.Fatalf("L2 nearest = %v, %v, want id 1", l2, err)
	}

	// Inner product has no MariaDB function: refused, never emulated.
	_, err = momo.Select[vecDoc](db).OrderBy(momo.InnerProductDistance(col, query).Asc()).Limit(1).All(ctx)
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("inner product: err = %v, want ErrUnsupported", err)
	}
}

func TestMySQLVectorCapability(t *testing.T) {
	ctx := context.Background()
	t.Run("mysql-8.4 refuses vector columns", func(t *testing.T) {
		db := testopen.MySQL84(t)
		if suite.CapsOf(db).Vectors {
			t.Fatal("MySQL driver must not report Capabilities().Vectors")
		}
		version := serverVersion(t, db)
		_, err := momo.MySQLVectorType("mysql", version, 3)
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("MySQLVectorType(%s): err = %v, want momo.ErrUnsupported", version, err)
		}
		if !strings.Contains(err.Error(), "8.4") {
			t.Fatalf("diagnostic %q does not name the server version", err)
		}
		// The server really has no such type.
		if _, err := momo.Exec(ctx, db, "CREATE TABLE vtab (id bigint PRIMARY KEY, emb VECTOR(3))"); err == nil {
			t.Fatal("MySQL 8.4 accepted a VECTOR column; the version gate would be wrong")
		}
	})
	t.Run("mysql-9.7 stores and refuses distance", func(t *testing.T) {
		db := testopen.MySQL97(t)
		typ, err := momo.MySQLVectorType("mysql", serverVersion(t, db), 3)
		if err != nil {
			t.Fatal(err)
		}
		mustExecMySQL(t, db, "CREATE TABLE docs (id bigint PRIMARY KEY, emb "+typ+" NOT NULL)")
		in := vecDoc{ID: 1, Emb: vector.Vector{1, 2, 3}}
		if _, err := momo.Insert[vecDoc](db).Model(&in).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		got, err := momo.Select[vecDoc](db).All(ctx)
		if err != nil || len(got) != 1 || !slices.Equal(got[0].Emb, in.Emb) {
			t.Fatalf("round trip: %v, %v", got, err)
		}
		col := momo.NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
		_, err = momo.Select[vecDoc](db).OrderBy(momo.L2Distance(col, vector.Vector{1, 2, 3}).Asc()).Limit(1).All(ctx)
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("distance query: err = %v, want momo.ErrUnsupported", err)
		}
	})
}
