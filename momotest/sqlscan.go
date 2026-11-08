// SPDX-License-Identifier: Apache-2.0

package momotest

import "bytes"

// stripSQL returns sql with the parts that cannot execute a statement replaced
// by single spaces: -- comments, nested /* */ comments, '...' strings (with
// doubled-quote escapes) and E'...' strings (with backslash escapes). Plain
// '...' strings are read as PostgreSQL does with standard_conforming_strings
// on, its default since 9.1: a backslash is an ordinary character. Quoted
// identifiers are kept whole, because an extension name may be quoted.
//
// Dollar-quoted bodies ($$...$$ and $tag$...$tag$) are scanned as code, in a
// pass of their own: a DO block can run CREATE EXTENSION, and wrongly flagging
// text inside a dollar-quoted string is a loud failure the author can fix,
// where missing a real statement would be silent. A body that is not valid SQL
// on its own, such as the text don't, ends inside a string when scanned; its
// raw text is kept then, so an apostrophe in a body can never hide what follows
// the closing delimiter. String literals inside a body are stripped like any
// other, so an EXECUTE string is not inspected.
func stripSQL(sql []byte) []byte {
	out, _ := scanSQL(sql)
	return out
}

// scanSQL is stripSQL that also reports whether the input ended outside every
// comment, string and quoted identifier.
func scanSQL(sql []byte) (out []byte, clean bool) {
	out = make([]byte, 0, len(sql))
	clean = true
	n := len(sql)
	for i := 0; i < n; {
		c := sql[i]
		switch {
		case c == '-' && i+1 < n && sql[i+1] == '-':
			for i < n && sql[i] != '\n' {
				i++
			}
			out = append(out, ' ')
		case c == '/' && i+1 < n && sql[i+1] == '*':
			var ok bool
			i, ok = skipBlockComment(sql, i)
			clean = clean && ok
			out = append(out, ' ')
		case c == '\'':
			var ok bool
			i, ok = skipString(sql, i+1, false)
			clean = clean && ok
			out = append(out, ' ')
		case (c == 'E' || c == 'e') && i+1 < n && sql[i+1] == '\'' && (i == 0 || !isIdent(sql[i-1])):
			var ok bool
			i, ok = skipString(sql, i+2, true)
			clean = clean && ok
			out = append(out, ' ')
		case c == '"':
			// A quoted identifier is kept, including "" escapes.
			start := i
			var ok bool
			i, ok = skipQuotedIdent(sql, i+1)
			clean = clean && ok
			out = append(out, sql[start:i]...)
		case c == '$' && (i == 0 || !isIdent(sql[i-1])):
			open := dollarTagEnd(sql, i)
			if open == 0 {
				out = append(out, c)
				i++
				break
			}
			delim := sql[i:open]
			bodyEnd, next := len(sql), len(sql)
			if k := bytes.Index(sql[open:], delim); k >= 0 {
				bodyEnd, next = open+k, open+k+len(delim)
			} else {
				clean = false // no closing delimiter
			}
			body := sql[open:bodyEnd]
			sub, subClean := scanSQL(body)
			if !subClean {
				sub = body
			}
			out = append(out, ' ')
			out = append(out, sub...)
			out = append(out, ' ')
			i = next
		default:
			out = append(out, c)
			i++
		}
	}
	return out, clean
}

// skipBlockComment returns the index after the (nested) comment that starts at
// i, and whether it was closed.
func skipBlockComment(sql []byte, i int) (int, bool) {
	n := len(sql)
	depth := 0
	for i < n {
		switch {
		case sql[i] == '/' && i+1 < n && sql[i+1] == '*':
			depth++
			i += 2
		case sql[i] == '*' && i+1 < n && sql[i+1] == '/':
			depth--
			i += 2
			if depth == 0 {
				return i, true
			}
		default:
			i++
		}
	}
	return n, false
}

// skipString returns the index after the string body that starts at i (just
// past the opening quote), and whether the string was closed. A doubled quote
// is an escaped quote; with backslashes true, a backslash also escapes the next
// byte.
func skipString(sql []byte, i int, backslashes bool) (int, bool) {
	n := len(sql)
	for i < n {
		switch {
		case backslashes && sql[i] == '\\':
			i += 2
		case sql[i] == '\'':
			if i+1 < n && sql[i+1] == '\'' {
				i += 2
				continue
			}
			return i + 1, true
		default:
			i++
		}
	}
	return n, false
}

// skipQuotedIdent returns the index after the quoted identifier whose body
// starts at i, and whether it was closed.
func skipQuotedIdent(sql []byte, i int) (int, bool) {
	n := len(sql)
	for i < n {
		if sql[i] == '"' {
			if i+1 < n && sql[i+1] == '"' {
				i += 2
				continue
			}
			return i + 1, true
		}
		i++
	}
	return n, false
}

// dollarTagEnd returns the index after a dollar-quote delimiter ($$ or
// $tag$) that starts at i, or 0 when sql[i] does not start one ($1 is a
// parameter, not a delimiter, because a tag cannot start with a digit).
func dollarTagEnd(sql []byte, i int) int {
	j := i + 1
	if j < len(sql) && sql[j] == '$' {
		return j + 1
	}
	if j >= len(sql) || !(isIdent(sql[j]) && (sql[j] < '0' || sql[j] > '9')) {
		return 0
	}
	for j < len(sql) && isIdent(sql[j]) {
		j++
	}
	if j < len(sql) && sql[j] == '$' {
		return j + 1
	}
	return 0
}

func isIdent(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b >= 0x80
}
