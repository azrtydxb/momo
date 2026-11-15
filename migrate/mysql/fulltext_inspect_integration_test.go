// SPDX-License-Identifier: Apache-2.0

//go:build integration

package mysql_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
	"github.com/azrtydxb/momo/schema"
)

// ftParser is the parser a server can run: ngram on MySQL, none on MariaDB.
func ftParser(db *momo.DB) string {
	if db.Driver().Dialect().Flavor == dialect.FlavorMySQL {
		return "ngram"
	}
	return ""
}

// Catches: an inspection that misses the full-text structure (the index read as
// a plain index), a parser lost on the way back from SHOW CREATE TABLE, and an
// inspected schema that differs from the expanded declaration.
func TestFullTextInspectRoundTrip(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		parser := ftParser(db)
		want := ftDeclared(parser)
		dir := t.TempDir()
		eng := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(dir))
		applyExample(t, dir, eng, want, "fulltext")

		got, err := eng.Inspect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tbl, ok := got.Table("articles")
		if !ok {
			t.Fatalf("articles not inspected: %+v", got.Tables)
		}
		ix, ok := tbl.FullTextIndex()
		if !ok || ix.FullText.Parser != parser || len(ix.FullText.Columns) != 2 || ix.FullText.Columns[0] != "title" {
			t.Fatalf("inspection misses the full-text structure: %+v", tbl.Indexes)
		}
		expanded, err := mmysql.New().ExpandSchema(want)
		if err != nil {
			t.Fatal(err)
		}
		if !schema.Equal(expanded, got) {
			t.Fatalf("inspection differs from the expanded declaration:\n%s", migratetest.Diff(got, expanded))
		}
		rep, err := eng.Drift(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !rep.Clean() {
			t.Fatalf("drift on a freshly applied full-text index: %+v", rep)
		}
	})
}

// Catches: InnoDB's hidden FTS_DOC_ID column or FTS_DOC_ID_INDEX showing up in
// an inspection, which would make every FULLTEXT table differ from its model.
func TestFullTextInspectHidesDocID(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		dir := t.TempDir()
		eng := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(dir))
		applyExample(t, dir, eng, ftDeclared(ftParser(db)), "fulltext")
		got, err := eng.Inspect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tbl, ok := got.Table("articles")
		if !ok {
			t.Fatalf("articles not inspected: %+v", got.Tables)
		}
		for _, c := range tbl.Columns {
			if strings.HasPrefix(strings.ToUpper(c.Name), "FTS_DOC_ID") {
				t.Errorf("hidden column inspected: %+v", c)
			}
		}
		for _, ix := range tbl.Indexes {
			if strings.HasPrefix(strings.ToUpper(ix.Name), "FTS_DOC_ID") {
				t.Errorf("hidden index inspected: %+v", ix)
			}
		}
		for _, u := range tbl.Uniques {
			if strings.HasPrefix(strings.ToUpper(u.Name), "FTS_DOC_ID") {
				t.Errorf("hidden unique key inspected: %+v", u)
			}
		}
	})
}

// Catches: a second AutoMigrate that plans or runs anything on a model with a
// FULLTEXT declaration, on MySQL 8.4, 9.7 and MariaDB.
func TestFullTextAutoMigrateTwice(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		want := ftDeclared(ftParser(db))
		eng := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(t.TempDir()))
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

// Catches: a baseline of a database that holds a FULLTEXT index refused as a
// mismatch, and drift after the baseline.
func TestFullTextBaselineClean(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		want := ftDeclared(ftParser(db))
		auto := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(t.TempDir()))
		if _, err := auto.AutoMigrate(ctx, want); err != nil {
			t.Fatal(err)
		}
		m, err := auto.Diff(ctx, want, "fulltext", migrate.DiffOptions{})
		if err != nil || m == nil {
			t.Fatalf("diff: %v %v", m, err)
		}
		dir := t.TempDir()
		writeMigration(t, dir, m)
		eng := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(dir))
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
