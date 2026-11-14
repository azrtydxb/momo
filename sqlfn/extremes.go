// SPDX-License-Identifier: Apache-2.0

package sqlfn

import (
	"cmp"
	"fmt"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/types"
)

// Ordered is the constraint of Greatest and Least: the Go integer, float and
// string kinds, time.Time, types.Date and types.Numeric. types.Numeric is
// accepted by exact type, not by ~, and only on PostgreSQL: SQLite and MySQL
// store it as TEXT, which would order lexically ("10" before "9"), so building
// such an expression there fails with momo.ErrUnsupported.
type Ordered interface {
	cmp.Ordered | time.Time | types.Date | types.Numeric
}

// ExtremeExpr is GREATEST or LEAST over two or more expressions of the Go type
// T. Build it with Greatest or Least.
type ExtremeExpr[T Ordered] struct {
	momo.Ops[T]
	least bool
	args  []momo.Expr
}

func extreme[T Ordered](least bool, a, b momo.Expression[T], more []momo.Expression[T]) ExtremeExpr[T] {
	args := make([]momo.Expr, 0, 2+len(more))
	args = append(args, a, b)
	for _, m := range more {
		args = append(args, m)
	}
	e := ExtremeExpr[T]{least: least, args: args}
	e.Ops = momo.NewOps[T](e)
	return e
}

// Greatest is the largest of its arguments, ignoring NULL, on every dialect.
// The result is NULL only when every argument is NULL.
//
// That is PostgreSQL's GREATEST. MySQL, MariaDB and SQLite return NULL when any
// argument is NULL, so there the call is rewritten to keep the PostgreSQL
// result: each argument x becomes COALESCE(x, the other arguments), and the
// largest of those is taken (GREATEST on MySQL and MariaDB, the scalar max()
// on SQLite). The rewrite repeats every argument once per argument, so
// n arguments render n*n operands; keep the list short and do not pass a
// non-deterministic expression such as random().
//
// Arguments are expressions of the same type T; wrap a Go value with
// momo.Value or momo.Lit. Strings compare under the collation of the dialect
// (MySQL's default is case-insensitive, SQLite compares bytes). A
// types.Numeric is PostgreSQL only (see Ordered).
func Greatest[T Ordered](a, b momo.Expression[T], more ...momo.Expression[T]) ExtremeExpr[T] {
	return extreme(false, a, b, more)
}

// Least is the smallest of its arguments, ignoring NULL, on every dialect: the
// mirror of Greatest, with the same NULL handling and rewrite (LEAST on
// PostgreSQL, MySQL and MariaDB, the scalar min() on SQLite).
func Least[T Ordered](a, b momo.Expression[T], more ...momo.Expression[T]) ExtremeExpr[T] {
	return extreme(true, a, b, more)
}

// As names the expression in a select list.
func (e ExtremeExpr[T]) As(alias string) momo.Aliased[T] { return momo.As[T](e, alias) }

// AppendSQL writes the call to b.
func (e ExtremeExpr[T]) AppendSQL(b *momo.Builder) {
	name := b.Dialect().Name
	fn := "GREATEST"
	if e.least {
		fn = "LEAST"
	}
	if isNumericType[T]() && name != dialect.Postgres {
		b.SetErr(&momo.Error{Kind: momo.ErrUnsupported, Op: "build", Err: fmt.Errorf(
			"%s on types.Numeric is PostgreSQL only: %s stores types.Numeric as TEXT, which orders lexically",
			fn, name)})
		b.WriteString("NULL")
		return
	}
	switch name {
	case dialect.Postgres:
		b.WriteString(fn + "(")
		for i, a := range e.args {
			if i > 0 {
				b.WriteString(", ")
			}
			a.AppendSQL(b)
		}
		b.WriteString(")")
	case dialect.MySQL, dialect.SQLite:
		if name == dialect.SQLite {
			fn = "max"
			if e.least {
				fn = "min"
			}
		}
		b.WriteString(fn + "(")
		for i := range e.args {
			if i > 0 {
				b.WriteString(", ")
			}
			// COALESCE(x_i, the others) is non-NULL whenever any argument is.
			b.WriteString("COALESCE(")
			e.args[i].AppendSQL(b)
			for j, o := range e.args {
				if j != i {
					b.WriteString(", ")
					o.AppendSQL(b)
				}
			}
			b.WriteString(")")
		}
		b.WriteString(")")
	default:
		b.Unsupported(fn)
	}
}

// isNumericType reports whether T is types.Numeric.
func isNumericType[T any]() bool {
	_, ok := any(*new(T)).(types.Numeric)
	return ok
}
