// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vector"
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
	// conflictTarget is set while rendering an ON CONFLICT target predicate.
	// Excluded references are invalid there, and SQLite needs literal values
	// inline because it matches the predicate against the index text.
	conflictTarget bool
	// scopes tracks, per SELECT being rendered, which table qualifiers its
	// FROM and JOIN sources expose and which qualifiers its columns used.
	scopes []*qscope
}

// qscope is one SELECT's source names and the qualifiers referenced inside it.
type qscope struct {
	names map[string]struct{}
	refs  map[string]struct{}
}

// withScope renders fn inside a scope whose sources expose names. Every
// statement builder (SELECT, and UPDATE, DELETE and MERGE with their target
// table) must render through it, so a column qualified by a table that is
// not among the sources fails the build with ErrValidation. A qualifier the
// scope does not provide is passed to the enclosing scope, which is what lets
// a correlated subquery reference its outer statement's target table.
func (b *Builder) withScope(names []string, fn func()) {
	b.pushScope(names)
	defer b.popScope()
	fn()
}

// pushScope opens a scope for a statement whose sources expose names.
func (b *Builder) pushScope(names []string) {
	sc := &qscope{names: make(map[string]struct{}, len(names)), refs: map[string]struct{}{}}
	for _, n := range names {
		if n != "" {
			sc.names[n] = struct{}{}
		}
	}
	b.scopes = append(b.scopes, sc)
}

// popScope closes the innermost scope. A qualifier it does not provide is
// handed to the enclosing SELECT (a correlated reference); with no
// enclosing SELECT it is a column of a table that is not in the query.
func (b *Builder) popScope() {
	if len(b.scopes) == 0 {
		b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: errors.New("internal error: scope stack underflow: popScope without a matching pushScope")})
		return
	}
	sc := b.scopes[len(b.scopes)-1]
	b.scopes = b.scopes[:len(b.scopes)-1]
	for q := range sc.refs {
		if _, ok := sc.names[q]; ok {
			continue
		}
		if n := len(b.scopes); n > 0 {
			b.scopes[n-1].refs[q] = struct{}{}
			continue
		}
		b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: fmt.Errorf("column reference to table %q, which is not in the query's FROM or JOIN sources", q)})
	}
}

// NewBuilder returns a builder for the dialect.
func NewBuilder(d *dialect.Dialect) *Builder { return &Builder{d: d} }

// Dialect returns the dialect being rendered.
func (b *Builder) Dialect() *dialect.Dialect { return b.d }

// WriteString appends trusted SQL text.
func (b *Builder) WriteString(s string) { b.buf = append(b.buf, s...) }

// WriteByte appends a trusted SQL byte. It always returns nil; the error
// result exists only so Builder satisfies io.ByteWriter.
func (b *Builder) WriteByte(c byte) error { b.buf = append(b.buf, c); return nil }

// writeByte appends a trusted SQL byte; internal renderers use it so no
// always-nil error is discarded.
func (b *Builder) writeByte(c byte) { b.buf = append(b.buf, c) }

// Ident appends a quoted identifier.
func (b *Builder) Ident(name string) { b.buf = b.d.AppendIdent(b.buf, name) }

// QualifiedIdent appends qualifier.name, both quoted. An empty qualifier
// (or an unqualified context) renders the bare quoted name.
func (b *Builder) QualifiedIdent(qualifier, name string) {
	if qualifier != "" && !b.unqualified {
		if n := len(b.scopes); n > 0 {
			b.scopes[n-1].refs[qualifier] = struct{}{}
		}
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
			if errors.Is(err, vector.ErrDialect) {
				err = &Error{Kind: ErrUnsupported, Err: err}
			}
			b.SetErr(err)
			return
		}
		v = enc
	}
	if b.conflictTarget && b.d.Name == dialect.SQLite {
		lit, err := sqliteLiteral(v)
		if err != nil {
			b.SetErr(err)
			return
		}
		b.WriteString(lit)
		return
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

// checkBindCap reports whether n more bind parameters fit under the dialect's
// limit. When they do not it records an ErrValidation build error, so the
// caller gets a typed error at build time instead of a driver failure. Inline
// literal rendering (SQLite conflict targets) binds nothing and always fits.
func (b *Builder) checkBindCap(n int, what string) bool {
	m := b.d.MaxParams
	if m <= 0 || b.conflictTarget || len(b.args)+n <= m {
		return true
	}
	b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: fmt.Errorf("%s needs %d bind parameters, over the %s limit of %d; chunk the list", what, len(b.args)+n, b.d.Name, m)})
	return false
}

// unsupported records an ErrUnsupported build error for feature.
func (b *Builder) unsupported(feature string) {
	b.SetErr(&Error{Kind: ErrUnsupported, Op: "build", Err: fmt.Errorf("%s is not supported by %s", feature, b.d.Name)})
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

// sqliteLiteral renders the value that would be bound (after DialectValuer
// and driver.Valuer) as an SQL literal: bool, integer, finite float or string
// without a NUL byte. SQLite matches an ON CONFLICT target predicate against
// the index text, so a bound value never matches; any other value fails with
// ErrValidation instead of silently reproducing that failure.
func sqliteLiteral(v any) (string, error) {
	orig := v
	if dv, ok := v.(driver.Valuer); ok {
		if rv := reflect.ValueOf(v); rv.Kind() == reflect.Pointer && rv.IsNil() {
			return "", sqliteLiteralErr(orig, "is NULL")
		}
		enc, err := dv.Value()
		if err != nil {
			return "", &Error{Kind: ErrValidation, Op: "insert", Err: fmt.Errorf("ON CONFLICT target predicate value of type %T on SQLite: %w", orig, err)}
		}
		v = enc
	}
	reason := "has an unsupported type"
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Invalid:
		reason = "is NULL"
	case reflect.Bool:
		if rv.Bool() {
			return "1", nil
		}
		return "0", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(rv.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			reason = "is NaN or Inf"
			break
		}
		bits := 64
		if rv.Kind() == reflect.Float32 {
			bits = 32
		}
		// Plain decimal in the range people write index literals in; SQLite
		// compares literal tokens as text, so 1e+06 would not match 1000000.0.
		format := byte('g')
		if a := math.Abs(f); a == 0 || (a >= 1e-6 && a < 1e21) {
			format = 'f'
		}
		out := strconv.FormatFloat(f, format, -1, bits)
		if !strings.ContainsAny(out, ".e") {
			out += ".0" // keep it a real, as in the index text
		}
		return out, nil
	case reflect.String:
		str := rv.String()
		if strings.IndexByte(str, 0) < 0 {
			return "'" + strings.ReplaceAll(str, "'", "''") + "'", nil
		}
		reason = "contains a NUL byte"
	}
	return "", sqliteLiteralErr(orig, reason)
}

func sqliteLiteralErr(v any, reason string) error {
	return &Error{Kind: ErrValidation, Op: "insert", Err: fmt.Errorf("value of type %T %s and cannot be rendered inline in an ON CONFLICT target predicate on SQLite; use momo.SQL", v, reason)}
}
