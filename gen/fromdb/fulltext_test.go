// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

func fullTextSource(t *testing.T, s *schema.Schema, d dialect.Name) (string, *Result) {
	t.Helper()
	res, err := Generate(s, Options{Package: "models", Dialect: d})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Files {
		if f.Table == "articles" {
			return string(f.Content), res
		}
	}
	t.Fatalf("no model for articles: %+v", res.Files)
	return "", nil
}

// Catches: the declaration not emitted by --from-db, the PostgreSQL generated
// column emitted as a model field next to the declaration, and a snapshot that
// still holds the generated column (it would not match the declaration).
func TestGenerateFromDBFullText(t *testing.T) {
	s := &schema.Schema{Dialect: dialect.Postgres, Tables: []schema.Table{{
		Name: "articles",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint", Identity: "by_default"},
			{Name: "title", Type: "text"}, {Name: "body", Type: "text"},
			{Name: "articles_search_tsv", Type: "tsvector", Nullable: true, Generated: "x"},
		},
		PrimaryKey: []string{"id"},
		Indexes: []schema.Index{{Name: "articles_search", Method: "fulltext",
			Parts:    []schema.IndexPart{{Column: "articles_search_tsv"}},
			FullText: &schema.FullText{Columns: []string{"title", "body"}, Config: "simple", Weights: []string{"A", "B"}}}},
	}}}
	src, res := fullTextSource(t, s, dialect.Postgres)
	if !strings.Contains(src, `c.FullText("articles_search", &m.Title, &m.Body).Config("simple").Weights("A", "B")`) {
		t.Fatalf("no c.FullText call:\n%s", src)
	}
	if strings.Contains(src, "ArticlesSearchTsv") || strings.Contains(src, "tsvector") {
		t.Fatalf("the generated column was emitted as a field:\n%s", src)
	}
	snap, _ := res.Snapshot.Table("articles")
	if _, ok := snap.Column("articles_search_tsv"); ok {
		t.Fatalf("the snapshot keeps the generated column: %+v", snap.Columns)
	}
	if ix, ok := snap.FullTextIndex(); !ok || len(ix.Parts) != 2 || ix.Parts[0].Column != "title" {
		t.Fatalf("snapshot index = %+v %v, want the source columns as key parts", ix, ok)
	}
}

// Catches: the expression form emitted without .Expression(), which would
// declare a generated column instead.
func TestGenerateFromDBFullTextExpression(t *testing.T) {
	s := &schema.Schema{Dialect: dialect.Postgres, Tables: []schema.Table{{
		Name: "articles",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint", Identity: "by_default"},
			{Name: "title", Type: "text"}, {Name: "body", Type: "text"},
		},
		PrimaryKey: []string{"id"},
		Indexes: []schema.Index{{Name: "articles_search", Method: "fulltext",
			Parts:    []schema.IndexPart{{Expr: "to_tsvector('english', coalesce(\"title\", ''))"}},
			FullText: &schema.FullText{Columns: []string{"title", "body"}, Config: "english", Expression: true}}},
	}}}
	src, _ := fullTextSource(t, s, dialect.Postgres)
	if !strings.Contains(src, `c.FullText("articles_search", &m.Title, &m.Body).Config("english").Expression()`) {
		t.Fatalf("no expression declaration:\n%s", src)
	}
}

// Catches: the plain declaration written as a call where the tag says it, the
// tag written for a declaration it cannot carry, and the SQLite sync triggers
// emitted as models.
func TestGenerateFromDBFullTextSearchTag(t *testing.T) {
	my := &schema.Schema{Dialect: dialect.MySQL, Tables: []schema.Table{{
		Name: "articles",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint"},
			{Name: "title", Type: "varchar(255)"}, {Name: "body", Type: "text"},
		},
		PrimaryKey: []string{"id"},
		Indexes: []schema.Index{{Name: "articles_search", Method: "fulltext",
			Parts:    []schema.IndexPart{{Column: "title"}, {Column: "body"}},
			FullText: &schema.FullText{Columns: []string{"title", "body"}}}},
	}}}
	src, _ := fullTextSource(t, my, dialect.MySQL)
	if strings.Count(src, "search`") != 2 || strings.Contains(src, "c.FullText") {
		t.Fatalf("want the search tag on both fields:\n%s", src)
	}

	my.Tables[0].Indexes[0].FullText.Parser = "ngram"
	src, _ = fullTextSource(t, my, dialect.MySQL)
	if strings.Contains(src, "search`") || !strings.Contains(src, `.Parser("ngram")`) {
		t.Fatalf("a parser needs the call form:\n%s", src)
	}

	lite := &schema.Schema{Dialect: dialect.SQLite, Tables: []schema.Table{{
		Name: "articles",
		Columns: []schema.Column{
			{Name: "id", Type: "INTEGER", Identity: "by_default"},
			{Name: "title", Type: "TEXT", Nullable: true}, {Name: "body", Type: "TEXT", Nullable: true},
		},
		PrimaryKey: []string{"id"},
		Indexes: []schema.Index{{Name: "articles_fts", Method: "fulltext",
			Parts:    []schema.IndexPart{{Column: "title"}, {Column: "body"}},
			FullText: &schema.FullText{Columns: []string{"title", "body"}, Tokenizer: "porter unicode61"}}},
	}}, Triggers: []schema.Trigger{
		{Name: "articles_fts_ai", Table: "articles", Definition: "CREATE TRIGGER articles_fts_ai ..."},
		{Name: "articles_fts_ad", Table: "articles", Definition: "CREATE TRIGGER articles_fts_ad ..."},
		{Name: "articles_fts_au", Table: "articles", Definition: "CREATE TRIGGER articles_fts_au ..."},
	}}
	src, res := fullTextSource(t, lite, dialect.SQLite)
	if !strings.Contains(src, `c.FullText("articles_search", &m.Title, &m.Body).Tokenizer("porter unicode61")`) || strings.Contains(src, "c.Trigger") {
		t.Fatalf("SQLite declaration:\n%s", src)
	}
	if len(res.Snapshot.Triggers) != 0 {
		t.Fatalf("the snapshot keeps the sync triggers: %+v", res.Snapshot.Triggers)
	}
}
