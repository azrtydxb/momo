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
