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
	Phrase bool // quoted or split at punctuation: Text holds several words
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

// splitWords splits s at every character that is not a word character and
// returns the runs that hold a letter or a digit, so well-known is two words
// and 3.14 is two words, as the FTS5 unicode61 tokenizer reads them.
func splitWords(s string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return !isWordRune(r) }) {
		if strings.IndexFunc(f, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0 {
			out = append(out, f)
		}
	}
	return out
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
	for _, w := range splitWords(text) {
		g = append(g, Term{Text: w})
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
	switch raw {
	case "":
		return
	case "-":
		w.neg = true
		return
	case "OR":
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
	words := splitWords(raw)
	if len(words) == 0 || (len(words) == 1 && operatorWord(words[0])) {
		return
	}
	w.add(Term{Text: strings.Join(words, " "), Phrase: len(words) > 1, Prefix: prefix}, negate)
}

func (w *webParser) phrase(raw string) {
	words := splitWords(raw)
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
