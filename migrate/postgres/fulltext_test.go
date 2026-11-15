// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

func articlesWithFullText(ft schema.FullText) *schema.Schema {
	return &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{
		Name: "articles",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint", Identity: "by_default"},
			{Name: "title", Type: "text", Nullable: true},
			{Name: "body", Type: "text", Nullable: true},
		},
		PrimaryKey: []string{"id"},
		Indexes: []schema.Index{{Name: "articles_search", Method: "fulltext",
			Parts: []schema.IndexPart{{Column: "title"}, {Column: "body"}}, FullText: &ft}},
	}}}
}

// Catches: a NULL source column making the whole vector NULL (no coalesce), the
// configuration not pinned as a literal, and the index not being GIN on the
// generated column.
func TestFullTextPostgresRender(t *testing.T) {
	var d Dialect
	s, err := d.ExpandSchema(articlesWithFullText(schema.FullText{
		Columns: []string{"title", "body"}, Config: "english", Weights: []string{"A", "B"}}))
	if err != nil {
		t.Fatal(err)
	}
	tbl := s.Tables[0]
	col, ok := tbl.Column("articles_search_tsv")
	if !ok || col.Type != "tsvector" {
		t.Fatalf("generated column missing: %+v", tbl.Columns)
	}
	want := `setweight(to_tsvector('english', coalesce("title", '')), 'A') || setweight(to_tsvector('english', coalesce("body", '')), 'B')`
	if col.Generated != want {
		t.Fatalf("generation expression\n got %s\nwant %s", col.Generated, want)
	}
	again, err := d.ExpandSchema(s)
	if err != nil || !schema.Equal(s, again) {
		t.Fatalf("expansion is not idempotent: %v", err)
	}
	changes, _, err := migrate.Diff(&schema.Schema{Dialect: "postgres"}, s, migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := d.Render(changes)
	if err != nil {
		t.Fatal(err)
	}
	var sql []string
	for _, st := range stmts {
		sql = append(sql, st.SQL)
	}
	all := strings.Join(sql, ";\n")
	for _, frag := range []string{
		`"articles_search_tsv" tsvector GENERATED ALWAYS AS (` + want + `) STORED`,
		`CREATE INDEX "articles_search" ON "articles" USING gin ("articles_search_tsv")`,
	} {
		if !strings.Contains(all, frag) {
			t.Errorf("rendered SQL lacks %q:\n%s", frag, all)
		}
	}
}

// Catches: an empty configuration or a hostile one reaching the generated
// expression.
func TestFullTextPostgresValidation(t *testing.T) {
	var d Dialect
	for _, ft := range []schema.FullText{
		{Columns: []string{"title"}},
		{Columns: []string{"title"}, Config: "english'); DROP TABLE articles;--"},
	} {
		if _, err := d.ExpandSchema(articlesWithFullText(ft)); err == nil {
			t.Errorf("accepted %+v", ft)
		}
	}
}

// Catches: a source column that is not on the table producing SQL that fails in
// the database, a generated column that silently replaces a user column of the
// same name, and a generated column name past PostgreSQL's identifier limit.
func TestFullTextPostgresExpansionRefusals(t *testing.T) {
	var d Dialect
	ft := schema.FullText{Columns: []string{"title", "body"}, Config: "english"}

	missing := articlesWithFullText(schema.FullText{Columns: []string{"title", "nope"}, Config: "english"})
	collide := articlesWithFullText(ft)
	collide.Tables[0].Columns = append(collide.Tables[0].Columns, schema.Column{Name: "articles_search_tsv", Type: "tsvector", Nullable: true})
	long := articlesWithFullText(ft)
	long.Tables[0].Indexes[0].Name = strings.Repeat("x", 60)

	for name, s := range map[string]*schema.Schema{"missing column": missing, "name collision": collide, "name too long": long} {
		_, err := d.ExpandSchema(s)
		if err == nil || !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), "articles") {
			t.Errorf("%s: err = %v, want ErrValidation naming the table", name, err)
		}
	}
}

// Catches: a CONCURRENTLY full-text index, which the migration splitter would
// move away from the generated-column recreation it depends on, being accepted.
func TestFullTextPostgresRefusesConcurrently(t *testing.T) {
	var d Dialect
	s := articlesWithFullText(schema.FullText{Columns: []string{"title", "body"}, Config: "english"})
	s.Tables[0].Indexes[0].Concurrently = true
	_, err := d.ExpandSchema(s)
	if err == nil || !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), "CONCURRENTLY") {
		t.Fatalf("err = %v, want ErrValidation naming CONCURRENTLY", err)
	}
}

// Catches: the drop-and-add of a generated column leaking to columns that no
// full-text index owns, where a changed expression must stay refused.
func TestGeneratedColumnChangeStillRefused(t *testing.T) {
	var d Dialect
	table := func(expr string) *schema.Schema {
		return &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{
			Name: "orders",
			Columns: []schema.Column{
				{Name: "id", Type: "bigint", Identity: "by_default"},
				{Name: "price", Type: "integer"},
				{Name: "total", Type: "integer", Generated: expr},
			},
			PrimaryKey: []string{"id"},
		}}}
	}
	changes, _, err := migrate.Diff(table("price * 2"), table("price * 3"), migrate.DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.Render(changes)
	if err == nil || !strings.Contains(err.Error(), "cannot be altered in place") {
		t.Fatalf("err = %v, want the in-place refusal", err)
	}
}

// Catches: an expression declaration that adds a generated column, and one that
// leaves the index without its expression.
func TestFullTextPostgresExpression(t *testing.T) {
	var d Dialect
	s, err := d.ExpandSchema(articlesWithFullText(schema.FullText{
		Columns: []string{"title", "body"}, Config: "english", Expression: true}))
	if err != nil {
		t.Fatal(err)
	}
	tbl := s.Tables[0]
	if len(tbl.Columns) != 3 {
		t.Fatalf("an expression index must add no column: %+v", tbl.Columns)
	}
	want := `to_tsvector('english', coalesce("title", '') || ' ' || coalesce("body", ''))`
	if got := tbl.Indexes[0].Parts; len(got) != 1 || got[0].Expr != want {
		t.Fatalf("index parts = %+v, want expression %s", got, want)
	}
}

// Catches: a config change that is refused as "write a custom migration", and
// one that alters the column without dropping the index that depends on it.
func TestFullTextConfigChangeDiff(t *testing.T) {
	var d Dialect
	from, _ := d.ExpandSchema(articlesWithFullText(schema.FullText{Columns: []string{"title", "body"}, Config: "english"}))
	to, _ := d.ExpandSchema(articlesWithFullText(schema.FullText{Columns: []string{"title", "body"}, Config: "simple"}))
	changes, _, err := migrate.Diff(from, to, migrate.DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := migrate.OrderErr(changes)
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := d.Render(ordered)
	if err != nil {
		t.Fatal(err)
	}
	var sql []string
	for _, st := range stmts {
		sql = append(sql, st.SQL)
	}
	want := []string{
		`DROP INDEX "articles_search"`,
		`ALTER TABLE "articles" DROP COLUMN "articles_search_tsv"`,
		`ALTER TABLE "articles" ADD COLUMN "articles_search_tsv" tsvector GENERATED ALWAYS AS (to_tsvector('simple', coalesce("title", '')) || to_tsvector('simple', coalesce("body", ''))) STORED`,
		`CREATE INDEX "articles_search" ON "articles" USING gin ("articles_search_tsv")`,
	}
	if strings.Join(sql, "\n") != strings.Join(want, "\n") {
		t.Fatalf("statements:\n%s\nwant:\n%s", strings.Join(sql, "\n"), strings.Join(want, "\n"))
	}
}

// Catches: the lint rule drifting from what the renderer emits, for a new
// declaration and for a configuration change that re-adds the column.
func TestFullTextRenderedSQLIsLinted(t *testing.T) {
	var d Dialect
	plain := articlesWithFullText(schema.FullText{Columns: []string{"title", "body"}})
	plain.Tables[0].Indexes = nil
	english, _ := d.ExpandSchema(articlesWithFullText(schema.FullText{Columns: []string{"title", "body"}, Config: "english"}))
	simple, _ := d.ExpandSchema(articlesWithFullText(schema.FullText{Columns: []string{"title", "body"}, Config: "simple"}))
	for name, pair := range map[string][2]*schema.Schema{"new declaration": {plain, english}, "config change": {english, simple}} {
		changes, _, err := migrate.Diff(pair[0], pair[1], migrate.DiffOptions{AllowDestructive: true})
		if err != nil {
			t.Fatal(err)
		}
		ordered, err := migrate.OrderErr(changes)
		if err != nil {
			t.Fatal(err)
		}
		stmts, err := d.Render(ordered)
		if err != nil {
			t.Fatal(err)
		}
		found := 0
		for _, f := range migrate.LintSQL("20260101000000", stmts, map[string]bool{"articles": true}, "postgres") {
			if f.Rule == migrate.RuleFullTextRewrite && f.Severity == migrate.SeverityWarning {
				found++
			}
		}
		if found != 1 {
			t.Errorf("%s: %d full-text rewrite warnings for %+v, want 1", name, found, stmts)
		}
	}
}
