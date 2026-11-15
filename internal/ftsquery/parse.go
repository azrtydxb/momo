// SPDX-License-Identifier: Apache-2.0

// Package ftsquery parses the search text a user types into a small
// dialect-neutral form and renders that form as a PostgreSQL tsquery text, an
// FTS5 query or a MySQL boolean-mode string. Every term is reduced to letters,
// digits, underscores and apostrophes before it is rendered, so no operator of
// any dialect survives into the output. The package imports only the standard
// library, so the root module can use it too.
package ftsquery

import (
	"strings"
	"unicode"
)

// Syntax says how Parse reads the text.
type Syntax uint8

const (
	// Web reads quoted phrases, -exclusions, OR and trailing-star prefixes.
	Web Syntax = iota
	// Plain reads every word as a term of one conjunction.
	Plain
	// Raw is dialect syntax passed through unchanged by the caller; Parse
	// returns an empty result for it.
	Raw
)

// Term is one searched word or phrase.
type Term struct {
	Text   string
	Phrase bool // quoted: Text holds several words
	Prefix bool // trailing *
}

// Parsed is a parsed query: OR-separated groups, each a conjunction, minus the
// excluded terms.
type Parsed struct {
	Groups   [][]Term // OR-separated groups, each a conjunction
	Excluded []Term
}

// Empty reports whether the query has no positive term. Such a query matches
// nothing and scores 0.
func (p Parsed) Empty() bool {
	for _, g := range p.Groups {
		if len(g) > 0 {
			return false
		}
	}
	return true
}

// HasPrefix reports whether any term is a prefix term.
func (p Parsed) HasPrefix() bool {
	for _, g := range p.Groups {
		for _, t := range g {
			if t.Prefix {
				return true
			}
		}
	}
	for _, t := range p.Excluded {
		if t.Prefix {
			return true
		}
	}
	return false
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '\''
}

// cleanWord keeps the word characters of s and reports whether a letter or a
// digit is left.
func cleanWord(s string) (string, bool) {
	var sb strings.Builder
	has := false
	for _, r := range s {
		if !isWordRune(r) {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			has = true
		}
		sb.WriteRune(r)
	}
	return sb.String(), has
}

// operatorWord is a bare uppercase word that reads as an operator in some
// dialect; the web syntax drops it instead of searching for it.
func operatorWord(s string) bool { return s == "AND" || s == "NOT" || s == "NEAR" }

// Parse reads text in syntax syn. It never fails and never panics: text that
// holds no searchable word gives an empty result.
func Parse(text string, syn Syntax) Parsed {
	switch syn {
	case Plain:
		return parsePlain(text)
	case Web:
		return parseWeb(text)
	}
	return Parsed{}
}

func parsePlain(text string) Parsed {
	var g []Term
	for _, f := range strings.FieldsFunc(text, unicode.IsSpace) {
		if w, ok := cleanWord(f); ok {
			g = append(g, Term{Text: w})
		}
	}
	if len(g) == 0 {
		return Parsed{}
	}
	return Parsed{Groups: [][]Term{g}}
}

type webParser struct {
	groups [][]Term
	cur    []Term
	excl   []Term
	word   []rune
	neg    bool // a lone "-" came directly before a quote
}

func (w *webParser) add(t Term, negate bool) {
	if negate {
		w.excl = append(w.excl, t)
		return
	}
	w.cur = append(w.cur, t)
}

func (w *webParser) flushWord() {
	raw := string(w.word)
	w.word = w.word[:0]
	switch {
	case raw == "":
		return
	case raw == "-":
		w.neg = true
		return
	case raw == "OR":
		if len(w.cur) > 0 {
			w.groups = append(w.groups, w.cur)
			w.cur = nil
		}
		return
	}
	negate := false
	if rs := []rune(raw); len(rs) > 1 && rs[0] == '-' && (unicode.IsLetter(rs[1]) || unicode.IsDigit(rs[1])) {
		negate = true
		raw = string(rs[1:])
	}
	prefix := strings.HasSuffix(raw, "*")
	if prefix {
		raw = strings.TrimRight(raw, "*")
	}
	text, ok := cleanWord(raw)
	if !ok || operatorWord(text) {
		return
	}
	w.add(Term{Text: text, Prefix: prefix}, negate)
}

func (w *webParser) phrase(raw string) {
	var words []string
	for _, f := range strings.FieldsFunc(raw, unicode.IsSpace) {
		if t, ok := cleanWord(f); ok {
			words = append(words, t)
		}
	}
	if len(words) > 0 {
		w.add(Term{Text: strings.Join(words, " "), Phrase: true}, w.neg)
	}
}

func parseWeb(text string) Parsed {
	var w webParser
	rs := []rune(text)
	for i := 0; i < len(rs); i++ {
		switch r := rs[i]; {
		case unicode.IsSpace(r):
			w.flushWord()
			w.neg = false
		case r == '"':
			w.flushWord()
			j := i + 1
			for j < len(rs) && rs[j] != '"' {
				j++
			}
			w.phrase(string(rs[i+1 : j]))
			w.neg = false
			i = j // the closing quote, or the end of the text
		default:
			w.word = append(w.word, r)
		}
	}
	w.flushWord()
	if len(w.cur) > 0 {
		w.groups = append(w.groups, w.cur)
	}
	return Parsed{Groups: w.groups, Excluded: w.excl}
}
