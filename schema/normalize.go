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
}

// NormalizeType returns the canonical spelling of a SQL type so that the
// declared form and the form reported by a database compare equal. It is
// idempotent.
func NormalizeType(d dialect.Name, t string) string {
	s := strings.Join(strings.Fields(strings.ToLower(t)), " ")
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
	return base + args + rest
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
	_ = d
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
			sb.WriteByte(' ')
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
	return s
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
		for j := range t.Indexes {
			ix := &t.Indexes[j]
			ix.Concurrently = false
			ix.Where = NormalizeExpr(d, ix.Where)
			for k := range ix.Parts {
				ix.Parts[k].Expr = NormalizeExpr(d, ix.Parts[k].Expr)
			}
		}
	}
	c.Sort()
	return json.Marshal(&c)
}
