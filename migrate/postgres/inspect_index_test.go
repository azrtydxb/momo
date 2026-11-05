// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"slices"
	"testing"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

func TestPostgresIndexInspection(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExec(t, db.Driver(), `CREATE EXTENSION IF NOT EXISTS vector`)
	mustExec(t, db.Driver(), `CREATE TABLE d (id bigint PRIMARY KEY, title text, emb vector(3), deleted_at timestamptz)`)
	mustExec(t, db.Driver(), `CREATE INDEX d_lower_title ON d (lower(title))`)
	mustExec(t, db.Driver(), `CREATE INDEX d_live ON d (id) WHERE deleted_at IS NULL`)
	mustExec(t, db.Driver(), `CREATE INDEX d_emb ON d USING hnsw (emb vector_cosine_ops) WITH (m = 16, ef_construction = 64)`)
	got, err := mpg.New().Inspect(ctx, pinned(t, db), migrate.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tbl, ok := got.Table("d")
	if !ok {
		t.Fatalf("table %q missing", "d")
	}
	byName := map[string]schema.Index{}
	for _, ix := range tbl.Indexes {
		byName[ix.Name] = ix
	}
	must := func(name string) schema.Index {
		ix, ok := byName[name]
		if !ok {
			t.Fatalf("index %q missing", name)
		}
		return ix
	}
	if must("d_lower_title").Parts[0].Expr != "lower(title)" {
		t.Fatalf("expression index: %+v", must("d_lower_title"))
	}
	if must("d_live").Where != "(deleted_at IS NULL)" {
		t.Fatalf("partial index predicate: %q", must("d_live").Where)
	}
	emb := must("d_emb")
	if emb.Method != "hnsw" || emb.Parts[0].OpClass != "vector_cosine_ops" || len(emb.With) != 2 {
		t.Fatalf("hnsw index: %+v", emb)
	}
}

// TestPostgresIndexInspectionDetails covers the rules the plan test does not:
// default method and opclass are left empty, sort order and NULLS placement
// are recorded only when non-default, unique indexes keep their flag while
// constraint-backed ones are left to the constraint, storage parameters keep
// their order, and ivfflat is read like hnsw (vector_l2_ops is its default
// opclass, so it is not recorded).
func TestPostgresIndexInspectionDetails(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExec(t, db.Driver(), `CREATE EXTENSION IF NOT EXISTS vector`)
	mustExec(t, db.Driver(), `CREATE TABLE "Mixed" (id bigint PRIMARY KEY, a text, b text, "Cap" int, emb vector(3), UNIQUE (b))`)
	mustExec(t, db.Driver(), `CREATE INDEX "Mixed_a_idx" ON "Mixed" (a)`)
	mustExec(t, db.Driver(), `CREATE UNIQUE INDEX "Mixed_Cap_key" ON "Mixed" ("Cap")`)
	mustExec(t, db.Driver(), `CREATE INDEX mixed_sort ON "Mixed" (a DESC NULLS LAST, b NULLS FIRST, "Cap" DESC)`)
	mustExec(t, db.Driver(), `CREATE INDEX mixed_l2 ON "Mixed" USING ivfflat (emb vector_l2_ops) WITH (lists = 50)`)
	got, err := mpg.New().Inspect(ctx, pinned(t, db), migrate.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tbl, _ := got.Table("Mixed")
	byName := map[string]schema.Index{}
	for _, ix := range tbl.Indexes {
		byName[ix.Name] = ix
	}
	if _, ok := byName["Mixed_b_key"]; ok {
		t.Fatalf("constraint-backed unique index listed as an index: %+v", tbl.Indexes)
	}
	plain := byName["Mixed_a_idx"]
	if plain.Unique || plain.Method != "" || plain.Where != "" || len(plain.With) != 0 ||
		!slices.Equal(plain.Parts, []schema.IndexPart{{Column: "a"}}) {
		t.Fatalf("plain index: %+v", plain)
	}
	if u := byName["Mixed_Cap_key"]; !u.Unique || !slices.Equal(u.Parts, []schema.IndexPart{{Column: "Cap"}}) {
		t.Fatalf("unique index: %+v", u)
	}
	wantSort := []schema.IndexPart{{Column: "a", Desc: true, Nulls: "last"}, {Column: "b", Nulls: "first"}, {Column: "Cap", Desc: true}}
	if s := byName["mixed_sort"]; !slices.Equal(s.Parts, wantSort) {
		t.Fatalf("sort order: %+v want %+v", s.Parts, wantSort)
	}
	if l2 := byName["mixed_l2"]; l2.Method != "ivfflat" || l2.Parts[0].OpClass != "" ||
		!slices.Equal(l2.With, []schema.KV{{Key: "lists", Value: "50"}}) {
		t.Fatalf("ivfflat index: %+v", l2)
	}
}
