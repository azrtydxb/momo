// SPDX-License-Identifier: Apache-2.0

package ftsquery

import (
	"reflect"
	"testing"
)

// Catches: operator characters of any dialect surviving into a term, the
// exclusion attached to the wrong term, and the OR split lost.
func TestParseAndRender(t *testing.T) {
	cases := []struct {
		in           string
		fts5, pgText string
		empty        bool
	}{
		{in: `red fox`, fts5: `"red" "fox"`},
		{in: `"red fox" -hen`, fts5: `"red fox" NOT "hen"`},
		{in: `fox OR hound`, fts5: `("fox") OR ("hound")`},
		{in: `a b OR c`, fts5: `("a" "b") OR ("c")`},
		{in: `fo*`, fts5: `"fo"*`, pgText: `'fo':*`},
		{in: `NEAR( (`, empty: true},
		{in: `-spam`, empty: true},
		{in: `   `, empty: true},
		{in: `a"b`, fts5: `"a" "b"`},
		{in: `col:val ^x`, fts5: `"colval" "x"`},
	}
	for _, c := range cases {
		p := Parse(c.in, Web)
		if p.Empty() != c.empty {
			t.Errorf("%q: Empty() = %v", c.in, p.Empty())
			continue
		}
		if c.empty {
			continue
		}
		if got := p.FTS5(); got != c.fts5 {
			t.Errorf("%q: FTS5() = %s, want %s", c.in, got, c.fts5)
		}
		if c.pgText != "" && p.PostgresTSQuery() != c.pgText {
			t.Errorf("%q: PostgresTSQuery() = %s, want %s", c.in, p.PostgresTSQuery(), c.pgText)
		}
	}
	if got := Parse(`go orm`, Plain).Groups; !reflect.DeepEqual(got, [][]Term{{{Text: "go"}, {Text: "orm"}}}) {
		t.Errorf("plain groups = %+v", got)
	}
}

// Catches: an exclusion that binds to one OR group only (FTS5 NOT and tsquery
// & bind tighter than OR), a phrase exclusion without grouping, a quote in a
// lexeme left undoubled, and a bare AND, NOT or NEAR kept as a search word.
func TestRenderCombinations(t *testing.T) {
	cases := []struct {
		in           string
		fts5, pgText string
	}{
		{in: `a b OR c -d`, fts5: `(("a" "b") OR ("c")) NOT "d"`, pgText: `('a' & 'b' | 'c') & ! 'd'`},
		{in: `red -"fox den"`, fts5: `"red" NOT "fox den"`, pgText: `'red' & ! ('fox' <-> 'den')`},
		{in: `"fox den" bark*`, fts5: `"fox den" "bark"*`, pgText: `'fox' <-> 'den' & 'bark':*`},
		{in: `it's`, fts5: `"it's"`, pgText: `'it''s'`},
		{in: `a AND b NOT c`, fts5: `"a" "b" "c"`, pgText: `'a' & 'b' & 'c'`},
		{in: `OR a OR OR b OR`, fts5: `("a") OR ("b")`, pgText: `'a' | 'b'`},
		{in: `a -b -c`, fts5: `"a" NOT "b" NOT "c"`, pgText: `'a' & ! 'b' & ! 'c'`},
		{in: `"unclosed phrase`, fts5: `"unclosed phrase"`, pgText: `'unclosed' <-> 'phrase'`},
	}
	for _, c := range cases {
		p := Parse(c.in, Web)
		if got := p.FTS5(); got != c.fts5 {
			t.Errorf("%q: FTS5() = %s, want %s", c.in, got, c.fts5)
		}
		if got := p.PostgresTSQuery(); got != c.pgText {
			t.Errorf("%q: PostgresTSQuery() = %s, want %s", c.in, got, c.pgText)
		}
	}
	if !Parse(`x*`, Web).HasPrefix() || Parse(`x`, Web).HasPrefix() {
		t.Error("HasPrefix does not follow the trailing star")
	}
	if p := Parse(`a -b *`, Plain); p.HasPrefix() || len(p.Excluded) != 0 || len(p.Groups) != 1 || len(p.Groups[0]) != 2 {
		t.Errorf("plain syntax kept an exclusion or a prefix: %+v", p)
	}
}
