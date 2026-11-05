// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/azrtydxb/momo/dialect"
)

// NamedArg is a named argument for an @name placeholder in raw SQL. Build it
// with Named.
type NamedArg struct {
	Name  string
	Value any
}

// Named binds value to every @name placeholder in a raw fragment. A name with
// no matching argument is a build error, except on MySQL where an unmatched
// @name is a user variable and is left verbatim.
func Named(name string, value any) NamedArg { return NamedArg{Name: name, Value: value} }

// inArg expands to one bind per element of a slice.
type inArg struct{ values any }

// In expands a slice argument into a comma-separated list of bind markers,
// one per element, for a raw placeholder written inside the parentheses of
// IN (?). An empty slice, or a value that is not a slice, is a build error.
func In(values any) any { return inArg{values} }

// identArg renders a quoted identifier.
type identArg struct{ name string }

// Ident renders name as a quoted identifier in the dialect's style. It is
// never bound as a value.
func Ident(name string) any { return identArg{name} }

// ------------------------------------------------------------ tokenizer

type rawTokKind uint8

const (
	rawText  rawTokKind = iota // copied verbatim
	rawParam                   // a positional '?'
	rawNamed                   // an @name; text is the name
)

type rawTok struct {
	kind rawTokKind
	text string
}

func isIdentByte(c byte, d *dialect.Dialect) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
		c >= 0x80 || c == '$' && d.Name == dialect.Postgres
}

func isIdentStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

// scanRaw splits sql into verbatim text and placeholders. A '?' or '@name'
// inside a string literal, quoted identifier, comment or PostgreSQL
// dollar-quoted string is text, as are the PostgreSQL operators ?| ?& @? @@
// and @>. "??" yields a literal '?'.
func scanRaw(sql string, d *dialect.Dialect) []rawTok {
	var toks []rawTok
	start := 0 // start of the pending verbatim run
	pg, my := d.Name == dialect.Postgres, d.Name == dialect.MySQL
	flush := func(end int) {
		if end > start {
			toks = append(toks, rawTok{rawText, sql[start:end]})
		}
	}
	// skipQuoted returns the index after a quoted run beginning at i (the
	// opening quote), honouring a doubled quote and, when backslash is set,
	// backslash escapes. An unterminated run extends to the end of sql.
	skipQuoted := func(i int, q byte, backslash bool) int {
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
		return len(sql)
	}
	i := 0
	for i < len(sql) {
		c := sql[i]
		switch {
		case c == '\'':
			esc := my || pg && i > 0 && (sql[i-1] == 'e' || sql[i-1] == 'E') && (i < 2 || !isIdentByte(sql[i-2], d))
			i = skipQuoted(i, '\'', esc)
		case c == '"':
			i = skipQuoted(i, '"', my)
		case c == '`' && d.Name != dialect.Postgres:
			i = skipQuoted(i, '`', false)
		case c == '[' && d.Name == dialect.SQLite:
			for i++; i < len(sql) && sql[i] != ']'; i++ {
			}
			if i < len(sql) {
				i++
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
		case c == '$' && pg && (i == 0 || !isIdentByte(sql[i-1], d)):
			j := i + 1
			if j < len(sql) && isIdentStart(sql[j]) {
				for j < len(sql) && isIdentByte(sql[j], d) && sql[j] != '$' {
					j++
				}
			}
			if j >= len(sql) || sql[j] != '$' {
				i++ // $1 or a lone dollar
				break
			}
			tag := sql[i : j+1]
			end := indexFrom(sql, tag, j+1)
			if end < 0 {
				i = len(sql)
			} else {
				i = end + len(tag)
			}
		case c == '?':
			switch {
			case i+1 < len(sql) && sql[i+1] == '?':
				flush(i)
				toks = append(toks, rawTok{rawText, "?"})
				i += 2
				start = i
			case pg && i+1 < len(sql) && (sql[i+1] == '&' || sql[i+1] == '|' && !(i+2 < len(sql) && sql[i+2] == '|')):
				i += 2 // jsonb ?& and ?| operators
			default:
				flush(i)
				toks = append(toks, rawTok{rawParam, ""})
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
			case i+1 < len(sql) && isIdentStart(sql[i+1]) && (i == 0 || !isIdentByte(sql[i-1], d)):
				j := i + 1
				for j < len(sql) && isIdentByte(sql[j], d) {
					j++
				}
				flush(i)
				toks = append(toks, rawTok{rawNamed, sql[i+1 : j]})
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
	return toks
}

func indexFrom(s, sub string, from int) int {
	for i := from; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// ------------------------------------------------------------ rendering

func rawErr(format string, a ...any) *Error {
	return &Error{Kind: ErrValidation, Op: "build", Err: fmt.Errorf(format, a...)}
}

// appendRaw renders a raw fragment. '?' binds the next positional argument
// (an Expr argument renders inline, an In argument expands to a list, an
// Ident argument renders a quoted identifier), "??" is a literal '?', and
// @name binds a NamedArg. Nothing inside a string literal, quoted
// identifier, comment or dollar-quoted string is touched. The argument count
// must match the placeholder count.
func (b *Builder) appendRaw(sql string, args []any) {
	var pos []any
	var named map[string]any
	for _, a := range args {
		na, ok := a.(NamedArg)
		if !ok {
			pos = append(pos, a)
			continue
		}
		if named == nil {
			named = map[string]any{}
		}
		if _, dup := named[na.Name]; dup {
			b.SetErr(rawErr("raw SQL %q has two named arguments called %q", sql, na.Name))
			return
		}
		named[na.Name] = na.Value
	}
	n := 0
	for _, tok := range scanRaw(sql, b.d) {
		switch tok.kind {
		case rawText:
			b.buf = append(b.buf, tok.text...)
		case rawParam:
			if n >= len(pos) {
				b.SetErr(rawErr("raw SQL %q has more placeholders than the %d arguments given", sql, len(pos)))
				return
			}
			b.appendRawArg(pos[n])
			n++
		case rawNamed:
			v, ok := named[tok.text]
			if !ok {
				if b.d.Name == dialect.MySQL {
					b.buf = append(b.buf, '@')
					b.buf = append(b.buf, tok.text...)
					continue
				}
				b.SetErr(rawErr("raw SQL %q uses @%s but no Named(%q, ...) argument was given", sql, tok.text, tok.text))
				return
			}
			b.appendRawArg(v)
		}
		if b.err != nil {
			return
		}
	}
	if n != len(pos) {
		b.SetErr(rawErr("raw SQL %q has %d placeholders but %d arguments were given", sql, n, len(pos)))
	}
}

func (b *Builder) appendRawArg(v any) {
	switch a := v.(type) {
	case Expr:
		a.AppendSQL(b)
	case inArg:
		b.appendIn(a)
	case identArg:
		if a.name == "" {
			b.SetErr(rawErr("raw SQL Ident argument has an empty name"))
			return
		}
		b.Ident(a.name)
	default:
		b.Arg(v)
		b.checkMaxParams()
	}
}

func (b *Builder) checkMaxParams() {
	if m := b.d.MaxParams; m > 0 && len(b.args) > m {
		b.SetErr(rawErr("%d bind parameters exceed the %s limit of %d", len(b.args), b.d.Name, m))
	}
}

func (b *Builder) appendIn(a inArg) {
	rv := reflect.ValueOf(a.values)
	if !rv.IsValid() || rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		b.SetErr(rawErr("In needs a slice, got %T", a.values))
		return
	}
	if rv.Len() == 0 {
		b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: errors.New("empty IN list in raw SQL; use the typed In which renders a constant")})
		return
	}
	if m := b.d.MaxParams; m > 0 && len(b.args)+rv.Len() > m {
		b.SetErr(rawErr("%d bind parameters exceed the %s limit of %d", len(b.args)+rv.Len(), b.d.Name, m))
		return
	}
	for i := 0; i < rv.Len(); i++ {
		if i > 0 {
			b.buf = append(b.buf, ", "...)
		}
		b.Arg(rv.Index(i).Interface())
	}
}
