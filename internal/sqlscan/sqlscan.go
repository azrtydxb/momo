// SPDX-License-Identifier: Apache-2.0

// Package sqlscan tokenizes raw SQL fragments into verbatim text and
// placeholders. It knows string literals, quoted identifiers, comments and
// PostgreSQL dollar quotes per dialect, and imports nothing from the momo
// root package.
package sqlscan

import (
	"fmt"

	"github.com/azrtydxb/momo/dialect"
)

// Kind classifies a Tok.
type Kind uint8

const (
	Text   Kind = iota // copied verbatim
	Param              // a positional '?'
	Named              // an @name; Text is the name
	Dollar             // a native $n; Text is the digits
)

// Tok is one token of a scanned fragment.
type Tok struct {
	Kind Kind
	Text string
}

// isIdentByte reports whether c continues an identifier in dialect d.
func isIdentByte(c byte, d *dialect.Dialect) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
		c >= 0x80 || c == '$' && d.Name == dialect.Postgres
}

func isIdentStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

// Scan splits sql into verbatim text and placeholders. A '?' or '@name'
// inside a string literal, quoted identifier, comment or PostgreSQL
// dollar-quoted string is text, as are the PostgreSQL operators ?| ?& @? @@
// and @>. "??" yields a literal '?'. A native $n outside those is a Dollar
// token on every dialect, so the caller can bind it on PostgreSQL and reject
// it elsewhere.
func Scan(sql string, d *dialect.Dialect) (toks []Tok, err error) {
	unterminated := func(what string) {
		if err == nil {
			err = fmt.Errorf("raw SQL %q has an unterminated %s", sql, what)
		}
	}
	start := 0 // start of the pending verbatim run
	pg, my := d.Name == dialect.Postgres, d.Name == dialect.MySQL
	flush := func(end int) {
		if end > start {
			toks = append(toks, Tok{Text, sql[start:end]})
		}
	}
	// skipQuoted returns the index after a quoted run beginning at i (the
	// opening quote), honouring a doubled quote and, when backslash is set,
	// backslash escapes. An unterminated run is recorded as an error.
	skipQuoted := func(i int, q byte, backslash bool, what string) int {
		for j := i + 1; j < len(sql); j++ {
			switch {
			case backslash && sql[j] == '\\':
				j++
			case sql[j] == q:
				if j+1 < len(sql) && sql[j+1] == q {
					j++
					continue
				}
				return j + 1
			}
		}
		unterminated(what)
		return len(sql)
	}
	i := 0
	for i < len(sql) {
		c := sql[i]
		switch {
		case c == '\'':
			esc := my || pg && i > 0 && (sql[i-1] == 'e' || sql[i-1] == 'E') && (i < 2 || !isIdentByte(sql[i-2], d))
			i = skipQuoted(i, '\'', esc, "string literal")
		case c == '"':
			i = skipQuoted(i, '"', my, "quoted identifier")
		case c == '`' && d.Name != dialect.Postgres:
			i = skipQuoted(i, '`', false, "quoted identifier")
		case c == '[' && d.Name == dialect.SQLite:
			for i++; i < len(sql) && sql[i] != ']'; i++ {
			}
			if i < len(sql) {
				i++
			} else {
				unterminated("quoted identifier")
			}
		case c == '-' && i+1 < len(sql) && sql[i+1] == '-' && (!my || i+2 >= len(sql) || sql[i+2] == ' ' || sql[i+2] == '\t' || sql[i+2] == '\n' || sql[i+2] == '\r'),
			c == '#' && my:
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(sql) && sql[i+1] == '*':
			depth := 1
			i += 2
			for i < len(sql) && depth > 0 {
				switch {
				case pg && sql[i] == '/' && i+1 < len(sql) && sql[i+1] == '*':
					depth++
					i += 2
				case sql[i] == '*' && i+1 < len(sql) && sql[i+1] == '/':
					depth--
					i += 2
				default:
					i++
				}
			}
			if depth > 0 {
				unterminated("block comment")
			}
		case c == '$' && !pg && i+1 < len(sql) && sql[i+1] >= '0' && sql[i+1] <= '9' && (i == 0 || !isIdentByte(sql[i-1], d)):
			i = scanDollarParam(sql, i, &start, &toks)
		case c == '$' && pg && (i == 0 || !isIdentByte(sql[i-1], d)):
			j := i + 1
			if j < len(sql) && isIdentStart(sql[j]) {
				for j < len(sql) && isIdentByte(sql[j], d) && sql[j] != '$' {
					j++
				}
			}
			if j >= len(sql) || sql[j] != '$' {
				if i+1 < len(sql) && sql[i+1] >= '0' && sql[i+1] <= '9' {
					i = scanDollarParam(sql, i, &start, &toks)
				} else {
					i++ // a lone dollar
				}
				break
			}
			tag := sql[i : j+1]
			end := indexFrom(sql, tag, j+1)
			if end < 0 {
				unterminated("dollar quote")
				i = len(sql)
			} else {
				i = end + len(tag)
			}
		case c == '?':
			switch {
			case i+1 < len(sql) && sql[i+1] == '?':
				flush(i)
				toks = append(toks, Tok{Text, "?"})
				i += 2
				start = i
			case pg && i+1 < len(sql) && (sql[i+1] == '&' || sql[i+1] == '|' && !(i+2 < len(sql) && sql[i+2] == '|')):
				i += 2 // jsonb ?& and ?| operators
			default:
				flush(i)
				toks = append(toks, Tok{Param, ""})
				i++
				start = i
			}
		case c == '@':
			switch {
			case i+1 < len(sql) && (sql[i+1] == '@' || sql[i+1] == '?' || sql[i+1] == '>'):
				i += 2 // @@ @? @> operators, MySQL @@system variables
				if my && sql[i-1] == '@' {
					for i < len(sql) && isIdentByte(sql[i], d) {
						i++
					}
				}
			case i+1 < len(sql) && isIdentStart(sql[i+1]) && (i == 0 || !isIdentByte(sql[i-1], d)) && !(pg && i > 0 && sql[i-1] == '<'):
				j := i + 1
				for j < len(sql) && isIdentByte(sql[j], d) {
					j++
				}
				flush(i)
				toks = append(toks, Tok{Named, sql[i+1 : j]})
				i = j
				start = i
			default:
				i++
			}
		case isIdentStart(c):
			for i < len(sql) && isIdentByte(sql[i], d) {
				i++
			}
		default:
			i++
		}
	}
	flush(len(sql))
	return toks, err
}

// scanDollarParam consumes a $n at i, flushing the verbatim run before it,
// and returns the index after the digits.
func scanDollarParam(sql string, i int, start *int, toks *[]Tok) int {
	j := i + 1
	for j < len(sql) && sql[j] >= '0' && sql[j] <= '9' {
		j++
	}
	if i > *start {
		*toks = append(*toks, Tok{Text, sql[*start:i]})
	}
	*toks = append(*toks, Tok{Dollar, sql[i+1 : j]})
	*start = j
	return j
}

func indexFrom(s, sub string, from int) int {
	for i := from; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
