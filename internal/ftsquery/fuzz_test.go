// SPDX-License-Identifier: Apache-2.0

package ftsquery

import (
	"regexp"
	"strings"
	"testing"
	"unicode"
)

// fts5Rest matches what is left of an FTS5 query once every double-quoted
// string is removed: only the fixed tokens may remain.
var fts5Rest = regexp.MustCompile(`^[ ()*]*((OR|NOT)[ ()*]*)*$`)

// stripQuoted removes every "..." span of s, with "" as an escaped quote, and
// reports false when a span is not closed.
func stripQuoted(s string) (string, bool) {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '"' {
			out.WriteByte(s[i])
			continue
		}
		closed := false
		for i++; i < len(s); i++ {
			if s[i] != '"' {
				continue
			}
			if i+1 < len(s) && s[i+1] == '"' {
				i++
				continue
			}
			closed = true
			break
		}
		if !closed {
			return "", false
		}
	}
	return out.String(), true
}

// Catches: user text reaching an FTS5 query or a tsquery outside a quoted
// string, an unbalanced quote, a panic on any input, and an empty or
// operator-bearing term.
func FuzzFullTextWebQuery(f *testing.F) {
	for _, s := range []string{
		``, `red fox`, `"red fox" -hen`, `a OR b -c "d e" NEAR( (`, `"`, `((`, `-`, `*`, `\`, `'`,
		`fo*`, `col:val ^x`, `a"b`, `"unclosed`, `-"x y"`, "\xff\xfe", `OR OR`, `a'b*`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		for _, syn := range []Syntax{Web, Plain} {
			p := Parse(s, syn)
			for _, g := range p.Groups {
				for _, term := range append(append([]Term(nil), g...), p.Excluded...) {
					if term.Text == "" || strings.ContainsAny(term.Text, `"\*:()&|!<>`) {
						t.Fatalf("%q: bad term %+v", s, term)
					}
				}
			}
			rest, ok := stripQuoted(p.FTS5())
			if !ok || !fts5Rest.MatchString(rest) {
				t.Fatalf("%q: FTS5() = %q leaves %q outside strings", s, p.FTS5(), rest)
			}
			if p.Empty() != (p.FTS5() == "") {
				t.Fatalf("%q: Empty() = %v but FTS5() = %q", s, p.Empty(), p.FTS5())
			}
			if n := strings.Count(p.PostgresTSQuery(), "'"); n%2 != 0 {
				t.Fatalf("%q: PostgresTSQuery() = %q has unbalanced quotes", s, p.PostgresTSQuery())
			}
		}
		for _, r := range Parse(s, Web).PostgresTSQuery() {
			if r == unicode.ReplacementChar {
				t.Fatalf("%q: replacement character in tsquery", s)
			}
		}
	})
}
