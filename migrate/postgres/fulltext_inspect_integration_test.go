// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

// ftModel is the declaration only, as a model produces it.
func ftModel(ft schema.FullText) *schema.Schema {
	return &schema.Schema{Dialect: "postgres", Extensions: []schema.Extension{{Name: "vector"}}, // installed in the test database; inspection reports every extension
		Tables: []schema.Table{{
			Name: "articles",
			Columns: []schema.Column{
				{Name: "id", Type: "bigint", Identity: "by_default"},
				{Name: "title", Type: "text", Nullable: true},
				{Name: "body", Type: "text", Nullable: true},
			},
			PrimaryKey: []string{"id"},
			Indexes: []schema.Index{{Name: "articles_search", Method: "fulltext",
				Parts:    []schema.IndexPart{{Column: "title"}, {Column: "body"}},
				FullText: &ft}},
		}}}
}

var ftForms = map[string]schema.FullText{
	"stored":     {Columns: []string{"title", "body"}, Config: "english", Weights: []string{"A", "B"}},
	"unweighted": {Columns: []string{"title", "body"}, Config: "simple"},
	"expression": {Columns: []string{"title", "body"}, Config: "english", Expression: true},
}

// ftApply creates want through a migration and returns the engine that applied it.
func ftApply(t *testing.T, db *momo.DB, want *schema.Schema) (*migrate.Engine, *migrate.Migration) {
	t.Helper()
	ctx := context.Background()
	gen := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
	m, err := gen.Diff(ctx, want, "fulltext", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatalf("diff: %v %v", m, err)
	}
	eng := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(m.Files))
	if _, err := eng.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	return eng, m
}

func ftInspected(t *testing.T, db *momo.DB) *schema.Schema {
	t.Helper()
	got, err := inspectOnce(context.Background(), t, db, migrate.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tbl, ok := got.Table("articles")
	if !ok {
		t.Fatalf("articles not inspected: %+v", got.Tables)
	}
	return &schema.Schema{Dialect: "postgres", Tables: []schema.Table{*tbl}}
}

// Catches: an inspection that misses the full-text structure (the index read
// as a plain GIN index and the generated column as a plain column), a folded
// declaration that differs from the expanded model in config, weights or
// columns, and a drift report that lists the index as changed.
func TestFullTextInspectRoundTrip(t *testing.T) {
	for name, ft := range ftForms {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			db := testopen.Postgres(t)
			want := ftModel(ft)
			eng, _ := ftApply(t, db, want)

			expanded, err := mpg.New().ExpandSchema(want)
			if err != nil {
				t.Fatal(err)
			}
			got := ftInspected(t, db)
			if ix, ok := got.Tables[0].FullTextIndex(); !ok || ix.FullText == nil || ix.FullText.Config != ft.Config {
				t.Fatalf("inspection misses the full-text structure: %+v", got.Tables[0].Indexes)
			}
			if !schema.Equal(expanded, got) {
				t.Fatalf("inspection differs from the expanded declaration:\n%s", migratetest.Diff(got, expanded))
			}
			rep, err := eng.Drift(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !rep.Clean() {
				t.Fatalf("drift reports a change on a freshly applied full-text index: %+v", rep)
			}
		})
	}
}

// Catches: a hand-written generated tsvector column with a GIN index folded
// into a full-text declaration, which would make a trigger-free recipe look
// declared.
func TestFullTextPostgresPlainGINStaysPlain(t *testing.T) {
	db := testopen.Postgres(t)
	mustExec(t, db.Driver(), `CREATE TABLE articles (id bigint PRIMARY KEY, title text, v tsvector GENERATED ALWAYS AS (to_tsvector('simple', coalesce(title, '') || 'x')) STORED)`)
	mustExec(t, db.Driver(), `CREATE INDEX articles_v_idx ON articles USING gin (v)`)
	mustExec(t, db.Driver(), `CREATE INDEX articles_t_idx ON articles USING gin (to_tsvector('simple', title))`)
	got := ftInspected(t, db)
	if _, ok := got.Tables[0].FullTextIndex(); ok {
		t.Fatalf("a hand-written index was folded: %+v", got.Tables[0].Indexes)
	}
	for _, ix := range got.Tables[0].Indexes {
		if ix.Method != "gin" {
			t.Errorf("index %s method = %q, want gin", ix.Name, ix.Method)
		}
	}
}

// Catches: a second AutoMigrate that plans or runs anything on a model with a
// full-text declaration (the live index read back as a plain GIN index made it
// run CREATE INDEX on an index that exists).
func TestFullTextAutoMigrateTwice(t *testing.T) {
	for name, ft := range ftForms {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			db := testopen.Postgres(t)
			want := ftModel(ft)
			eng := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
			first, err := eng.AutoMigrate(ctx, want)
			if err != nil {
				t.Fatal(err)
			}
			if len(first.Applied) == 0 || len(first.Skipped) != 0 {
				t.Fatalf("first run: applied %d, skipped %+v", len(first.Applied), first.Skipped)
			}
			again, err := eng.AutoMigrate(ctx, want)
			if err != nil {
				t.Fatalf("second AutoMigrate: %v", err)
			}
			if len(again.Applied) != 0 || len(again.Skipped) != 0 {
				t.Fatalf("second AutoMigrate planned changes: applied %+v, skipped %+v", again.Applied, again.Skipped)
			}
		})
	}
}

// Catches: a baseline of a database that holds a full-text index refused as a
// mismatch, and drift after the baseline.
func TestFullTextBaselineClean(t *testing.T) {
	for name, ft := range ftForms {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			db := testopen.Postgres(t)
			want := ftModel(ft)
			auto := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
			if _, err := auto.AutoMigrate(ctx, want); err != nil {
				t.Fatal(err)
			}
			m, err := auto.Diff(ctx, want, "fulltext", migrate.DiffOptions{})
			if err != nil || m == nil {
				t.Fatalf("diff: %v %v", m, err)
			}
			eng := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(m.Files))
			if err := eng.Baseline(ctx, m.Version, false); err != nil {
				t.Fatalf("baseline: %v", err)
			}
			rep, err := eng.Drift(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !rep.Clean() {
				t.Fatalf("drift after the baseline: %+v", rep)
			}
		})
	}
}
