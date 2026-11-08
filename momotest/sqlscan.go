// SPDX-License-Identifier: Apache-2.0

package momotest

// stripSQL returns sql with the parts that cannot execute a statement replaced
// by single spaces: -- comments, nested /* */ comments, '...' strings (with ”
// escapes) and E'...' strings (with backslash escapes). Quoted identifiers are
// kept, because an extension name may be quoted. Dollar-quote delimiters
// ($$ and $tag$) are dropped but their bodies are kept and scanned as code:
// a DO block can run CREATE EXTENSION, and wrongly flagging text inside a
// dollar-quoted string is a loud failure the author can fix, where missing a
// real statement would be silent.
func stripSQL(sql []byte) []byte {
	out := make([]byte, 0, len(sql))
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
			depth := 0
			for i < n {
				switch {
				case sql[i] == '/' && i+1 < n && sql[i+1] == '*':
					depth++
					i += 2
				case sql[i] == '*' && i+1 < n && sql[i+1] == '/':
					depth--
					i += 2
				default:
					i++
				}
				if depth == 0 {
					break
				}
			}
			out = append(out, ' ')
		case c == '\'':
			i = skipString(sql, i+1, false)
			out = append(out, ' ')
		case (c == 'E' || c == 'e') && i+1 < n && sql[i+1] == '\'' && (i == 0 || !isIdent(sql[i-1])):
			i = skipString(sql, i+2, true)
			out = append(out, ' ')
		case c == '"':
			// A quoted identifier is kept whole, including "" escapes.
			start := i
			i++
			for i < n {
				if sql[i] == '"' {
					if i+1 < n && sql[i+1] == '"' {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
			out = append(out, sql[start:i]...)
		case c == '$' && (i == 0 || !isIdent(sql[i-1])):
			if end := dollarTagEnd(sql, i); end > 0 {
				i = end
				out = append(out, ' ')
			} else {
				out = append(out, c)
				i++
			}
		default:
			out = append(out, c)
			i++
		}
	}
	return out
}

// skipString returns the index after the string body that starts at i (just
// past the opening quote). A doubled quote is an escaped quote; with
// backslashes true, a backslash also escapes the next byte.
func skipString(sql []byte, i int, backslashes bool) int {
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
			return i + 1
		default:
			i++
		}
	}
	return n
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
