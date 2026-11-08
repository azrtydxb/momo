// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
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
	got, err := inspectOnce(ctx, t, db, migrate.InspectOptions{})
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
	got, err := inspectOnce(ctx, t, db, migrate.InspectOptions{})
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

func TestInspectWithInvalidIndexes(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExec(t, db.Driver(), `CREATE TABLE inv (id bigint PRIMARY KEY, v int)`)
	mustExec(t, db.Driver(), `INSERT INTO inv VALUES (1, 7), (2, 7)`)
	// A CONCURRENTLY build over duplicate data fails and leaves an invalid index.
	if _, err := db.Driver().Exec(ctx, `CREATE UNIQUE INDEX CONCURRENTLY inv_v_key ON inv (v)`, nil); err == nil {
		t.Fatal("expected the unique build to fail")
	}
	mustExec(t, db.Driver(), `CREATE INDEX inv_ok ON inv (v)`)
	pc, err := db.Driver().(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	s, invalid, err := mpg.New().InspectWithInvalid(ctx, pc, migrate.InspectOptions{})
	_ = pc.Release()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(invalid, []string{"inv_v_key"}) {
		t.Fatalf("invalid = %v", invalid)
	}
	if tbl, _ := s.Table("inv"); len(tbl.Indexes) != 2 {
		t.Fatalf("indexes = %+v", tbl.Indexes)
	}
}

// A model that declares the default opclass and the storage parameters in a
// different order must compare Equal to what inspection reports.
func TestInspectedIndexEqualsModel(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExec(t, db.Driver(), `CREATE EXTENSION IF NOT EXISTS vector`)
	mustExec(t, db.Driver(), `CREATE TABLE m (id bigint PRIMARY KEY, title text, tags text[], emb vector(3))`)
	mustExec(t, db.Driver(), `CREATE INDEX m_emb_ivf ON m USING ivfflat (emb vector_l2_ops) WITH (lists = 10)`)
	mustExec(t, db.Driver(), `CREATE INDEX m_emb_hnsw ON m USING hnsw (emb vector_l2_ops) WITH (m = 16, ef_construction = 64)`)
	mustExec(t, db.Driver(), `CREATE INDEX m_title ON m (title text_ops)`)
	mustExec(t, db.Driver(), `CREATE INDEX m_tags ON m USING gin (tags array_ops)`)
	got, err := inspectOnce(ctx, t, db, migrate.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	inspected, _ := got.Table("m")
	model := &schema.Schema{FormatVersion: schema.FormatVersion, Dialect: got.Dialect, Extensions: got.Extensions, Tables: []schema.Table{{
		Name: "m", PrimaryKey: []string{"id"}, Columns: inspected.Columns,
		Indexes: []schema.Index{
			{Name: "m_emb_ivf", Method: "ivfflat", Parts: []schema.IndexPart{{Column: "emb", OpClass: "vector_l2_ops"}}, With: []schema.KV{{Key: "lists", Value: "10"}}},
			{Name: "m_emb_hnsw", Method: "hnsw", Parts: []schema.IndexPart{{Column: "emb", OpClass: "vector_l2_ops"}}, With: []schema.KV{{Key: "ef_construction", Value: "64"}, {Key: "m", Value: "16"}}},
			{Name: "m_title", Method: "btree", Parts: []schema.IndexPart{{Column: "title", OpClass: "text_ops"}}},
			{Name: "m_tags", Method: "gin", Parts: []schema.IndexPart{{Column: "tags", OpClass: "array_ops"}}},
		},
	}}}
	if !schema.Equal(model, got) {
		a, _ := json.MarshalIndent(schema.Normalized(model), "", " ")
		b, _ := json.MarshalIndent(schema.Normalized(got), "", " ")
		t.Fatalf("model != inspected\nmodel: %s\ninspected: %s", a, b)
	}
}
