// SPDX-License-Identifier: Apache-2.0

package sqlite

import "strings"

// tokKind is the kind of a lexical token of SQLite DDL text.
type tokKind int

const (
	tokWord   tokKind = iota // bare word or number: CREATE, 12, a.b
	tokIdent                 // quoted identifier: "x", `x`, [x]
	tokString                // 'text'
	tokGroup                 // a balanced parenthesised group, parentheses included
	tokPunct                 // any other single character: , ; = and so on
)

// token is one lexical unit of DDL text. start and end are byte offsets into
// the scanned string, so the original spelling of a run of tokens can be sliced
// back out of it.
type token struct {
	kind       tokKind
	text       string
	start, end int
}

// upper reports the token upper-cased when it is a bare word, and "" for every
// other kind, so a quoted identifier never matches a keyword.
func (t token) upper() string {
	if t.kind != tokWord {
		return ""
	}
	return strings.ToUpper(t.text)
}

// name returns the token as an identifier: unquoted for a quoted identifier,
// the text itself for anything else.
func (t token) name() string {
	if t.kind == tokIdent {
		return unquoteIdent(t.text)
	}
	return t.text
}

// isName reports whether the token can stand for an identifier.
func (t token) isName() bool { return t.kind == tokWord || t.kind == tokIdent || t.kind == tokString }

// unquoteIdent strips the quotes of "x", `x` or [x] and undoubles an embedded
// quote character.
func unquoteIdent(s string) string {
	if len(s) < 2 {
		return s
	}
	switch s[0] {
	case '"', '`':
		q := string(s[0])
		return strings.ReplaceAll(s[1:len(s)-1], q+q, q)
	case '[':
		return strings.TrimSuffix(s[1:], "]")
	}
	return s
}

// unquoteString strips the quotes of 'x' and undoubles an embedded quote.
func unquoteString(s string) string {
	if len(s) < 2 || s[0] != '\'' {
		return s
	}
	return strings.ReplaceAll(s[1:len(s)-1], "''", "'")
}

// tokenize splits SQL text into tokens. Whitespace and comments are skipped. A
// parenthesised group is one token, so a comma or keyword inside it is never
// seen at the top level. An unterminated quote or group runs to the end of the
// text. It never fails and never panics, whatever the input.
func tokenize(s string) []token {
	var out []token
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v':
			i++
		case c == '-' && i+1 < len(s) && s[i+1] == '-':
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				i = len(s)
			} else {
				i += end + 4
			}
		case c == '\'':
			j := skipQuoted(s, i, '\'')
			out = append(out, token{tokString, s[i:j], i, j})
			i = j
		case c == '"' || c == '`':
			j := skipQuoted(s, i, c)
			out = append(out, token{tokIdent, s[i:j], i, j})
			i = j
		case c == '[':
			j := strings.IndexByte(s[i:], ']')
			if j < 0 {
				j = len(s)
			} else {
				j += i + 1
			}
			out = append(out, token{tokIdent, s[i:j], i, j})
			i = j
		case c == '(':
			j := skipGroup(s, i)
			out = append(out, token{tokGroup, s[i:j], i, j})
			i = j
		case isWordByte(c):
			j := i
			for j < len(s) && isWordByte(s[j]) {
				j++
			}
			out = append(out, token{tokWord, s[i:j], i, j})
			i = j
		default:
			out = append(out, token{tokPunct, s[i : i+1], i, i + 1})
			i++
		}
	}
	return out
}

func isWordByte(c byte) bool {
	return c == '_' || c == '$' || c == '.' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

// skipQuoted returns the offset after the quoted run that starts at s[i] with
// quote q, where a doubled q is an escaped quote. An unterminated run ends at
// the end of s.
func skipQuoted(s string, i int, q byte) int {
	for j := i + 1; j < len(s); j++ {
		if s[j] != q {
			continue
		}
		if j+1 < len(s) && s[j+1] == q {
			j++
			continue
		}
		return j + 1
	}
	return len(s)
}

// skipGroup returns the offset after the parenthesised group that starts at
// s[i], skipping quotes and comments inside it. An unbalanced group ends at the
// end of s.
func skipGroup(s string, i int) int {
	depth := 0
	for j := i; j < len(s); j++ {
		c := s[j]
		switch {
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				return j + 1
			}
		case c == '\'' || c == '"' || c == '`':
			j = skipQuoted(s, j, c) - 1
		case c == '[':
			k := strings.IndexByte(s[j:], ']')
			if k < 0 {
				return len(s)
			}
			j += k
		case c == '-' && j+1 < len(s) && s[j+1] == '-':
			for j < len(s) && s[j] != '\n' {
				j++
			}
		case c == '/' && j+1 < len(s) && s[j+1] == '*':
			end := strings.Index(s[j+2:], "*/")
			if end < 0 {
				return len(s)
			}
			j += end + 3
		}
	}
	return len(s)
}

// groupInner returns the text inside a group token, without its parentheses.
func groupInner(t token) string {
	s := strings.TrimPrefix(t.text, "(")
	return strings.TrimSuffix(s, ")")
}

// splitCommas splits tokens at the top-level comma tokens. A trailing empty
// part is dropped.
func splitCommas(toks []token) [][]token {
	var parts [][]token
	start := 0
	for i, t := range toks {
		if t.kind == tokPunct && t.text == "," {
			parts = append(parts, toks[start:i])
			start = i + 1
		}
	}
	if start < len(toks) {
		parts = append(parts, toks[start:])
	}
	return parts
}

// spelled returns the original text of toks, sliced from src. toks must be a
// contiguous run of tokens scanned from src.
func spelled(src string, toks []token) string {
	if len(toks) == 0 {
		return ""
	}
	return strings.TrimSpace(src[toks[0].start:toks[len(toks)-1].end])
}

// groupNames returns the identifiers of a group token such as (a, "b").
func groupNames(t token) []string {
	var out []string
	for _, p := range splitCommas(tokenize(groupInner(t))) {
		if len(p) > 0 && p[0].isName() {
			out = append(out, p[0].name())
		}
	}
	return out
}
