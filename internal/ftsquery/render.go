// SPDX-License-Identifier: Apache-2.0

package ftsquery

import "strings"

// fts5Term renders one term as an FTS5 string, with " doubled.
func fts5Term(t Term) string {
	s := `"` + strings.ReplaceAll(t.Text, `"`, `""`) + `"`
	if t.Prefix {
		s += "*"
	}
	return s
}

// FTS5 renders the query as an FTS5 MATCH expression: terms are quoted
// strings, a group of several terms is an implicit AND, groups are joined by
// OR, and every exclusion follows as NOT. Several groups with an exclusion are
// wrapped in one pair of parentheses, because NOT binds tighter than OR and
// would otherwise exclude from the last group only. An empty query renders as
// the empty string.
func (p Parsed) FTS5() string {
	if p.Empty() {
		return ""
	}
	var sb strings.Builder
	multi := len(p.Groups) > 1
	wrap := multi && len(p.Excluded) > 0
	if wrap {
		sb.WriteByte('(')
	}
	for i, g := range p.Groups {
		if i > 0 {
			sb.WriteString(" OR ")
		}
		if multi {
			sb.WriteByte('(')
		}
		for j, t := range g {
			if j > 0 {
				sb.WriteByte(' ')
			}
			sb.WriteString(fts5Term(t))
		}
		if multi {
			sb.WriteByte(')')
		}
	}
	if wrap {
		sb.WriteByte(')')
	}
	for _, t := range p.Excluded {
		sb.WriteString(" NOT ")
		sb.WriteString(fts5Term(t))
	}
	return sb.String()
}

// pgLexeme quotes one lexeme for to_tsquery, doubling ' and \.
func pgLexeme(w string) string {
	w = strings.ReplaceAll(w, `\`, `\\`)
	return "'" + strings.ReplaceAll(w, "'", "''") + "'"
}

// pgTerm renders a term: a word, or the words of a phrase joined by <->. The
// prefix marker goes on the last word.
func pgTerm(t Term) string {
	words := strings.Fields(t.Text)
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = pgLexeme(w)
	}
	if t.Prefix && len(parts) > 0 {
		parts[len(parts)-1] += ":*"
	}
	return strings.Join(parts, " <-> ")
}

// PostgresTSQuery renders the query as to_tsquery text with every lexeme
// quoted: & inside a group, | between groups, ! for an exclusion, <-> inside
// a phrase. An empty query renders as the empty string.
func (p Parsed) PostgresTSQuery() string {
	if p.Empty() {
		return ""
	}
	groups := make([]string, len(p.Groups))
	for i, g := range p.Groups {
		terms := make([]string, len(g))
		for j, t := range g {
			terms[j] = pgTerm(t)
		}
		groups[i] = strings.Join(terms, " & ")
	}
	out := strings.Join(groups, " | ")
	if len(groups) > 1 && len(p.Excluded) > 0 {
		out = "(" + out + ")"
	}
	for _, t := range p.Excluded {
		s := pgTerm(t)
		if t.Phrase && strings.Contains(s, "<->") {
			s = "(" + s + ")"
		}
		out += " & ! " + s
	}
	return out
}
