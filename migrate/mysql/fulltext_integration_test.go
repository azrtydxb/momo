// SPDX-License-Identifier: Apache-2.0

//go:build integration

package mysql_test

import (
	"context"
	"errors"
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

func ftDeclared(parser string) *schema.Schema {
	return &schema.Schema{Dialect: "mysql", Tables: []schema.Table{{
		Name: "articles",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint"},
			{Name: "title", Type: "varchar(255)"},
			{Name: "body", Type: "text"},
		},
		PrimaryKey: []string{"id"},
		Indexes: []schema.Index{{Name: "articles_search", Method: "fulltext",
			Parts:    []schema.IndexPart{{Column: "title"}, {Column: "body"}},
			FullText: &schema.FullText{Columns: []string{"title", "body"}, Parser: parser}}},
	}}}
}

// scanOne runs q through the driver and scans its first row into dest.
func scanOne(ctx context.Context, db *momo.DB, q string, dest ...any) (err error) {
	rows, err := db.Driver().Query(ctx, q, nil)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := rows.Close(); err == nil {
			err = cerr
		}
	}()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return errors.New("the query returned no row")
	}
	return rows.Scan(dest...)
}

// Catches: the index missing on the server after apply, and the parser lost on
// MySQL. MariaDB runs without a parser (no ngram plugin).
func TestFullTextMySQLMigrationRoundTrip(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		parser := ""
		if db.Driver().Dialect().Flavor == dialect.FlavorMySQL {
			parser = "ngram"
		}
		dir := t.TempDir()
		eng := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(dir))
		applyExample(t, dir, eng, ftDeclared(parser), "fulltext")
		var typ string
		if err := scanOne(ctx, db, `SELECT INDEX_TYPE FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'articles' AND INDEX_NAME = 'articles_search' LIMIT 1`, &typ); err != nil || typ != "FULLTEXT" {
			t.Fatalf("index type = %q (%v), want FULLTEXT", typ, err)
		}
		if parser == "ngram" {
			var ddl, name string
			if err := scanOne(ctx, db, "SHOW CREATE TABLE articles", &name, &ddl); err != nil || !strings.Contains(ddl, "WITH PARSER `ngram`") {
				t.Fatalf("parser missing from SHOW CREATE TABLE: %v\n%s", err, ddl)
			}
		}
	})
}

// Catches: a declared parser sent to MariaDB, which has no ngram plugin, and
// an error that is not ErrUnsupported.
func TestFullTextMariaDBParserRefused(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		if db.Driver().Dialect().Flavor != dialect.FlavorMariaDB {
			t.Skip("MariaDB only")
		}
		eng := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(t.TempDir()))
		m, err := eng.Diff(context.Background(), ftDeclared("ngram"), "fulltext", migrate.DiffOptions{})
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("err = %v, want ErrUnsupported", err)
		}
		if m != nil {
			t.Fatalf("a migration was produced: %+v", m)
		}
	})
}

// Catches: a full-text index declared on a table the database holds as MyISAM
// reaching the server, and an error that is not ErrValidation naming the
// table. AutoMigrate is the engine path that sees the live database; Diff
// reads no database.
func TestFullTextRefusedOnNonInnoDBTable(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		mustExec(t, db, "CREATE TABLE ft_legacy (id bigint NOT NULL, title varchar(255) NOT NULL, body text NOT NULL, PRIMARY KEY (id)) ENGINE=MyISAM")
		want := ftDeclared("")
		want.Tables[0].Name = "ft_legacy"
		eng := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(t.TempDir()))
		_, err := eng.AutoMigrate(ctx, want)
		if !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), "ft_legacy") {
			t.Fatalf("err = %v, want ErrValidation naming ft_legacy", err)
		}
		var typ string
		if err := scanOne(ctx, db, `SELECT INDEX_TYPE FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ft_legacy' AND INDEX_NAME = 'articles_search' LIMIT 1`, &typ); err == nil {
			t.Fatalf("the index was created anyway (%s)", typ)
		}
	})
}

// Catches: a live MyISAM table reported as drift against a snapshot that never
// records a storage engine.
func TestDriftIgnoresNonInnoDBEngine(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		want := ftDeclared("")
		want.Tables[0].Indexes = nil
		dir := t.TempDir()
		eng := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(dir))
		applyExample(t, dir, eng, want, "plain")
		mustExec(t, db, "ALTER TABLE articles ENGINE=MyISAM")
		rep, err := eng.Drift(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !rep.Clean() {
			t.Fatalf("a non-InnoDB table is reported as drift: %+v", rep.Changed)
		}
	})
}
