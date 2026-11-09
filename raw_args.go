// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/sqlscan"
)

// NamedArg is a named argument for an @name placeholder in raw SQL. Build it
// with Named.
type NamedArg struct {
	Name  string
	Value any
}

// Named binds value to every @name placeholder in a raw fragment. An @name
// with no matching argument is a build error, except on MySQL where it is a
// user variable and is left verbatim. A Named argument that no @name in the
// fragment references is a build error on every dialect, so a typo cannot
// run silently.
//
// Raw scanner assumptions: MySQL NO_BACKSLASH_ESCAPES and PostgreSQL
// standard_conforming_strings=off are not supported (MySQL strings always
// honour backslash escapes, PostgreSQL ones only after E). MySQL /*! ... */
// executable comments are treated as ordinary comments. On PostgreSQL the
// operators ?| and ?& are operators, not binds followed by | or &: write
// "? |" with a space for a bind followed by OR. A "<@" on PostgreSQL is the
// containment operator, never an @name. On PostgreSQL a native $n binds
// positional argument n (a reused $n shares one bind); a fragment uses one
// style, so mixing $n with ? or @name is a build error, and so is a $n
// off PostgreSQL. An unterminated string, quoted identifier,
// block comment or dollar quote is a build error.
func Named(name string, value any) NamedArg { return NamedArg{Name: name, Value: value} }

// inArg expands to one bind per element of a slice.
type inArg struct{ values any }

// In expands a slice argument into a comma-separated list of bind markers,
// one per element, for a raw placeholder written inside the parentheses of
// IN (?). An empty slice, or a value that is not a slice, is a build error,
// as is an element that is an Ident, In, Named or Expr argument. A []byte is a
// slice like any other: In([]byte) expands to one bind per byte.
func In(values any) any { return inArg{values} }

// identArg renders a quoted identifier.
type identArg struct{ name string }

// Ident renders name as a quoted identifier in the dialect's style. It is
// never bound as a value.
func Ident(name string) any { return identArg{name} }

// ------------------------------------------------------------ rendering

func rawErr(format string, a ...any) *Error {
	return validation("build", "", format, a...)
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
	toks, err := sqlscan.Scan(sql, b.d)
	if err != nil {
		b.SetErr(rawErr("%s", err))
		return
	}
	if b.appendRawDollar(sql, toks, pos, named != nil) {
		return
	}
	used := map[string]bool{}
	n := 0
	for _, tok := range toks {
		switch tok.Kind {
		case sqlscan.Text:
			b.buf = append(b.buf, tok.Text...)
		case sqlscan.Param:
			if n >= len(pos) {
				b.SetErr(rawErr("raw SQL %q has more placeholders than the %d arguments given", sql, len(pos)))
				return
			}
			b.appendRawArg(pos[n])
			n++
		case sqlscan.Named:
			used[tok.Text] = true
			v, ok := named[tok.Text]
			if !ok {
				if b.d.Name == dialect.MySQL {
					b.buf = append(b.buf, '@')
					b.buf = append(b.buf, tok.Text...)
					continue
				}
				b.SetErr(rawErr("raw SQL %q uses @%s but no Named(%q, ...) argument was given", sql, tok.Text, tok.Text))
				return
			}
			b.appendRawArg(v)
		}
		if b.err != nil {
			return
		}
	}
	for _, a := range args {
		if na, ok := a.(NamedArg); ok && !used[na.Name] {
			b.SetErr(rawErr("raw SQL %q has Named(%q, ...) but no @%s uses it", sql, na.Name, na.Name))
			return
		}
	}
	if n != len(pos) {
		b.SetErr(rawErr("raw SQL %q has %d placeholders but %d arguments were given", sql, n, len(pos)))
	}
}

// appendRawDollar renders a fragment written with native $n placeholders and
// reports whether it handled the fragment (true also on error). $n binds
// positional argument n; a reused $n shares one bind for a plain value and
// re-renders an Expr, In or Ident argument. One style per statement: a ? or
// @name beside a $n, or a Named argument, is ErrValidation; a $n off
// PostgreSQL is ErrUnsupported; an out-of-range $n or an unused argument is
// ErrValidation.
func (b *Builder) appendRawDollar(sql string, toks []sqlscan.Tok, pos []any, hasNamed bool) bool {
	has, other := false, false
	for _, tok := range toks {
		switch tok.Kind {
		case sqlscan.Dollar:
			has = true
		case sqlscan.Param, sqlscan.Named:
			other = true
		}
	}
	if !has {
		return false
	}
	if b.d.Name != dialect.Postgres {
		b.SetErr(newError(ErrUnsupported, "build", "", fmt.Errorf("raw SQL %q uses a $n placeholder, which only PostgreSQL supports; write ? instead", sql)))
		return true
	}
	if other || hasNamed {
		b.SetErr(rawErr("raw SQL %q mixes native $n placeholders with ? or @name binds", sql))
		return true
	}
	bound := make(map[int]int, len(pos)) // $n -> bind number of its plain value
	used := make([]bool, len(pos))
	for _, tok := range toks {
		if tok.Kind == sqlscan.Text {
			b.buf = append(b.buf, tok.Text...)
			continue
		}
		n, err := strconv.Atoi(tok.Text)
		if err != nil || n < 1 || n > len(pos) {
			b.SetErr(rawErr("raw SQL %q uses $%s but %d arguments were given", sql, tok.Text, len(pos)))
			return true
		}
		used[n-1] = true
		if k, ok := bound[n]; ok {
			b.buf = b.d.AppendPlaceholder(b.buf, k)
			continue
		}
		b.appendRawArg(pos[n-1])
		if b.err != nil {
			return true
		}
		switch pos[n-1].(type) {
		case Expr, inArg, identArg:
		default:
			bound[n] = len(b.args)
		}
	}
	for i, u := range used {
		if !u {
			b.SetErr(rawErr("raw SQL %q never uses argument $%d", sql, i+1))
			return true
		}
	}
	return true
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
	if m := b.d.MaxInList; m > 0 && rv.Len() > m {
		b.SetErr(&Error{Kind: ErrLimitExceeded, Op: "build", Err: fmt.Errorf("IN list has %d values, over the limit of %d (WithMaxInList)", rv.Len(), m)})
		return
	}
	if m := b.d.MaxParams; m > 0 && len(b.args)+rv.Len() > m {
		b.SetErr(rawErr("%d bind parameters exceed the %s limit of %d", len(b.args)+rv.Len(), b.d.Name, m))
		return
	}
	for i := 0; i < rv.Len(); i++ {
		switch rv.Index(i).Interface().(type) {
		case identArg, inArg, NamedArg, Expr:
			b.SetErr(rawErr("In element %d is a %T; elements must be plain values", i, rv.Index(i).Interface()))
			return
		}
	}
	for i := 0; i < rv.Len(); i++ {
		if i > 0 {
			b.buf = append(b.buf, ", "...)
		}
		b.Arg(rv.Index(i).Interface())
	}
}
