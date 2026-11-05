// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode"

	"github.com/azrtydxb/momo/dialect"
)

var postgresTypes = map[string]string{
	"int":              "integer",
	"int4":             "integer",
	"integer":          "integer",
	"int8":             "bigint",
	"bigint":           "bigint",
	"int2":             "smallint",
	"smallint":         "smallint",
	"bool":             "boolean",
	"boolean":          "boolean",
	"varchar":          "character varying",
	"float8":           "double precision",
	"double precision": "double precision",
	"float4":           "real",
	"real":             "real",
	"decimal":          "numeric",
	"char":             "character",
}

// NormalizeType returns the canonical spelling of a SQL type so that the
// declared form and the form reported by a database compare equal. It is
// idempotent.
func NormalizeType(d dialect.Name, t string) string {
	s := strings.Join(strings.Fields(strings.ToLower(t)), " ")
	if elem, ok := strings.CutSuffix(s, "[]"); ok && d == dialect.Postgres {
		return NormalizeType(d, elem) + "[]"
	}
	base, args, rest := s, "", ""
	if i := strings.IndexByte(s, '('); i >= 0 {
		base = strings.TrimSpace(s[:i])
		args, rest = splitArgs(s[i:])
	}
	if d == dialect.Postgres {
		if out, ok := postgresTime(base, args, rest); ok {
			return out
		}
		if m, ok := postgresTypes[base]; ok {
			base = m
		}
	}
	if rest != "" {
		rest = " " + rest
	}
	out := base + args + rest
	if d == dialect.SQLite {
		out = strings.ToUpper(out)
	}
	return out
}

// splitArgs splits "(args) rest" into the parenthesised argument list, with
// whitespace inside it removed, and the trimmed text after the closing
// parenthesis. An unbalanced list is returned whole with an empty rest.
func splitArgs(s string) (args, rest string) {
	var sb strings.Builder
	depth := 0
	for i, r := range s {
		switch {
		case r == '(':
			depth++
		case r == ')' && depth > 0:
			depth--
		case r == ' ' && depth > 0:
			continue
		}
		sb.WriteRune(r)
		if depth == 0 {
			return sb.String(), strings.TrimSpace(s[i+1:])
		}
	}
	return sb.String(), ""
}

// postgresTime canonicalises the timestamp and time families, whose optional
// precision sits between the type name and the time zone clause:
// timestamp(3) with time zone.
func postgresTime(base, args, rest string) (string, bool) {
	head, tail, _ := strings.Cut(base, " ")
	if rest != "" {
		tail = strings.TrimSpace(tail + " " + rest)
	}
	var zone string
	switch head {
	case "timestamp", "time":
		zone = "without time zone"
	case "timestamptz", "timetz":
		zone = "with time zone"
		head = strings.TrimSuffix(head, "tz")
	default:
		return "", false
	}
	if tail != "" {
		if tail != "with time zone" && tail != "without time zone" {
			return "", false
		}
		zone = tail
	}
	return head + args + " " + zone, true
}

// NormalizeExpr returns the canonical spelling of a SQL expression: outside
// quoted text whitespace is collapsed and letters are lowercased, and
// redundant enclosing parentheses are removed. Quoted text is kept verbatim.
// It is idempotent.
func NormalizeExpr(d dialect.Name, e string) string {
	for {
		n := normalizeExprOnce(d, e)
		if n == e {
			return n
		}
		e = n
	}
}

func normalizeExprOnce(d dialect.Name, e string) string {
	var sb strings.Builder
	var quote rune
	space := false
	for _, r := range e {
		if quote != 0 {
			sb.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if space {
			if r != ')' && !strings.HasSuffix(sb.String(), "(") {
				sb.WriteByte(' ')
			}
			space = false
		}
		if r == '\'' || r == '"' {
			quote = r
			sb.WriteRune(r)
			continue
		}
		sb.WriteRune(unicode.ToLower(r))
	}
	s := strings.TrimSpace(sb.String())
	for len(s) >= 2 && s[0] == '(' && enclosedByOnePair(s) {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	if d == dialect.Postgres {
		s = postgresRewrite(s)
	}
	return s
}

// postgresRewrite undoes spellings PostgreSQL substitutes for what was
// declared: CURRENT_TIMESTAMP and the legacy 'now'::text default are reported
// as now(), and every string literal gets a ::type cast appended, which is
// dropped here.
func postgresRewrite(s string) string {
	switch s {
	case "current_timestamp", "'now'::text":
		return "now()"
	}
	return stripLiteralCasts(s)
}

// stripLiteralCasts removes a ::type cast that directly follows a
// single-quoted literal. The type is a name with optional multi-word
// suffixes, a length list and array brackets.
func stripLiteralCasts(s string) string {
	var sb strings.Builder
	inDouble := false
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case inDouble:
			inDouble = c != '"'
		case c == '"':
			inDouble = true
		case c == '\'':
			start := i
			j := i + 1
			for j < len(s) && s[j] != '\'' {
				j++
			}
			if j < len(s) {
				j++
			}
			sb.WriteString(s[start:j])
			i = j
			if strings.HasPrefix(s[j:], "::") && j-start > 1 && s[j-1] == '\'' {
				i = j + 2 + castTypeLen(s[j+2:])
			}
			continue
		}
		sb.WriteByte(c)
		i++
	}
	return sb.String()
}

// castTypeLen returns the length of the type name at the start of s.
func castTypeLen(s string) int {
	n := 0
	isWord := func(c byte) bool {
		return c == '_' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
	}
	for n < len(s) && isWord(s[n]) {
		n++
	}
	for _, suffix := range []string{" varying", " precision", " with time zone", " without time zone"} {
		if strings.HasPrefix(s[n:], suffix) {
			n += len(suffix)
			break
		}
	}
	if n < len(s) && s[n] == '(' {
		if k := strings.IndexByte(s[n:], ')'); k >= 0 {
			n += k + 1
		}
	}
	for strings.HasPrefix(s[n:], "[]") {
		n += 2
	}
	return n
}

// enclosedByOnePair reports whether the opening parenthesis at s[0] closes
// at the very end of s.
func enclosedByOnePair(s string) bool {
	var quote byte
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i == len(s)-1
			}
		}
	}
	return false
}

// Equal reports whether two schemas are equivalent: order of named
// collections, the Concurrently migration option, and the spelling of types
// and expressions are ignored. Neither argument is modified.
func Equal(a, b *Schema) bool {
	ca, err := canonical(a)
	if err != nil {
		return false
	}
	cb, err := canonical(b)
	if err != nil {
		return false
	}
	return bytes.Equal(ca, cb)
}

func canonical(s *Schema) ([]byte, error) {
	c, err := normalizedCopy(s)
	if err != nil {
		return nil, err
	}
	return json.Marshal(c)
}

// Normalized returns a deep copy of s with every type and expression
// rewritten to its canonical spelling, the Concurrently migration option
// cleared and the collections sorted. It is idempotent and does not modify s.
// A schema that cannot be JSON-encoded yields an empty schema; Equal reports
// such schemas as unequal.
func Normalized(s *Schema) *Schema {
	c, err := normalizedCopy(s)
	if err != nil {
		return &Schema{}
	}
	return c
}

func normalizedCopy(s *Schema) (*Schema, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var c Schema
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	d := c.Dialect
	for i := range c.Tables {
		t := &c.Tables[i]
		for j := range t.Columns {
			col := &t.Columns[j]
			col.Type = NormalizeType(d, col.Type)
			col.Default = NormalizeExpr(d, col.Default)
			col.Generated = NormalizeExpr(d, col.Generated)
		}
		for j := range t.Checks {
			t.Checks[j].Expr = NormalizeExpr(d, t.Checks[j].Expr)
		}
		normalizeIndexes(d, t.Columns, t.Indexes)
	}
	for i := range c.Views {
		v := &c.Views[i]
		v.Query = NormalizeExpr(d, v.Query)
		normalizeIndexes(d, nil, v.Indexes)
	}
	for i := range c.Functions {
		c.Functions[i].Body = NormalizeExpr(d, c.Functions[i].Body)
	}
	for i := range c.Triggers {
		c.Triggers[i].When = NormalizeExpr(d, c.Triggers[i].When)
	}
	for i := range c.Policies {
		c.Policies[i].Using = NormalizeExpr(d, c.Policies[i].Using)
		c.Policies[i].Check = NormalizeExpr(d, c.Policies[i].Check)
	}
	c.Sort()
	return &c, nil
}
