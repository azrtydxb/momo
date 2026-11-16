// SPDX-License-Identifier: Apache-2.0

package mysql

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

func ftTable(colType, engine string, unique bool, parser string) *schema.Schema {
	return &schema.Schema{Dialect: "mysql", Tables: []schema.Table{{
		Name: "articles", Engine: engine,
		Columns:    []schema.Column{{Name: "id", Type: "bigint"}, {Name: "title", Type: colType}, {Name: "body", Type: "text"}},
		PrimaryKey: []string{"id"},
		Indexes: []schema.Index{{Name: "articles_search", Method: "fulltext", Unique: unique,
			Parts:    []schema.IndexPart{{Column: "title"}, {Column: "body"}},
			FullText: &schema.FullText{Columns: []string{"title", "body"}, Parser: parser}}},
	}}}
}

// Catches: the parser dropped from the DDL and the column order changed.
func TestFullTextMySQLRender(t *testing.T) {
	d := Dialect{}
	to := ftTable("varchar(255)", "", false, "ngram")
	changes, _, err := migrate.Diff(&schema.Schema{Dialect: "mysql"}, to, migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := d.Render(changes)
	if err != nil {
		t.Fatal(err)
	}
	var sql []string
	for _, s := range stmts {
		sql = append(sql, s.SQL)
	}
	if !strings.Contains(strings.Join(sql, "\n"), "CREATE FULLTEXT INDEX `articles_search` ON `articles` (`title`, `body`) WITH PARSER ngram") {
		t.Fatalf("rendered:\n%s", strings.Join(sql, "\n"))
	}
}

// Catches: the lint rule drifting from the CREATE FULLTEXT INDEX statement the
// renderer emits, with and without the parser clause.
func TestFullTextRenderedSQLIsLinted(t *testing.T) {
	for _, parser := range []string{"", "ngram"} {
		plain := ftTable("varchar(255)", "", false, parser)
		plain.Tables[0].Indexes = nil
		changes, _, err := migrate.Diff(plain, ftTable("varchar(255)", "", false, parser), migrate.DiffOptions{})
		if err != nil {
			t.Fatal(err)
		}
		stmts, err := Dialect{}.Render(changes)
		if err != nil {
			t.Fatal(err)
		}
		found := 0
		for _, f := range migrate.LintSQL("20260101000000", stmts, map[string]bool{"articles": true}, "mysql") {
			if f.Rule == migrate.RuleFullTextFirstIndex && f.Severity == migrate.SeverityWarning {
				found++
			}
		}
		if found != 1 {
			t.Errorf("parser %q: %d first-index warnings for %+v, want 1", parser, found, stmts)
		}
	}
}

// Catches: a non-text column, a unique FULLTEXT and a non-InnoDB table
// reaching the server.
func TestFullTextMySQLRenderValidation(t *testing.T) {
	d := Dialect{}
	cases := map[string]*schema.Schema{
		"non-text column": ftTable("bigint", "", false, ""),
		"unique":          ftTable("text", "", true, ""),
		"myisam":          ftTable("text", "MyISAM", false, ""),
		"bad parser":      ftTable("text", "", false, "ngram; DROP TABLE x"),
	}
	for name, s := range cases {
		err := d.ValidateSchema(s)
		if err == nil || !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation", name, err)
		}
	}
	if err := d.ValidateSchema(ftTable("varchar(100)", "InnoDB", false, "ngram")); err != nil {
		t.Errorf("valid declaration refused: %v", err)
	}
}

// Catches: a full-text index declared Concurrently being refused on MySQL or
// rendering differently from a plain one. MySQL has no concurrent build, so the
// flag is ignored the way it is for an ordinary index.
func TestFullTextMySQLIgnoresConcurrently(t *testing.T) {
	d := Dialect{}
	render := func(s *schema.Schema) []migrate.Statement {
		changes, _, err := migrate.Diff(&schema.Schema{Dialect: "mysql"}, s, migrate.DiffOptions{})
		if err != nil {
			t.Fatal(err)
		}
		stmts, err := d.Render(changes)
		if err != nil {
			t.Fatal(err)
		}
		return stmts
	}
	conc := ftTable("varchar(255)", "", false, "ngram")
	conc.Tables[0].Indexes[0].Concurrently = true
	if err := d.ValidateSchema(conc); err != nil {
		t.Fatalf("a Concurrently full-text index must validate on MySQL: %v", err)
	}
	want, got := render(ftTable("varchar(255)", "", false, "ngram")), render(conc)
	if len(got) != len(want) {
		t.Fatalf("Concurrently changed the statement count: %+v vs %+v", got, want)
	}
	for i := range got {
		if got[i].SQL != want[i].SQL || got[i].NoTx {
			t.Fatalf("statement %d differs or is no-transaction: %+v vs %+v", i, got[i], want[i])
		}
	}
}
