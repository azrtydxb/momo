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

// concurrentSQL renders each change of a plan on its own, the way the
// no-transaction split does, and returns the statements.
func concurrentSQL(t *testing.T, plan []migrate.Change) []migrate.Statement {
	t.Helper()
	var d Dialect
	var out []migrate.Statement
	for _, c := range plan {
		stmts, err := d.Render([]migrate.Change{c})
		if err != nil {
			t.Fatalf("render %s %s on its own: %v", c.Kind, c.Name, err)
		}
		out = append(out, stmts...)
	}
	return out
}

func concurrentArticles(config string, weights []string) *schema.Schema {
	s := articlesWithFullText(schema.FullText{Columns: []string{"title", "body"}, Config: config, Weights: weights})
	s.Tables[0].Indexes[0].Concurrently = true
	return s
}

// Catches: a CONCURRENTLY full-text declaration being refused, and one that
// expands without the generated column or loses the flag the index build needs.
func TestFullTextPostgresAcceptsConcurrently(t *testing.T) {
	var d Dialect
	s, err := d.ExpandSchema(concurrentArticles("english", nil))
	if err != nil {
		t.Fatalf("a CONCURRENTLY full-text index must expand: %v", err)
	}
	tbl := s.Tables[0]
	if _, ok := tbl.Column("articles_search_tsv"); !ok || !tbl.Indexes[0].Concurrently {
		t.Fatalf("want the generated column and a concurrent index: %+v", tbl)
	}
	changes, _, err := migrate.Diff(&schema.Schema{Dialect: "postgres"}, s, migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var create *migrate.Statement
	for _, st := range concurrentSQL(t, changes) {
		if strings.HasPrefix(st.SQL, "CREATE INDEX") {
			create = &st
		}
	}
	if create == nil || !create.NoTx || create.SQL != `CREATE INDEX CONCURRENTLY "articles_search" ON "articles" USING gin ("articles_search_tsv")` {
		t.Fatalf("index statement = %+v", create)
	}
}

// Catches: the generated column being recreated only when the index change
// shares the plan (so the no-transaction split, which renders change by change,
// hits the in-place refusal), and the old index being dropped CONCURRENTLY,
// which would split the drop away from the transaction that drops its column.
func TestFullTextPostgresConcurrentConfigChange(t *testing.T) {
	var d Dialect
	from, _ := d.ExpandSchema(concurrentArticles("english", nil))
	to, _ := d.ExpandSchema(concurrentArticles("simple", []string{"A", "B"}))
	changes, _, err := migrate.Diff(from, to, migrate.DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := migrate.OrderErr(changes)
	if err != nil {
		t.Fatal(err)
	}
	var sql []string
	for _, st := range concurrentSQL(t, ordered) {
		tag := "tx: "
		if st.NoTx {
			tag = "notx: "
		}
		sql = append(sql, tag+st.SQL)
	}
	want := []string{
		`tx: DROP INDEX "articles_search"`,
		`tx: ALTER TABLE "articles" DROP COLUMN "articles_search_tsv"`,
		`tx: ALTER TABLE "articles" ADD COLUMN "articles_search_tsv" tsvector GENERATED ALWAYS AS (setweight(to_tsvector('simple', coalesce("title", '')), 'A') || setweight(to_tsvector('simple', coalesce("body", '')), 'B')) STORED`,
		`notx: CREATE INDEX CONCURRENTLY "articles_search" ON "articles" USING gin ("articles_search_tsv")`,
	}
	if strings.Join(sql, "\n") != strings.Join(want, "\n") {
		t.Fatalf("statements:\n%s\nwant:\n%s", strings.Join(sql, "\n"), strings.Join(want, "\n"))
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
