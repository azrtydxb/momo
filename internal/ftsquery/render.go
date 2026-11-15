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

// mysqlWord renders one word for boolean mode. A word with an apostrophe is
// quoted, so the apostrophe is never read next to an operator. Words hold only
// letters, digits, underscores and apostrophes, so a quote cannot occur in one.
func mysqlWord(w string) string {
	if strings.Contains(w, "'") {
		return `"` + w + `"`
	}
	return w
}

// mysqlTerm renders a term with the operator sign (+ required, - excluded).
// A phrase is a quoted string. Boolean mode has no prefix phrase, so a prefix
// term of several words keeps its leading words as a phrase and puts the
// wildcard on the last word; an apostrophe at either end of that word is
// dropped, because a quoted word cannot carry the wildcard.
func mysqlTerm(t Term, sign string) string {
	words := strings.Fields(t.Text)
	switch {
	case len(words) == 0:
		return ""
	case t.Prefix:
		last := strings.Trim(words[len(words)-1], "'")
		if last == "" {
			return ""
		}
		var out string
		if lead := words[:len(words)-1]; len(lead) == 1 {
			out = sign + mysqlWord(lead[0]) + " "
		} else if len(lead) > 1 {
			out = sign + `"` + strings.Join(lead, " ") + `" `
		}
		return out + sign + last + "*"
	case t.Phrase && len(words) > 1:
		return sign + `"` + strings.Join(words, " ") + `"`
	}
	return sign + mysqlWord(words[0])
}

// MySQLBoolean renders the query as a MySQL or MariaDB boolean-mode string:
// each term is required (+word, +"a phrase", +prefix*), several groups are
// written (+a +b) (+c) so that any group may match, and each exclusion is a
// top-level -term. No operator character reaches the string except those the
// renderer writes: words hold only letters, digits, underscores and
// apostrophes. An empty query renders as the empty string.
func (p Parsed) MySQLBoolean() string {
	if p.Empty() {
		return ""
	}
	multi := len(p.Groups) > 1
	var parts []string
	for _, g := range p.Groups {
		var terms []string
		for _, t := range g {
			if s := mysqlTerm(t, "+"); s != "" {
				terms = append(terms, s)
			}
		}
		if len(terms) == 0 {
			continue
		}
		s := strings.Join(terms, " ")
		if multi {
			s = "(" + s + ")"
		}
		parts = append(parts, s)
	}
	for _, t := range p.Excluded {
		if s := mysqlTerm(t, "-"); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
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
