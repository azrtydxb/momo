// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo"
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

// ftStep generates the migration that takes the database to want, applies it,
// and requires the next diff to be empty. The files of every step stay in dir.
func ftStep(t *testing.T, drv momo.Driver, dir fstest.MapFS, want *schema.Schema, name string, at time.Time) *migrate.Migration {
	t.Helper()
	ctx := context.Background()
	gen := migratetest.NewEngine(t, mpg.New(), drv, dir, migrate.WithClock(func() time.Time { return at }))
	m, err := gen.Diff(ctx, want, name, migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatalf("diff %s: %v %v", name, m, err)
	}
	for n, b := range m.Files {
		dir[n] = &fstest.MapFile{Data: b}
	}
	eng := migratetest.NewEngine(t, mpg.New(), drv, dir, migrate.WithClock(func() time.Time { return at.Add(time.Hour) }))
	if _, err := eng.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatalf("apply %s: %v", name, err)
	}
	if again, err := eng.Diff(ctx, want, name+"_again", migrate.DiffOptions{}); err != nil || again != nil {
		t.Fatalf("diff after %s is not empty: %v %v", name, again, err)
	}
	return m
}

func ftScalar(t *testing.T, drv momo.Driver, sql string) int {
	t.Helper()
	rows, err := drv.Query(context.Background(), sql, nil)
	if err != nil {
		t.Fatalf("query %q: %v", sql, err)
	}
	defer func() { _ = rows.Close() }()
	var n int
	if !rows.Next() {
		t.Fatalf("query %q: no row", sql)
	}
	if err := rows.Scan(&n); err != nil {
		t.Fatalf("query %q: %v", sql, err)
	}
	return n
}

// Catches: a concurrent full-text declaration on a table that has rows failing
// to apply as a transactional column add plus a no-transaction index build, an
// index left invalid, and a following diff that is not empty.
func TestFullTextPostgresConcurrentlyAddedToExistingTable(t *testing.T) {
	db := testopen.Postgres(t)
	dir := fstest.MapFS{}
	ftStep(t, db.Driver(), dir, splitArticles(nil, false), "init", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	mustExec(t, db.Driver(), `INSERT INTO articles (title, body) VALUES ('red fox', NULL), (NULL, 'red fox')`)

	ft := &schema.FullText{Columns: []string{"title", "body"}, Config: "english"}
	m := ftStep(t, db.Driver(), dir, splitArticles(ft, true), "search", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	if m.Next == nil || !m.Next.NoTx {
		t.Fatalf("want a trailing no-transaction migration: %+v", m)
	}
	if n := ftScalar(t, db.Driver(), `SELECT count(*) FROM pg_index WHERE indexrelid = 'articles_search'::regclass AND indisvalid`); n != 1 {
		t.Fatalf("the concurrent index is missing or invalid (%d)", n)
	}
	if n := ftScalar(t, db.Driver(), `SELECT count(*) FROM articles WHERE articles_search_tsv @@ websearch_to_tsquery('english', 'fox')`); n != 2 {
		t.Fatalf("matches = %d, want 2", n)
	}
}

// Catches: a Config and Weights change of a concurrent declaration that does not
// apply (the old index dropped outside the transaction that recreates the
// column, or the column change refused), leaves the old vector in place, or
// leaves a following diff that is not empty.
func TestFullTextPostgresConcurrentlyConfigChange(t *testing.T) {
	db := testopen.Postgres(t)
	dir := fstest.MapFS{}
	ftStep(t, db.Driver(), dir, splitArticles(&schema.FullText{Columns: []string{"title", "body"}, Config: "english"}, true), "init", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	mustExec(t, db.Driver(), `INSERT INTO articles (title, body) VALUES ('red foxes', NULL)`)
	if n := ftScalar(t, db.Driver(), `SELECT count(*) FROM articles WHERE articles_search_tsv @@ to_tsquery('simple', 'fox')`); n != 1 {
		t.Fatalf("english stemming: matches = %d, want 1", n)
	}
	// The word "the" is a stop word under english, so it is not in the old vector.
	mustExec(t, db.Driver(), `INSERT INTO articles (title, body) VALUES ('the', NULL)`)
	if n := ftScalar(t, db.Driver(), `SELECT count(*) FROM articles WHERE articles_search_tsv @@ to_tsquery('simple', 'the')`); n != 0 {
		t.Fatalf("english stop word: matches = %d, want 0", n)
	}

	m := ftStep(t, db.Driver(), dir, splitArticles(&schema.FullText{Columns: []string{"title", "body"}, Config: "simple", Weights: []string{"A", "B"}}, true), "retune", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	if m.Next == nil || !m.Next.NoTx {
		t.Fatalf("want a trailing no-transaction migration: %+v", m)
	}
	if n := ftScalar(t, db.Driver(), `SELECT count(*) FROM pg_index WHERE indexrelid = 'articles_search'::regclass AND indisvalid`); n != 1 {
		t.Fatalf("the rebuilt index is missing or invalid (%d)", n)
	}
	if n := ftScalar(t, db.Driver(), `SELECT count(*) FROM articles WHERE articles_search_tsv @@ to_tsquery('simple', 'fox')`); n != 0 {
		t.Fatalf("the vector still uses the english configuration: %d matches for the stem", n)
	}
	if n := ftScalar(t, db.Driver(), `SELECT count(*) FROM articles WHERE articles_search_tsv @@ to_tsquery('simple', 'foxes')`); n != 1 {
		t.Fatalf("the vector was not recomputed: %d matches for the word", n)
	}
	// Only the simple configuration keeps the stop word, so the row matches only
	// under the new vector.
	if n := ftScalar(t, db.Driver(), `SELECT count(*) FROM articles WHERE articles_search_tsv @@ to_tsquery('simple', 'the')`); n != 1 {
		t.Fatalf("a row that matches only the new configuration: %d matches, want 1", n)
	}
}
