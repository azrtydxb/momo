// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

func fullTextIndex(t *testing.T, pkg string, d dialect.Name) schema.Index {
	t.Helper()
	g, diags, err := Load([]string{"./" + pkg}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load %s: %v %v", pkg, err, diags)
	}
	s, sdiags := BuildSchema(g.Models, Config{Dialects: []dialect.Name{d}})
	if len(sdiags) != 0 {
		t.Fatalf("schema diagnostics: %v", sdiags)
	}
	tbl, ok := s.Table("articles")
	if !ok {
		t.Fatal("articles table missing")
	}
	ix, ok := tbl.FullTextIndex()
	if !ok {
		t.Fatal("no full-text index on articles")
	}
	return *ix
}

// Catches: the tag and the explicit call producing different declarations, and
// a source column order that does not follow the call.
func TestGenerateFullTextDeclaration(t *testing.T) {
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		a := fullTextIndex(t, "fulltexttag", d)
		b := fullTextIndex(t, "fulltextcall", d)
		if a.Name != "articles_search" || a.Method != "fulltext" {
			t.Fatalf("%s: tag form = %+v", d, a)
		}
		if got := strings.Join(a.FullText.Columns, ","); got != "title,body" {
			t.Fatalf("%s: columns = %s", d, got)
		}
		if !schema.Equal(&schema.Schema{Dialect: d, Tables: []schema.Table{{Name: "x", Indexes: []schema.Index{a}}}},
			&schema.Schema{Dialect: d, Tables: []schema.Table{{Name: "x", Indexes: []schema.Index{b}}}}) {
			t.Fatalf("%s: tag and call differ:\n%+v\n%+v", d, a, b)
		}
	}
}

// Catches: Concurrently() on a full-text chain being rejected or dropped, so
// the index never reaches the no-transaction migration, and the sibling
// options being lost when it is present.
func TestGenerateFullTextConcurrently(t *testing.T) {
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		ix := fullTextIndex(t, "fulltextconcurrent", d)
		if !ix.Concurrently {
			t.Fatalf("%s: Concurrently() not carried onto the index: %+v", d, ix)
		}
		if ix.FullText.Config != "english" || ix.Name != "articles_search" {
			t.Fatalf("%s: other options lost: %+v", d, ix)
		}
		if plain := fullTextIndex(t, "fulltextcall", d); plain.Concurrently {
			t.Fatalf("%s: an index without Concurrently() is concurrent: %+v", d, plain)
		}
	}
}

// Catches: the tag accepted on a non-text field, a second declaration
// accepted, and a diagnostic that does not name the field.
func TestGenerateFullTextDeclarationRejected(t *testing.T) {
	g, _, err := Load([]string{"./fulltextbad"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	_, diags := BuildSchema(g.Models, Config{Dialects: []dialect.Name{dialect.Postgres}})
	text := diagStrings(diags)
	for _, want := range []string{
		"field Views", "search needs a text column", "one full-text declaration per table",
		"field Labels",
		"is not a plain identifier",
		`full-text tokenizer "unicode61`,
		`full-text parser "ngram; x"`,
		"full-text weights: got 1, want one per column (2)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("diagnostics lack %q:\n%s", want, text)
		}
	}
}

// Catches: a repeated option or a field reference outside the model passing
// the companion reader silently.
func TestGenerateFullTextCallRejected(t *testing.T) {
	_, diags, err := Load([]string{"./fulltextbadcall"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	text := diagStrings(diags)
	for _, want := range []string{"duplicate Config", "unsupported companion expression: &other.Views"} {
		if !strings.Contains(text, want) {
			t.Errorf("diagnostics lack %q:\n%s", want, text)
		}
	}
}
