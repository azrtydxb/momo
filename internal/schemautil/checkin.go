// SPDX-License-Identifier: Apache-2.0

package schemautil

import (
	"slices"
	"strings"
	"unicode"

	"github.com/azrtydxb/momo/dialect"
)

// SortedValues returns the values sorted and without duplicates, as a new slice.
func SortedValues(values []string) []string {
	out := slices.Clone(values)
	slices.Sort(out)
	return slices.Compact(out)
}

// CheckInExpr renders the expression of a value-list check for dialect d: the
// column quoted the way d quotes identifiers, then IN and the sorted,
// de-duplicated values as single-quoted literals separated by a comma, with no
// space after it (the spelling MySQL and MariaDB report back).
func CheckInExpr(d dialect.Name, column string, values []string) string {
	q := `"`
	if d == dialect.MySQL {
		q = "`"
	}
	sorted := SortedValues(values)
	lits := make([]string, len(sorted))
	for i, v := range sorted {
		lits[i] = "'" + strings.ReplaceAll(v, "'", "''") + "'"
	}
	return q + strings.ReplaceAll(column, q, q+q) + q + " IN (" + strings.Join(lits, ",") + ")"
}

type checkTok struct {
	text   string
	str    bool // a string literal; text holds its value
	punct  bool
	quoted bool
}

// ParseCheckIn reads the expression of a CHECK constraint that restricts one
// column to a list of string literals and returns the column and the values in
// the order written. It accepts the spellings the supported databases report:
// col IN ('a', 'b'), the PostgreSQL form col = ANY (ARRAY['a'::text, 'b'::text])
// with its casts, and the MySQL form with character set introducers. Any other
// expression, a list that holds something but string literals, or a literal
// with a backslash, reports false.
func ParseCheckIn(expr string) (column string, values []string, ok bool) {
	// MySQL reports the quotes of a string literal escaped (_utf8mb4\'a\').
	toks, ok := checkTokens(strings.ReplaceAll(expr, `\'`, `'`))
	if !ok {
		return "", nil, false
	}
	w := toks[:0:0]
	for _, t := range toks {
		if t.punct && strings.ContainsAny(t.text, "()[]") {
			continue
		}
		w = append(w, t)
	}
	if len(w) < 3 || w[0].str || w[0].punct {
		return "", nil, false
	}
	column = w[0].text
	i := 1
	switch {
	case !w[i].punct && !w[i].str && !w[i].quoted && strings.EqualFold(w[i].text, "in"):
		i++
	case w[i].punct && w[i].text == "=" && i+2 < len(w) && !w[i+1].str && strings.EqualFold(w[i+1].text, "any") && strings.EqualFold(w[i+2].text, "array"):
		i += 3
	case w[i].punct && w[i].text == "=" && len(w) == 3 && w[2].str:
		// PostgreSQL and MySQL report a list of one value as an equality.
		i++
	default:
		return "", nil, false
	}
	for i < len(w) {
		if !w[i].str {
			return "", nil, false
		}
		values = append(values, w[i].text)
		i++
		if i == len(w) {
			break
		}
		if !w[i].punct || w[i].text != "," || i+1 == len(w) {
			return "", nil, false
		}
		i++
	}
	if len(values) == 0 {
		return "", nil, false
	}
	return column, values, true
}

// checkTokens splits a check expression into words, string literals and the
// punctuation ( ) [ ] , =. Casts (::type), character set introducers
// (_utf8mb4'x') and identifier quotes are dropped; anything else is not a
// value list and reports false.
func checkTokens(expr string) ([]checkTok, bool) {
	rs := []rune(expr)
	var out []checkTok
	isWord := func(r rune) bool {
		return r == '_' || r == '$' || r == '.' || unicode.IsLetter(r) || unicode.IsDigit(r)
	}
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case unicode.IsSpace(r):
			i++
		case r == '\'':
			var sb strings.Builder
			i++
			closed := false
			for i < len(rs) {
				if rs[i] == '\'' {
					if i+1 < len(rs) && rs[i+1] == '\'' {
						sb.WriteRune('\'')
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				if rs[i] == '\\' {
					return nil, false
				}
				sb.WriteRune(rs[i])
				i++
			}
			if !closed {
				return nil, false
			}
			out = append(out, checkTok{text: sb.String(), str: true})
		case r == '"' || r == '`':
			var sb strings.Builder
			i++
			closed := false
			for i < len(rs) {
				if rs[i] == r {
					if i+1 < len(rs) && rs[i+1] == r {
						sb.WriteRune(r)
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				sb.WriteRune(rs[i])
				i++
			}
			if !closed {
				return nil, false
			}
			out = append(out, checkTok{text: sb.String(), quoted: true})
		case r == ':' && i+1 < len(rs) && rs[i+1] == ':':
			i = skipCast(rs, i+2)
		case strings.ContainsRune("()[],=", r):
			out = append(out, checkTok{text: string(r), punct: true})
			i++
		case isWord(r):
			j := i
			for j < len(rs) && isWord(rs[j]) {
				j++
			}
			w := string(rs[i:j])
			i = j
			if strings.HasPrefix(w, "_") && i < len(rs) && rs[i] == '\'' {
				continue // a character set introducer such as _utf8mb4
			}
			out = append(out, checkTok{text: w})
		default:
			return nil, false
		}
	}
	return out, true
}

// skipCast returns the index after the type of a ::cast that starts at i: a
// type name of one or more words, an optional (length) and any [] suffixes.
func skipCast(rs []rune, i int) int {
	word := func(i int) int {
		for i < len(rs) && unicode.IsSpace(rs[i]) {
			i++
		}
		j := i
		for j < len(rs) && (rs[j] == '_' || unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j])) {
			j++
		}
		return j
	}
	j := word(i)
	for {
		k := word(j)
		if k == j {
			break
		}
		next := strings.ToLower(strings.TrimSpace(string(rs[j:k])))
		if next != "varying" && next != "precision" && next != "without" && next != "with" && next != "time" && next != "zone" {
			break
		}
		j = k
	}
	if j < len(rs) && rs[j] == '(' {
		k := j + 1
		for k < len(rs) && (unicode.IsDigit(rs[k]) || rs[k] == ',' || unicode.IsSpace(rs[k])) {
			k++
		}
		if k < len(rs) && rs[k] == ')' {
			j = k + 1
		}
	}
	for j+1 < len(rs) && rs[j] == '[' && rs[j+1] == ']' {
		j += 2
	}
	return j
}
