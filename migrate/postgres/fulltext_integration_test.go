// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"testing"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

// Catches: a second diff after apply that is not empty, and a NULL source
// column hiding a row that has text in the other column.
func TestFullTextPostgresMigrationRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testopen.Postgres(t)
	want := &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{
		Name: "articles",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint", Identity: "by_default"},
			{Name: "title", Type: "text", Nullable: true},
			{Name: "body", Type: "text", Nullable: true},
		},
		PrimaryKey: []string{"id"},
		Indexes: []schema.Index{{Name: "articles_search", Method: "fulltext",
			Parts:    []schema.IndexPart{{Column: "title"}, {Column: "body"}},
			FullText: &schema.FullText{Columns: []string{"title", "body"}, Config: "english", Weights: []string{"A", "B"}}}},
	}}}

	gen := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
	mig, err := gen.Diff(ctx, want, "fulltext", migrate.DiffOptions{})
	if err != nil || mig == nil {
		t.Fatalf("diff: %v %v", mig, err)
	}
	eng := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(mig.Files))
	if _, err := eng.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	again, err := eng.Diff(ctx, want, "again", migrate.DiffOptions{})
	if err != nil || again != nil {
		t.Fatalf("second diff is not empty: %v %v", again, err)
	}

	mustExec(t, db.Driver(), `INSERT INTO articles (title, body) VALUES ('red fox', NULL), (NULL, 'red fox')`)
	rows, err := db.Driver().Query(ctx, `SELECT count(*) FROM articles WHERE articles_search_tsv @@ websearch_to_tsquery('english', 'fox')`, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var n int
	if !rows.Next() {
		t.Fatal("count row missing")
	}
	if err := rows.Scan(&n); err != nil || n != 2 {
		t.Fatalf("matches = %d (%v), want 2: a NULL source column must not null the vector", n, err)
	}
}
