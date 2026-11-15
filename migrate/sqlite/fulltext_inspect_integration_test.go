// SPDX-License-Identifier: Apache-2.0

//go:build integration && cgo && !windows

package sqlite_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
	"github.com/azrtydxb/momo/schema"
)

func ftsInspect(t *testing.T, db *momo.DB) *schema.Schema {
	t.Helper()
	c := pinned(t, db)
	got, err := msqlite.New().Inspect(context.Background(), c, migrate.InspectOptions{})
	release(t, c)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// Catches: a shadow table or a non-FTS5 virtual table reported as a table, and
// an FTS5 table that is not folded into an index of its content table.
func TestSQLiteInspectIgnoresShadowTables(t *testing.T) {
	db := testopen.SQLite(t)
	for _, q := range []string{
		`CREATE TABLE articles (id INTEGER PRIMARY KEY, title TEXT, body TEXT)`,
		`CREATE VIRTUAL TABLE articles_fts USING fts5(title, body, content='articles', content_rowid='id')`,
		`CREATE VIRTUAL TABLE geo USING rtree(id, minx, maxx, miny, maxy)`,
	} {
		ftsExec(t, db, q)
	}
	s := ftsInspect(t, db)
	if len(s.Tables) != 1 || s.Tables[0].Name != "articles" {
		t.Fatalf("tables = %+v, want only articles", s.Tables)
	}
	ix, ok := s.Tables[0].FullTextIndex()
	if !ok || ix.Name != "articles_fts" || !reflect.DeepEqual(ix.FullText.Columns, []string{"title", "body"}) {
		t.Fatalf("full-text index = %+v %v", ix, ok)
	}
}

// Catches: an inspection that misses the full-text structure, a tokenizer lost
// on the way back, and an inspected schema that differs from the expanded
// declaration (a second AutoMigrate or a drift check would see a change).
func TestFullTextInspectRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testopen.SQLite(t)
	want := ftsDeclared()
	eng, _ := ftsMigrate(t, db, want)
	expanded, err := msqlite.New().ExpandSchema(want)
	if err != nil {
		t.Fatal(err)
	}
	got := ftsInspect(t, db)
	if ix, ok := got.Tables[0].FullTextIndex(); !ok || ix.FullText.Tokenizer != "porter unicode61" {
		t.Fatalf("inspection misses the full-text structure: %+v", got.Tables)
	}
	if !schema.Equal(expanded, got) {
		t.Fatalf("inspection differs from the expanded declaration:\n%s", migratetest.Diff(got, expanded))
	}
	rep, err := eng.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean() {
		t.Fatalf("drift on a freshly applied full-text table: %+v", rep)
	}
}

// Catches: a dropped sync trigger that drift does not report, which would leave
// the FTS5 table silently out of step with its content.
func TestFullTextSQLiteDriftReportsTrigger(t *testing.T) {
	ctx := context.Background()
	db := testopen.SQLite(t)
	eng, _ := ftsMigrate(t, db, ftsDeclared())
	ftsExec(t, db, `DROP TRIGGER articles_fts_au`)
	rep, err := eng.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Clean() || !strings.Contains(fmt.Sprintf("%+v", rep.Missing), "articles_fts_au") {
		t.Fatalf("drift = %+v, want the missing trigger articles_fts_au", rep)
	}
}

// Catches: a second AutoMigrate that plans or runs anything on a model with a
// full-text declaration, and sync triggers that the first run skipped (the FTS5
// table would not follow its content).
func TestFullTextAutoMigrateTwice(t *testing.T) {
	ctx := context.Background()
	db := testopen.SQLite(t)
	want := ftsDeclared()
	eng := newEngine(t, db, migratetest.EmptyFS(), "20260101000000")
	first, err := eng.AutoMigrate(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Applied) == 0 || len(first.Skipped) != 0 {
		t.Fatalf("first run: applied %d, skipped %+v", len(first.Applied), first.Skipped)
	}
	ftsExec(t, db, `INSERT INTO articles (id, title, body) VALUES (1, 'red fox', 'quick')`)
	if got := ftsSearch(t, db, `"fox"`); len(got) != 1 {
		t.Fatalf("the sync triggers were not created by AutoMigrate: %v", got)
	}
	again, err := eng.AutoMigrate(ctx, want)
	if err != nil {
		t.Fatalf("second AutoMigrate: %v", err)
	}
	if len(again.Applied) != 0 || len(again.Skipped) != 0 {
		t.Fatalf("second AutoMigrate planned changes: applied %+v, skipped %+v", again.Applied, again.Skipped)
	}
}

// Catches: AutoMigrate applying a bare CREATE TRIGGER or CREATE VIRTUAL TABLE
// for a changed declaration (its drops are skipped, so the objects exist) and
// failing with "already exists" instead of reporting a skip.
func TestFullTextAutoMigrateChangedColumnsSkips(t *testing.T) {
	ctx := context.Background()
	db := testopen.SQLite(t)
	eng := newEngine(t, db, migratetest.EmptyFS(), "20260101000000")
	if _, err := eng.AutoMigrate(ctx, ftsDeclared()); err != nil {
		t.Fatal(err)
	}
	next := ftsDeclared()
	ix := &next.Tables[0].Indexes[0]
	ix.Parts = []schema.IndexPart{{Column: "title"}}
	ix.FullText.Columns = []string{"title"}
	rep, err := eng.AutoMigrate(ctx, next)
	if err != nil {
		t.Fatalf("a changed declaration must be skipped, not fail: %v", err)
	}
	if len(rep.Applied) != 0 || len(rep.Skipped) == 0 {
		t.Fatalf("applied %+v, skipped %+v; want a skip report only", rep.Applied, rep.Skipped)
	}
	for _, s := range rep.Skipped {
		if s.Reason == "" {
			t.Errorf("skip without a reason: %+v", s.Change)
		}
	}
	ftsExec(t, db, `INSERT INTO articles (id, title, body) VALUES (1, 'red fox', 'quick')`)
	if got := ftsSearch(t, db, `"quick"`); len(got) != 1 {
		t.Fatalf("the existing FTS5 table and triggers were disturbed: %v", got)
	}
}

// Catches: a user trigger that merely shares the name of a sync trigger being
// treated as the generated one and applied by AutoMigrate.
func TestFullTextAutoMigrateSkipsUserTriggerNamedLikeSync(t *testing.T) {
	ctx := context.Background()
	db := testopen.SQLite(t)
	eng := newEngine(t, db, migratetest.EmptyFS(), "20260101000000")
	want := ftsDeclared()
	want.Triggers = []schema.Trigger{{
		Name: "articles_fts_ai", Table: "articles",
		Definition: `CREATE TRIGGER "articles_fts_ai" AFTER INSERT ON "articles" BEGIN SELECT 1; END`,
	}}
	rep, err := eng.AutoMigrate(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	skipped := false
	for _, s := range rep.Skipped {
		if s.Change.Kind == migrate.CreateTrigger && s.Change.Name == "articles_fts_ai" {
			skipped = true
		}
	}
	if !skipped {
		t.Fatalf("the user trigger was not skipped: applied %+v, skipped %+v", rep.Applied, rep.Skipped)
	}
	rows, err := db.Driver().Query(ctx, `SELECT count(*) FROM sqlite_schema WHERE type = 'trigger' AND name = 'articles_fts_ai'`, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var n int
	if !rows.Next() || rows.Scan(&n) != nil || n != 0 {
		t.Fatalf("the user trigger was created: %d", n)
	}
}

// Catches: a baseline of a database that holds a full-text table refused as a
// mismatch, and drift after the baseline.
func TestFullTextBaselineClean(t *testing.T) {
	ctx := context.Background()
	db := testopen.SQLite(t)
	want := ftsDeclared()
	auto := newEngine(t, db, migratetest.EmptyFS(), "20260101000000")
	if _, err := auto.AutoMigrate(ctx, want); err != nil {
		t.Fatal(err)
	}
	m, err := auto.Diff(ctx, want, "fts", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatalf("diff: %v %v", m, err)
	}
	eng := newEngine(t, db, migratetest.FS(m.Files), "20260102000000")
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
}
