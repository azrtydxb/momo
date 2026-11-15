// SPDX-License-Identifier: Apache-2.0

//go:build integration

package fts_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/azrtydxb/momo/fts"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
	"github.com/azrtydxb/momo/schema"
)

// Catches: the migration's FTS5 table and the query layer disagreeing about the
// table name or the content columns, so a search over a migrated table finds
// nothing; and rows written after the migration not reaching the index.
func TestFullTextSearchOverMigratedSQLiteTable(t *testing.T) {
	ctx := context.Background()
	db := testopen.SQLite(t)

	decl := *articleMeta.TableSchema
	decl.Columns = []schema.Column{
		{Name: "id", Type: "INTEGER", Identity: "by_default"},
		{Name: "title", Type: "TEXT", Nullable: true},
		{Name: "body", Type: "TEXT", Nullable: true},
	}
	want := &schema.Schema{Dialect: "sqlite", Tables: []schema.Table{decl}}
	eng := migratetest.NewEngine(t, msqlite.New(), db.Driver(), migratetest.EmptyFS())
	m, err := eng.Diff(ctx, want, "articles", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatalf("diff = %v, %v", m, err)
	}
	applier := migratetest.NewEngine(t, msqlite.New(), db.Driver(), migratetest.FS(m.Files))
	if _, err := applier.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	execAll(t, db,
		`INSERT INTO articles (id, title, body) VALUES (1, 'red fox', 'the red fox jumps')`,
		`INSERT INTO articles (id, title, body) VALUES (2, 'blue whale', 'slow and large')`,
		`INSERT INTO articles (id, title, body) VALUES (3, 'red hen', 'a red hen and a fox')`)
	if got := searchIDs(t, db, fts.Web(`red fox`)); !reflect.DeepEqual(got, []int64{1, 3}) {
		t.Fatalf("search after insert: %v, want [1 3]", got)
	}
	execAll(t, db, `UPDATE articles SET title = 'green frog', body = 'pond' WHERE id = 1`)
	if got := searchIDs(t, db, fts.Web(`red fox`)); !reflect.DeepEqual(got, []int64{3}) {
		t.Fatalf("search after update: %v, want [3]", got)
	}
	execAll(t, db, `DELETE FROM articles WHERE id = 3`)
	if got := searchIDs(t, db, fts.Web(`fox`)); len(got) != 0 {
		t.Fatalf("search after delete: %v, want none", got)
	}
}
