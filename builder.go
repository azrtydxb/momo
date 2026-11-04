// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/dialect"
)

// Builder accumulates SQL text and bind arguments for one statement.
//
// Identifiers go through Ident (always quoted); values go through Arg
// (always bound as parameters). Raw text is only written by Momo itself or
// by the explicit raw-SQL escape hatches (SQL, Raw), whose safety contract
// is documented on those functions.
type Builder struct {
	d    *dialect.Dialect
	buf  []byte
	args []any
	err  error
	// unqualified renders columns without their table qualifier. It is
	// set while rendering SET targets, INSERT column lists, ON CONFLICT
	// targets and RETURNING lists.
	unqualified bool
}

// NewBuilder returns a builder for the dialect.
func NewBuilder(d *dialect.Dialect) *Builder { return &Builder{d: d} }

// Dialect returns the dialect being rendered.
func (b *Builder) Dialect() *dialect.Dialect { return b.d }

// WriteString appends trusted SQL text.
func (b *Builder) WriteString(s string) { b.buf = append(b.buf, s...) }

// WriteByte appends a trusted SQL byte.
func (b *Builder) WriteByte(c byte) error { b.buf = append(b.buf, c); return nil }

// Ident appends a quoted identifier.
func (b *Builder) Ident(name string) { b.buf = b.d.AppendIdent(b.buf, name) }

// QualifiedIdent appends qualifier.name, both quoted. An empty qualifier
// (or an unqualified context) renders the bare quoted name.
func (b *Builder) QualifiedIdent(qualifier, name string) {
	if qualifier != "" && !b.unqualified {
		b.Ident(qualifier)
		b.buf = append(b.buf, '.')
	}
	b.Ident(name)
}

// DialectValuer is implemented by values whose wire encoding depends on the
// dialect, such as vectors (pgvector text vs. sqlite-vec float32 blobs).
type DialectValuer interface {
	DialectValue(d dialect.Name) (any, error)
}

// Arg appends a bind placeholder and records v as its argument.
func (b *Builder) Arg(v any) {
	if dv, ok := v.(DialectValuer); ok {
		enc, err := dv.DialectValue(b.d.Name)
		if err != nil {
			b.SetErr(err)
			return
		}
		v = enc
	}
	b.args = append(b.args, v)
	b.buf = b.d.AppendPlaceholder(b.buf, len(b.args))
}

// SetErr records the first build error; rendering continues but the
// statement will not be executed.
func (b *Builder) SetErr(err error) {
	if b.err == nil {
		b.err = err
	}
}

// Err returns the first build error.
func (b *Builder) Err() error { return b.err }

// String returns the SQL text built so far.
func (b *Builder) String() string { return string(b.buf) }

// Args returns the bind arguments built so far.
func (b *Builder) Args() []any { return b.args }

// unsupported records an ErrUnsupported build error for feature.
func (b *Builder) unsupported(feature string) {
	b.SetErr(&Error{Kind: ErrUnsupported, Op: "build", Err: fmt.Errorf("%s is not supported by %s", feature, b.d.Name)})
}

// appendRaw renders a raw fragment, replacing each '?' with a bind
// placeholder for the corresponding argument. "??" renders a literal '?'
// (for PostgreSQL's jsonb ? operators). Argument count must match.
func (b *Builder) appendRaw(sql string, args []any) {
	n := 0
	for i := 0; i < len(sql); i++ {
		c := sql[i]
		if c != '?' {
			b.buf = append(b.buf, c)
			continue
		}
		if i+1 < len(sql) && sql[i+1] == '?' {
			b.buf = append(b.buf, '?')
			i++
			continue
		}
		if n >= len(args) {
			b.SetErr(fmt.Errorf("momo: raw SQL %q has more placeholders than the %d arguments given", sql, len(args)))
			return
		}
		if e, ok := args[n].(Expr); ok {
			e.AppendSQL(b)
		} else {
			b.Arg(args[n])
		}
		n++
	}
	if n != len(args) {
		b.SetErr(fmt.Errorf("momo: raw SQL %q has %d placeholders but %d arguments were given", sql, n, len(args)))
	}
}

// Compiled is a rendered statement ready for inspection or execution.
type Compiled struct {
	SQL  string
	Args []any
}

func (c Compiled) String() string {
	if len(c.Args) == 0 {
		return c.SQL
	}
	parts := make([]string, len(c.Args))
	for i := range c.Args {
		parts[i] = "$" + strconv.Itoa(i+1)
	}
	return c.SQL + " -- " + strings.Join(parts, ", ") + " bound"
}

func compile(d *dialect.Dialect, render func(*Builder)) (Compiled, error) {
	if d == nil {
		return Compiled{}, errors.New("momo: no dialect (nil Querier?)")
	}
	b := Builder{d: d}
	render(&b)
	if b.err != nil {
		return Compiled{}, b.err
	}
	return Compiled{SQL: string(b.buf), Args: b.args}, nil
}
