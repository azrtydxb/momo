// SPDX-License-Identifier: Apache-2.0

package sqlfn

import (
	"fmt"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/types"
)

// Fractional is the constraint of Floor and Ceil: the Go float kinds and
// types.Numeric. The Go integer kinds are left out because PostgreSQL's
// floor(integer) returns double precision, so the result would not scan back
// into the same Go type; use Cast to a float first. types.Numeric is accepted
// by exact type, not by ~, and only on PostgreSQL (see Floor).
type Fractional interface {
	~float32 | ~float64 | types.Numeric
}

// RoundedExpr is FLOOR or CEIL of an expression of the Go type T. Build it
// with Floor or Ceil.
type RoundedExpr[T Fractional] struct {
	momo.Ops[T]
	e    momo.Expr
	ceil bool
}

func rounded[T Fractional](e momo.Expression[T], ceil bool) RoundedExpr[T] {
	r := RoundedExpr[T]{e: e, ceil: ceil}
	r.Ops = momo.NewOps[T](r)
	return r
}

// Floor is the largest whole number not above e, with the type of e: a float
// stays a float (floor(2.7) is 2, scanned as 2.0) and a types.Numeric stays a
// types.Numeric. NULL gives NULL. It renders floor(e) on PostgreSQL, MySQL and
// MariaDB.
//
// SQLite has floor() only when it is built with the math functions, which the
// mattn driver omits without its sqlite_math_functions build tag, so Momo does
// not call it: it renders a CASE over CAST(e AS INTEGER) that gives the same
// result on every SQLite build and repeats e a few times. A float with an
// absolute value of 2^52 or more is already whole and is returned as it is.
//
// types.Numeric is PostgreSQL only: SQLite and MySQL store it as TEXT, so the
// function would run through floating point and lose the exactness the type
// exists for; building such an expression there fails with momo.ErrUnsupported.
func Floor[T Fractional](e momo.Expression[T]) RoundedExpr[T] { return rounded(e, false) }

// Ceil is the smallest whole number not below e: the mirror of Floor, with the
// same types, NULL handling and dialect rules (ceil on PostgreSQL, MySQL and
// MariaDB; a CAST based CASE on SQLite).
func Ceil[T Fractional](e momo.Expression[T]) RoundedExpr[T] { return rounded(e, true) }

// As names the expression in a select list.
func (r RoundedExpr[T]) As(alias string) momo.Aliased[T] { return momo.As[T](r, alias) }

// AppendSQL writes the call to b.
func (r RoundedExpr[T]) AppendSQL(b *momo.Builder) {
	name := b.Dialect().Name
	fn := "floor"
	if r.ceil {
		fn = "ceil"
	}
	if isNumericType[T]() && name != dialect.Postgres {
		b.SetErr(&momo.Error{Kind: momo.ErrUnsupported, Op: "build", Err: fmt.Errorf(
			"%s on types.Numeric is PostgreSQL only: %s stores types.Numeric as TEXT, so the function would be floating point and inexact",
			fn, name)})
		b.WriteString("NULL")
		return
	}
	switch name {
	case dialect.Postgres, dialect.MySQL:
		b.WriteString(fn + "(")
		r.e.AppendSQL(b)
		b.WriteString(")")
	case dialect.SQLite:
		r.appendSQLite(b)
	default:
		b.Unsupported(fn)
	}
}

// appendSQLite writes floor/ceil without the SQLite math functions. CAST AS
// INTEGER truncates toward zero, so the floor is that minus one when e is below
// it, and the ceiling is that plus one when e is above it. Floats from 2^52 are
// whole already, which also keeps the cast away from the int64 limits.
func (r RoundedExpr[T]) appendSQLite(b *momo.Builder) {
	cmpOp, adj := " < ", " - "
	if r.ceil {
		cmpOp, adj = " > ", " + "
	}
	operand := func() {
		b.WriteString("(")
		r.e.AppendSQL(b)
		b.WriteString(")")
	}
	b.WriteString("CASE WHEN abs(")
	operand()
	b.WriteString(") >= 4503599627370496.0 THEN ")
	operand()
	b.WriteString(" ELSE CAST(CAST(")
	operand()
	b.WriteString(" AS INTEGER)" + adj + "(")
	operand()
	b.WriteString(cmpOp + "CAST(")
	operand()
	b.WriteString(" AS INTEGER)) AS REAL) END")
}
