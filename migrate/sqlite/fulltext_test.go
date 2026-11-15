// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

func ftsArticles(pk []string, pkType string) *schema.Schema {
	return &schema.Schema{Dialect: "sqlite", Tables: []schema.Table{{
		Name: "articles",
		Columns: []schema.Column{
			{Name: "id", Type: pkType},
			{Name: "title", Type: "TEXT", Nullable: true},
			{Name: "body", Type: "TEXT", Nullable: true},
		},
		PrimaryKey: pk,
		Indexes: []schema.Index{{Name: "articles_search", Method: "fulltext",
			Parts:    []schema.IndexPart{{Column: "title"}, {Column: "body"}},
			FullText: &schema.FullText{Columns: []string{"title", "body"}, Tokenizer: "porter unicode61"}}},
	}}}
}

// renderPlan diffs from to to and renders the ordered plan.
func renderPlan(t *testing.T, from, to *schema.Schema, opts migrate.DiffOptions) string {
	t.Helper()
	var d Dialect
	changes, _, err := migrate.Diff(from, to, opts)
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := migrate.OrderErr(changes)
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := d.RenderSchema(from, to, ordered)
	if err != nil {
		t.Fatal(err)
	}
	var sql []string
	for _, s := range stmts {
		sql = append(sql, s.SQL)
	}
	return strings.Join(sql, "\n")
}

// Catches: the update trigger issuing insert before delete (index corruption),
// a missing rebuild after creation, and the tokenizer dropped from the DDL.
func TestFullTextSQLiteRender(t *testing.T) {
	var d Dialect
	to, err := d.ExpandSchema(ftsArticles([]string{"id"}, "INTEGER"))
	if err != nil {
		t.Fatal(err)
	}
	all := renderPlan(t, &schema.Schema{Dialect: "sqlite"}, to, migrate.DiffOptions{})
	vt := strings.Index(all, `CREATE VIRTUAL TABLE "articles_fts" USING fts5("title", "body", content='articles', content_rowid='id', tokenize='porter unicode61')`)
	rb := strings.Index(all, `INSERT INTO "articles_fts"("articles_fts") VALUES('rebuild')`)
	tr := strings.Index(all, `CREATE TRIGGER "articles_fts_ai"`)
	if vt < 0 || rb < vt || tr < 0 {
		t.Fatalf("create order wrong (vt=%d rebuild=%d trigger=%d):\n%s", vt, rb, tr, all)
	}
	if tr < rb {
		t.Fatalf("a trigger is created before the table is filled:\n%s", all)
	}
	_, au, found := strings.Cut(all, `CREATE TRIGGER "articles_fts_au"`)
	if !found || strings.Index(au, `VALUES ('delete'`) > strings.Index(au, `VALUES (new."id"`) {
		t.Fatalf("update trigger inserts before it deletes:\n%s", au)
	}
}

// Catches: a second expansion changing the schema (the diff would never
// settle), and the options SQLite ignores staying in the description.
func TestFullTextSQLiteExpandIdempotent(t *testing.T) {
	var d Dialect
	in := ftsArticles([]string{"id"}, "INTEGER")
	in.Tables[0].Indexes[0].FullText.Config = "english"
	in.Tables[0].Indexes[0].FullText.Weights = []string{"A", "B"}
	once, err := d.ExpandSchema(in)
	if err != nil {
		t.Fatal(err)
	}
	twice, err := d.ExpandSchema(once)
	if err != nil || !schema.Equal(once, twice) {
		t.Fatalf("expansion is not idempotent: %v", err)
	}
	ix := once.Tables[0].Indexes[0]
	if ix.Name != "articles_fts" || ix.FullText.Config != "" || ix.FullText.Weights != nil {
		t.Fatalf("expanded index = %+v", ix)
	}
	if len(once.Triggers) != 3 {
		t.Fatalf("triggers = %d, want 3", len(once.Triggers))
	}
	if in.Tables[0].Indexes[0].Name != "articles_search" || len(in.Triggers) != 0 {
		t.Fatal("ExpandSchema modified its argument")
	}
}

// Catches: a composite or non-integer key reaching SQL, and a diagnostic that
// does not name the table.
func TestFullTextSQLiteNeedsIntegerKey(t *testing.T) {
	var d Dialect
	for name, s := range map[string]*schema.Schema{
		"composite": ftsArticles([]string{"id", "title"}, "INTEGER"),
		"text key":  ftsArticles([]string{"id"}, "TEXT"),
	} {
		_, err := d.ExpandSchema(s)
		if err == nil || !strings.Contains(err.Error(), "articles") || !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation naming articles", name, err)
		}
	}
}

// Catches: a declared column the table lacks reaching CREATE VIRTUAL TABLE, and
// a diagnostic that names neither the table nor the column.
func TestFullTextSQLiteUnknownColumn(t *testing.T) {
	var d Dialect
	s := ftsArticles([]string{"id"}, "INTEGER")
	s.Tables[0].Indexes[0].FullText.Columns = []string{"title", "missing"}
	_, err := d.ExpandSchema(s)
	if err == nil || !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), "articles") || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("err = %v, want ErrValidation naming articles and missing", err)
	}
}

// Catches: a table or key name with a quote reaching the content= option.
func TestCreateFTS5SQLRefusesHostileNames(t *testing.T) {
	ix := schema.Index{Name: "x", Method: "fulltext", FullText: &schema.FullText{Columns: []string{"title"}}}
	for _, c := range [][2]string{{"art'icles", "id"}, {"articles", "i'd"}, {"articles", "id; DROP TABLE x"}} {
		if _, err := createFTS5SQL(c[0], ix, c[1]); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("createFTS5SQL(%q, %q) err = %v, want ErrValidation", c[0], c[1], err)
		}
	}
}

// Catches: the virtual table dropped while its triggers still point at it, and
// a shadow object left behind after the declaration is removed.
func TestFullTextSQLiteDrop(t *testing.T) {
	var d Dialect
	from, err := d.ExpandSchema(ftsArticles([]string{"id"}, "INTEGER"))
	if err != nil {
		t.Fatal(err)
	}
	to := ftsArticles([]string{"id"}, "INTEGER")
	to.Tables[0].Indexes = nil
	all := renderPlan(t, from, to, migrate.DiffOptions{AllowDestructive: true})
	lastTrigger := strings.LastIndex(all, `DROP TRIGGER IF EXISTS "articles_fts_`)
	dropTable := strings.Index(all, `DROP TABLE "articles_fts"`)
	if lastTrigger < 0 || dropTable < lastTrigger {
		t.Fatalf("drop table before triggers (or a statement is missing):\n%s", all)
	}
}

// Catches: the virtual table left behind when its content table is dropped.
func TestFullTextSQLiteDropContentTable(t *testing.T) {
	var d Dialect
	from, err := d.ExpandSchema(ftsArticles([]string{"id"}, "INTEGER"))
	if err != nil {
		t.Fatal(err)
	}
	all := renderPlan(t, from, &schema.Schema{Dialect: "sqlite"}, migrate.DiffOptions{AllowDestructive: true})
	if !strings.Contains(all, `DROP TABLE IF EXISTS "articles_fts"`) || !strings.Contains(all, `DROP TABLE "articles"`) {
		t.Fatalf("both tables must be dropped:\n%s", all)
	}
}
