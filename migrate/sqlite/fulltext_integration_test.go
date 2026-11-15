// SPDX-License-Identifier: Apache-2.0

//go:build integration && cgo && !windows

package sqlite_test

import (
	"context"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	"github.com/azrtydxb/momo/schema"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

// ftsDeclared is the model schema: the declaration only, as the snapshot holds it.
func ftsDeclared() *schema.Schema {
	return &schema.Schema{Dialect: "sqlite", Tables: []schema.Table{{
		Name: "articles",
		Columns: []schema.Column{
			{Name: "id", Type: "INTEGER", Identity: "by_default"},
			{Name: "title", Type: "TEXT", Nullable: true},
			{Name: "body", Type: "TEXT", Nullable: true},
		},
		PrimaryKey: []string{"id"},
		Indexes: []schema.Index{{Name: "articles_search", Method: "fulltext",
			Parts:    []schema.IndexPart{{Column: "title"}, {Column: "body"}},
			FullText: &schema.FullText{Columns: []string{"title", "body"}, Tokenizer: "porter unicode61"}}},
	}}}
}

// ftsMigrate writes and applies the first migration of want. It returns the
// engine that applied it and the migration files.
func ftsMigrate(t *testing.T, db *momo.DB, want *schema.Schema) (*migrate.Engine, map[string][]byte) {
	t.Helper()
	ctx := context.Background()
	m, err := newEngine(t, db, migratetest.EmptyFS(), "20260101000000").Diff(ctx, want, "fts", migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if m == nil {
		t.Fatal("expected a migration")
	}
	eng := newEngine(t, db, migratetest.FS(m.Files), "20260102000000")
	if _, err := eng.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatalf("apply: %v\n%s", err, upFile(t, m.Files))
	}
	return eng, m.Files
}

func ftsExec(t *testing.T, db *momo.DB, sql string, args ...any) {
	t.Helper()
	if _, err := db.Driver().Exec(context.Background(), sql, args); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func ftsSearch(t *testing.T, db *momo.DB, q string) []int64 {
	return ftsSearchIn(t, db, "articles_fts", q)
}

func ftsSearchIn(t *testing.T, db *momo.DB, fts, q string) []int64 {
	t.Helper()
	rows, err := db.Driver().Query(context.Background(), `SELECT rowid FROM "`+fts+`" WHERE "`+fts+`" MATCH ? ORDER BY rowid`, []any{q})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

// Catches: a second diff after apply that is not empty, a stale or missing row
// after an insert, update or delete, and an integrity-check failure after an
// update followed by a delete.
func TestFullTextSQLiteMigrationRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testopen.SQLite(t)
	want := ftsDeclared()
	eng, _ := ftsMigrate(t, db, want)
	if m, err := eng.Diff(ctx, want, "again", migrate.DiffOptions{}); err != nil || m != nil {
		t.Fatalf("second diff = %v, %v; want none", m, err)
	}
	ftsExec(t, db, `INSERT INTO articles (id, title, body) VALUES (1, 'red fox', 'quick'), (2, 'blue whale', 'slow')`)
	if got := ftsSearch(t, db, `"fox"`); len(got) != 1 || got[0] != 1 {
		t.Fatalf("after insert: %v", got)
	}
	ftsExec(t, db, `UPDATE articles SET title = 'green frog' WHERE id = 1`)
	if got := ftsSearch(t, db, `"fox"`); len(got) != 0 {
		t.Fatalf("after update the old token is still indexed: %v", got)
	}
	if got := ftsSearch(t, db, `"frog"`); len(got) != 1 {
		t.Fatalf("after update the new token is missing: %v", got)
	}
	ftsExec(t, db, `DELETE FROM articles WHERE id = 2`)
	if got := ftsSearch(t, db, `"whale"`); len(got) != 0 {
		t.Fatalf("after delete: %v", got)
	}
	ftsExec(t, db, `INSERT INTO articles_fts(articles_fts) VALUES('integrity-check')`)
}

// Catches: a trigger that does not keep the FTS5 table in step on one of the
// three events, and a stemmer that is not applied.
func TestFullTextSQLiteSync(t *testing.T) {
	db := testopen.SQLite(t)
	ftsMigrate(t, db, ftsDeclared())
	t.Run("insert", func(t *testing.T) {
		ftsExec(t, db, `INSERT INTO articles (id, title, body) VALUES (1, 'running dogs', NULL)`)
		if got := ftsSearch(t, db, `"run"`); len(got) != 1 {
			t.Fatalf("porter stemming should match run: %v", got)
		}
	})
	t.Run("update", func(t *testing.T) {
		ftsExec(t, db, `UPDATE articles SET body = 'sleeping cats' WHERE id = 1`)
		if got := ftsSearch(t, db, `"cats"`); len(got) != 1 {
			t.Fatalf("update not indexed: %v", got)
		}
	})
	t.Run("delete", func(t *testing.T) {
		ftsExec(t, db, `DELETE FROM articles WHERE id = 1`)
		if got := ftsSearch(t, db, `"cats"`); len(got) != 0 {
			t.Fatalf("delete not indexed: %v", got)
		}
	})
}

// Catches: the update trigger inserting before it deletes, which corrupts the
// index only once a later delete reads the stale entry.
func TestFullTextSQLiteIntegrityAfterUpdateDelete(t *testing.T) {
	db := testopen.SQLite(t)
	ftsMigrate(t, db, ftsDeclared())
	ftsExec(t, db, `INSERT INTO articles (id, title, body) VALUES (1, 'alpha beta', 'gamma')`)
	ftsExec(t, db, `UPDATE articles SET title = 'delta epsilon' WHERE id = 1`)
	ftsExec(t, db, `DELETE FROM articles WHERE id = 1`)
	ftsExec(t, db, `INSERT INTO articles_fts(articles_fts) VALUES('integrity-check')`)
}

// Catches: the FTS5 triggers lost, or the table left unsynchronised, when the
// content table is rebuilt (adding a CHECK rebuilds it and drops its triggers).
func TestFullTextSQLiteBaseTableRebuild(t *testing.T) {
	ctx := context.Background()
	db := testopen.SQLite(t)
	want := ftsDeclared()
	eng, files := ftsMigrate(t, db, want)
	ftsExec(t, db, `INSERT INTO articles (id, title, body) VALUES (1, 'red fox', 'quick')`)

	next := ftsDeclared()
	// A CHECK constraint cannot be added in place: SQLite rebuilds the table.
	next.Tables[0].Checks = []schema.Check{{Name: "title_short", Expr: "length(title) < 1000"}}
	m, err := eng.Diff(ctx, next, "title_not_null", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatalf("diff = %v, %v", m, err)
	}
	if !strings.Contains(upFile(t, m.Files), `CREATE TABLE "new_articles"`) {
		t.Fatalf("expected a table rebuild:\n%s", upFile(t, m.Files))
	}
	e3 := newEngine(t, db, migratetest.FS(mergeFiles(files, m.Files)), "20260103000000")
	if _, err := e3.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatalf("apply of the rebuild: %v\n%s", err, upFile(t, m.Files))
	}
	if got := ftsSearch(t, db, `"fox"`); len(got) != 1 {
		t.Fatalf("row lost from the index by the rebuild: %v", got)
	}
	ftsExec(t, db, `INSERT INTO articles (id, title, body) VALUES (2, 'blue whale', 'slow')`)
	if got := ftsSearch(t, db, `"whale"`); len(got) != 1 {
		t.Fatalf("triggers missing after the rebuild: %v", got)
	}
	if again, err := e3.Diff(ctx, next, "again", migrate.DiffOptions{}); err != nil || again != nil {
		t.Fatalf("diff after the rebuild = %v, %v; want none", again, err)
	}
}

// Catches: a dropped source column of the declaration leaving the old FTS5
// table behind (the index drop skipped as if the column drop took it along),
// so the re-create fails; and triggers that still name the dropped column.
func TestFullTextSQLiteDropSourceColumn(t *testing.T) {
	ctx := context.Background()
	db := testopen.SQLite(t)
	eng, files := ftsMigrate(t, db, ftsDeclared())
	ftsExec(t, db, `INSERT INTO articles (id, title, body) VALUES (1, 'red fox', 'quick')`)

	next := ftsDeclared()
	next.Tables[0].Columns = next.Tables[0].Columns[:2] // body is gone
	ix := &next.Tables[0].Indexes[0]
	ix.Parts = []schema.IndexPart{{Column: "title"}}
	ix.FullText.Columns = []string{"title"}
	m, err := eng.Diff(ctx, next, "drop_body", migrate.DiffOptions{AllowDestructive: true})
	if err != nil || m == nil {
		t.Fatalf("diff = %v, %v", m, err)
	}
	up := upFile(t, m.Files)
	if !strings.Contains(up, `DROP TABLE "articles_fts"`) || !strings.Contains(up, `CREATE VIRTUAL TABLE "articles_fts"`) {
		t.Fatalf("the FTS5 table must be dropped and created again:\n%s", up)
	}
	e3 := newEngine(t, db, migratetest.FS(mergeFiles(files, m.Files)), "20260103000000")
	if _, err := e3.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatalf("apply: %v\n%s", err, up)
	}
	if got := ftsSearch(t, db, `"fox"`); len(got) != 1 {
		t.Fatalf("existing row missing from the new FTS5 table: %v", got)
	}
	ftsExec(t, db, `INSERT INTO articles (id, title) VALUES (2, 'blue whale')`)
	ftsExec(t, db, `UPDATE articles SET title = 'green frog' WHERE id = 2`)
	if got := ftsSearch(t, db, `"frog"`); len(got) != 1 {
		t.Fatalf("triggers do not keep the new FTS5 table in step: %v", got)
	}
	ftsExec(t, db, `INSERT INTO articles_fts(articles_fts) VALUES('integrity-check')`)
	if again, err := e3.Diff(ctx, next, "again", migrate.DiffOptions{}); err != nil || again != nil {
		t.Fatalf("diff after the change = %v, %v; want none", again, err)
	}
}

// Catches: ALTER TABLE RENAME leaving the FTS5 table's content= option on the
// old name, so a search after the rename finds nothing or fails, and the old
// FTS5 table or its triggers surviving the rename.
func TestFullTextSQLiteTableRename(t *testing.T) {
	ctx := context.Background()
	db := testopen.SQLite(t)
	eng, files := ftsMigrate(t, db, ftsDeclared())
	ftsExec(t, db, `INSERT INTO articles (id, title, body) VALUES (1, 'red fox', 'quick')`)

	next := ftsDeclared()
	next.Tables[0].Name = "posts"
	next.Tables[0].Indexes[0].Name = "posts_search"
	m, err := eng.Diff(ctx, next, "rename", migrate.DiffOptions{RenameTables: map[string]string{"articles": "posts"}})
	if err != nil || m == nil {
		t.Fatalf("diff = %v, %v", m, err)
	}
	up := upFile(t, m.Files)
	e3 := newEngine(t, db, migratetest.FS(mergeFiles(files, m.Files)), "20260103000000")
	if _, err := e3.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatalf("apply: %v\n%s", err, up)
	}
	if got := ftsSearchIn(t, db, "posts_fts", `"fox"`); len(got) != 1 {
		t.Fatalf("existing row missing after the rename: %v\n%s", got, up)
	}
	ftsExec(t, db, `INSERT INTO posts (id, title, body) VALUES (2, 'blue whale', 'slow')`)
	ftsExec(t, db, `UPDATE posts SET title = 'green frog' WHERE id = 1`)
	if got := ftsSearchIn(t, db, "posts_fts", `"frog"`); len(got) != 1 {
		t.Fatalf("triggers do not follow the rename: %v", got)
	}
	ftsExec(t, db, `INSERT INTO posts_fts(posts_fts) VALUES('integrity-check')`)
	var n int
	rows, err := db.Driver().Query(ctx, `SELECT count(*) FROM sqlite_schema WHERE name LIKE 'articles%'`, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() || rows.Scan(&n) != nil || n != 0 {
		t.Fatalf("objects of the old name survive the rename: %d", n)
	}
}

// Catches: a cgo SQLite build without FTS5 or bm25, reported as the raw driver
// string. The pure-Go driver runs the same probe in driver/sqlitepure.
func TestFullTextSQLiteDriverSupport(t *testing.T) {
	ctx := context.Background()
	db := testopen.SQLite(t)
	ftsExec(t, db, `CREATE VIRTUAL TABLE p USING fts5(a)`)
	ftsExec(t, db, `INSERT INTO p(a) VALUES ('x')`)
	rows, err := db.Driver().Query(ctx, `SELECT bm25(p) FROM p WHERE p MATCH 'x'`, nil)
	if err != nil {
		if strings.Contains(err.Error(), "no such module") {
			t.Fatalf("the cgo SQLite build has no FTS5 (build with -tags sqlite_fts5 or CGO_CFLAGS=-DSQLITE_ENABLE_FTS5 and CGO_LDFLAGS=-lm): %v", err)
		}
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatal("bm25 probe returned no row")
	}
}
