// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"testing"

	"github.com/azrtydxb/momo/schema"
)

func TestHasTableText(t *testing.T) {
	plain := &schema.Schema{Tables: []schema.Table{{Name: "t", Columns: []schema.Column{{Name: "id", Type: "bigint"}}}}}
	if hasTableText(plain) {
		t.Fatal("a table without checks, defaults or expression indexes has no table text")
	}
	cases := map[string]func(*schema.Table){
		"check":     func(tb *schema.Table) { tb.Checks = []schema.Check{{Name: "c", Expr: "id > 0"}} },
		"default":   func(tb *schema.Table) { tb.Columns[0].Default = "0" },
		"generated": func(tb *schema.Table) { tb.Columns[0].Generated = "id + 1" },
		"where":     func(tb *schema.Table) { tb.Indexes = []schema.Index{{Name: "i", Where: "id > 0"}} },
		"expr": func(tb *schema.Table) {
			tb.Indexes = []schema.Index{{Name: "i", Parts: []schema.IndexPart{{Expr: "lower(x)"}}}}
		},
	}
	for name, mut := range cases {
		s := &schema.Schema{Tables: []schema.Table{{Name: "t", Columns: []schema.Column{{Name: "id", Type: "bigint"}}}}}
		mut(&s.Tables[0])
		if !hasTableText(s) {
			t.Errorf("%s must count as table text", name)
		}
	}
}

// rewriteTables must take the stored form of checks, defaults, generated
// expressions and index text, and leave undeclared text alone.
func TestRewriteTables(t *testing.T) {
	declared := &schema.Schema{Tables: []schema.Table{{
		Name: "t",
		Columns: []schema.Column{
			{Name: "a", Type: "text", Default: "'x'"},
			{Name: "b", Type: "text"},
		},
		Checks:  []schema.Check{{Name: "c", Expr: "length(title) > 0"}},
		Indexes: []schema.Index{{Name: "i", Where: "s IN ('a')", Parts: []schema.IndexPart{{Column: "b"}, {Expr: "lower(title)"}}}},
	}}}
	read := &schema.Schema{Tables: []schema.Table{{
		Name: "t",
		Columns: []schema.Column{
			{Name: "a", Type: "text", Default: "'x'::text"},
			{Name: "b", Type: "text", Default: "ignored"},
		},
		Checks:  []schema.Check{{Name: "c", Expr: "(length((title)::text) > 0)"}},
		Indexes: []schema.Index{{Name: "i", Where: "(s = ANY (ARRAY['a'::text]))", Parts: []schema.IndexPart{{Column: "b"}, {Expr: "lower((title)::text)"}}}},
	}}}
	if err := rewriteTables(declared, read); err != nil {
		t.Fatal(err)
	}
	tb := declared.Tables[0]
	if tb.Columns[0].Default != "'x'::text" || tb.Columns[1].Default != "" {
		t.Errorf("defaults: %+v", tb.Columns)
	}
	if tb.Checks[0].Expr != "(length((title)::text) > 0)" {
		t.Errorf("check: %q", tb.Checks[0].Expr)
	}
	ix := tb.Indexes[0]
	if ix.Where != "(s = ANY (ARRAY['a'::text]))" || ix.Parts[0].Expr != "" || ix.Parts[1].Expr != "lower((title)::text)" {
		t.Errorf("index: %+v", ix)
	}

	missing := &schema.Schema{Tables: []schema.Table{{Name: "t", Checks: []schema.Check{{Name: "gone", Expr: "true"}}}}}
	if err := rewriteTables(missing, read); err == nil {
		t.Fatal("a check missing from the scratch schema must be an error")
	}
}

// An expression key PostgreSQL stores as a plain column (v::vector(3) with an
// operator class on a vector(3) column) reads back as a column part. The
// whole read-back part replaces the declared one, or it would have neither a
// column nor an expression.
func TestRewriteTablesTakesTheWholeReadBackPart(t *testing.T) {
	declared := &schema.Schema{Tables: []schema.Table{{Name: "t", Indexes: []schema.Index{{
		Name: "i", Method: "hnsw",
		Parts: []schema.IndexPart{{Expr: "(v::vector(3))", OpClass: "vector_cosine_ops"}},
	}}}}}
	read := &schema.Schema{Tables: []schema.Table{{Name: "t", Indexes: []schema.Index{{
		Name: "i", Method: "hnsw",
		Parts: []schema.IndexPart{{Column: "v", OpClass: "vector_cosine_ops"}},
	}}}}}
	if err := rewriteTables(declared, read); err != nil {
		t.Fatal(err)
	}
	if got := declared.Tables[0].Indexes[0].Parts[0]; got != (schema.IndexPart{Column: "v", OpClass: "vector_cosine_ops"}) {
		t.Fatalf("part = %+v", got)
	}
}

func TestPlanRoundsSeparatesTablesOfOneName(t *testing.T) {
	text := func(ns string) schema.Table {
		return schema.Table{Schema: ns, Name: "t", Checks: []schema.Check{{Name: "c", Expr: "true"}}}
	}
	plain := schema.Table{Name: "plain", Columns: []schema.Column{{Name: "id", Type: "int"}}}
	s := &schema.Schema{Tables: []schema.Table{text(""), plain, text("b")}}
	rounds := planRounds(s, true)
	if len(rounds) != 2 || len(rounds[0].tables) != 1 || rounds[0].tables[0] != 0 || rounds[1].tables[0] != 2 {
		t.Fatalf("same-name tables must go to separate rounds, and tables without text to none: %+v", rounds)
	}
	s.Views = []schema.View{{Name: "v", Query: "select 1"}}
	all := planRounds(s, false)
	if len(all) != 2 || len(all[0].tables) != 2 || len(all[0].views) != 1 {
		t.Fatalf("with a view every table is created (the view may use any): %+v", all)
	}
}
