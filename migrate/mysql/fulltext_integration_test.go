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
		row := db.QueryRow(ctx, `SELECT INDEX_TYPE FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'articles' AND INDEX_NAME = 'articles_search' LIMIT 1`)
		if err := row.Scan(&typ); err != nil || typ != "FULLTEXT" {
			t.Fatalf("index type = %q (%v), want FULLTEXT", typ, err)
		}
		if parser == "ngram" {
			var ddl, name string
			if err := db.QueryRow(ctx, "SHOW CREATE TABLE articles").Scan(&name, &ddl); err != nil || !strings.Contains(ddl, "WITH PARSER `ngram`") {
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
