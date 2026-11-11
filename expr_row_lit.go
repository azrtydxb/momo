// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/azrtydxb/momo/dialect"
)

// RowExpr is a row value, (a, b, ...), compared as a whole with another row
// value of the same length. Build it with Row. Comparison is lexicographic:
// (a, b) < (x, y) is a < x OR (a = x AND b < y), which is the keyset
// position check. A NULL element makes the comparison NULL, as in SQL.
type RowExpr struct{ elems []Expr }

// Row is the row value (e1, e2, ...), for example a FIFO position check:
//
//	momo.Row(w.CreatedAt, w.ID).Lt(momo.Row(me.At, momo.Value(id)))
//
// Both sides need at least two elements and the same number of them, or the
// build fails with ErrValidation. A dialect without row values fails with
// ErrUnsupported.
func Row(elems ...Expr) RowExpr { return RowExpr{elems: append([]Expr(nil), elems...)} }

// AppendSQL writes "(e1, e2, ...)" to b.
func (r RowExpr) AppendSQL(b *Builder) {
	b.WriteString("(")
	for i, e := range r.elems {
		if i > 0 {
			b.WriteString(", ")
		}
		e.AppendSQL(b)
	}
	b.WriteString(")")
}

func (RowExpr) resultType() bool { return false }

// Lt is true when the row is lexicographically less than other.
func (r RowExpr) Lt(other RowExpr) Predicate { return r.cmp(" < ", other) }

// Le is true when the row is lexicographically less than or equal to other.
func (r RowExpr) Le(other RowExpr) Predicate { return r.cmp(" <= ", other) }

// Gt is true when the row is lexicographically greater than other.
func (r RowExpr) Gt(other RowExpr) Predicate { return r.cmp(" > ", other) }

// Ge is true when the row is lexicographically greater than or equal to other.
func (r RowExpr) Ge(other RowExpr) Predicate { return r.cmp(" >= ", other) }

func (r RowExpr) cmp(op string, other RowExpr) Predicate {
	return PredicateFunc(func(b *Builder) {
		if len(r.elems) < 2 || len(r.elems) != len(other.elems) {
			b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: fmt.Errorf("row comparison needs two rows of the same length of at least 2, got %d and %d", len(r.elems), len(other.elems))})
			b.WriteString("1 = 0")
			return
		}
		if !b.Dialect().RowValues {
			b.Unsupported("row-value comparison")
			return
		}
		r.AppendSQL(b)
		b.WriteString(op)
		other.AppendSQL(b)
	})
}

// Literal is the constraint of Lit: the types that render safely as an SQL
// literal. A string is accepted, and Lit refuses the characters that would let
// it escape its quotes (see Lit).
type Literal interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64 | ~bool | ~string | time.Time
}

// LitExpr is a value written into the SQL text instead of bound. Build it
// with Lit.
type LitExpr[T Literal] struct {
	Ops[T]
	v T
}

// Lit renders v inline, not as a bound parameter. Use it where two
// occurrences of an expression must be the same text, such as the same
// COALESCE(col, 0) in the select list and in GROUP BY: bound parameters make
// them differ and PostgreSQL rejects the grouping (42803).
//
// It accepts integers, floats, bool, string and time.Time, and the build fails
// with ErrValidation, on every dialect, for a value it cannot render safely:
//   - a string renders as a standard SQL string literal with each ' doubled;
//     a string containing a backslash or a NUL byte is refused, because MySQL
//     (and PostgreSQL with standard_conforming_strings off) read a backslash
//     as an escape inside a literal;
//   - a time.Time renders as DATE 'YYYY-MM-DD' (a quoted date elsewhere) and
//     must be exactly 00:00:00 UTC; a value with a time of day is refused
//     rather than silently truncated;
//   - a NaN or infinite float is refused.
//
// Prefer a bound Value everywhere the text need not match.
func Lit[T Literal](v T) LitExpr[T] {
	l := LitExpr[T]{v: v}
	l.Ops = NewOps[T](l)
	return l
}

// As names the expression in a select list.
func (l LitExpr[T]) As(alias string) Aliased[T] { return As[T](l, alias) }

// AppendSQL writes the literal to b.
func (l LitExpr[T]) AppendSQL(b *Builder) {
	s, err := litText(b.Dialect().Name, any(l.v))
	if err != nil {
		b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: err})
		b.WriteString("NULL")
		return
	}
	b.WriteString(s)
}

func litText(d dialect.Name, v any) (string, error) {
	if t, ok := v.(time.Time); ok {
		u := t.UTC()
		if u.Hour() != 0 || u.Minute() != 0 || u.Second() != 0 || u.Nanosecond() != 0 {
			return "", errors.New("literal time must be exactly 00:00:00 UTC, a date only; bind it with Value to keep the time of day")
		}
		s := u.Format("2006-01-02")
		if d == dialect.Postgres {
			return "DATE '" + s + "'", nil
		}
		return "'" + s + "'", nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Bool:
		if d == dialect.SQLite {
			if rv.Bool() {
				return "1", nil
			}
			return "0", nil
		}
		if rv.Bool() {
			return "TRUE", nil
		}
		return "FALSE", nil
	case reflect.String:
		str := rv.String()
		if strings.ContainsRune(str, '\\') || strings.ContainsRune(str, 0) {
			return "", errors.New("literal string must not contain a backslash or NUL; bind it with Value instead")
		}
		return "'" + strings.ReplaceAll(str, "'", "''") + "'", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return paren(strconv.FormatInt(rv.Int(), 10)), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(rv.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return "", errors.New("literal float must be finite")
		}
		bits := 64
		if rv.Kind() == reflect.Float32 {
			bits = 32
		}
		s := strconv.FormatFloat(f, 'g', -1, bits)
		if !strings.ContainsAny(s, ".e") {
			s += ".0"
		}
		return paren(s), nil
	}
	return "", fmt.Errorf("literal of type %T is not supported", v)
}

// paren wraps a negative number so it cannot fuse with a preceding operator.
func paren(s string) string {
	if strings.HasPrefix(s, "-") {
		return "(" + s + ")"
	}
	return s
}
