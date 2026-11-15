// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"reflect"
	"testing"

	"github.com/azrtydxb/momo/schema"
)

// Catches: an unrelated generation expression claimed as a declaration, lost
// weights, and a nested concatenation (three or more terms) read as a single
// unparsable term.
func TestParseGenerationExpr(t *testing.T) {
	ft, ok := parseGenerationExpr(`(setweight(to_tsvector('english'::regconfig, COALESCE(title, ''::text)), 'A'::"char") || setweight(to_tsvector('english'::regconfig, COALESCE(body, ''::text)), 'B'::"char"))`)
	if !ok || ft.Config != "english" || !reflect.DeepEqual(ft.Columns, []string{"title", "body"}) || !reflect.DeepEqual(ft.Weights, []string{"A", "B"}) {
		t.Fatalf("got %+v %v", ft, ok)
	}
	if _, ok := parseGenerationExpr(`lower(title)`); ok {
		t.Fatal("an unrelated generation expression was claimed")
	}
	three, ok := parseGenerationExpr(`((to_tsvector('simple'::regconfig, COALESCE(a, ''::text)) || to_tsvector('simple'::regconfig, COALESCE("B", ''::text))) || to_tsvector('simple'::regconfig, COALESCE(c, ''::text)))`)
	if !ok || three.Config != "simple" || !reflect.DeepEqual(three.Columns, []string{"a", "B", "c"}) || len(three.Weights) != 0 {
		t.Fatalf("three terms: %+v %v", three, ok)
	}
	for _, bad := range []string{
		`(to_tsvector('english'::regconfig, COALESCE(a, ''::text)) || to_tsvector('simple'::regconfig, COALESCE(b, ''::text)))`,
		`(setweight(to_tsvector('english'::regconfig, COALESCE(a, ''::text)), 'A'::"char") || to_tsvector('english'::regconfig, COALESCE(b, ''::text)))`,
		`to_tsvector('english'::regconfig, a)`,
	} {
		if _, ok := parseGenerationExpr(bad); ok {
			t.Errorf("accepted %s", bad)
		}
	}
}

// Catches: the expression form read back with another column list, and a
// separator other than a single space accepted.
func TestParseIndexExpression(t *testing.T) {
	ft, ok := parseIndexExpression(`to_tsvector('english', coalesce(title, '') || ' ' || coalesce(body, ''))`)
	if !ok || !ft.Expression || ft.Config != "english" || !reflect.DeepEqual(ft.Columns, []string{"title", "body"}) {
		t.Fatalf("got %+v %v", ft, ok)
	}
	nested, ok := parseIndexExpression(`to_tsvector('english'::regconfig, ((COALESCE(title, ''::text) || ' '::text) || COALESCE(body, ''::text)))`)
	if !ok || !reflect.DeepEqual(nested.Columns, []string{"title", "body"}) {
		t.Fatalf("nested: %+v %v", nested, ok)
	}
	if _, ok := parseIndexExpression(`to_tsvector('english', coalesce(title, '') || '-' || coalesce(body, ''))`); ok {
		t.Fatal("a separator other than a space was accepted")
	}
	if _, ok := parseIndexExpression(`lower(title)`); ok {
		t.Fatal("an unrelated expression was claimed")
	}
}

// Catches: an inspected full-text table that does not equal the expanded
// declaration (so a second AutoMigrate or a drift check would see a change),
// and a hand-written generated column or GIN index being folded.
func TestFoldFullTextEqualsExpansion(t *testing.T) {
	var d Dialect
	for name, ft := range map[string]schema.FullText{
		"stored":     {Columns: []string{"title", "body"}, Config: "english", Weights: []string{"A", "B"}},
		"unweighted": {Columns: []string{"title"}, Config: "simple"},
		"expression": {Columns: []string{"title", "body"}, Config: "english", Expression: true},
	} {
		want, err := d.ExpandSchema(articlesWithFullText(ft))
		if err != nil {
			t.Fatal(err)
		}
		// What the inspector reads before the fold: the generated column as
		// PostgreSQL prints it, and the index as a plain GIN index.
		read := want.Tables[0]
		read.Columns = append([]schema.Column(nil), read.Columns...)
		read.Indexes = append([]schema.Index(nil), read.Indexes...)
		ix := read.Indexes[0]
		ix.Method, ix.FullText = "gin", nil
		if ft.Expression {
			ix.Parts = []schema.IndexPart{{Expr: `to_tsvector('english', coalesce(title, '') || ' ' || coalesce(body, ''))`}}
		} else {
			for i := range read.Columns {
				if read.Columns[i].Name == schema.TSVectorColumn("articles_search") {
					read.Columns[i].Generated = pgPrinted(ft)
				}
			}
		}
		read.Indexes[0] = ix
		foldFullText(&read)
		got := &schema.Schema{Dialect: "postgres", Tables: []schema.Table{read}}
		if !schema.Equal(want, got) {
			t.Errorf("%s: folded table differs from the expansion:\nwant %+v\n got %+v", name, want.Tables[0], read)
		}
	}

	plain := schema.Table{Name: "t",
		Columns: []schema.Column{{Name: "v_tsv", Type: "tsvector", Nullable: true, Generated: "to_tsvector('simple'::regconfig, a || b)"}},
		Indexes: []schema.Index{{Name: "v", Method: "gin", Parts: []schema.IndexPart{{Column: "v_tsv"}}}}}
	foldFullText(&plain)
	if plain.Indexes[0].Method != "gin" || plain.Indexes[0].FullText != nil {
		t.Fatalf("an unparsable generated column was folded: %+v", plain.Indexes[0])
	}
}

// pgPrinted spells the generation expression the way pg_get_expr prints it.
func pgPrinted(ft schema.FullText) string {
	s := ""
	for i, c := range ft.Columns {
		term := `to_tsvector('` + ft.Config + `'::regconfig, COALESCE(` + c + `, ''::text))`
		if len(ft.Weights) > 0 {
			term = `setweight(` + term + `, '` + ft.Weights[i] + `'::"char")`
		}
		if i == 0 {
			s = term
		} else {
			s = "(" + s + " || " + term + ")"
		}
	}
	return s
}
