// SPDX-License-Identifier: Apache-2.0

package timeop

import (
	"math"
	"strconv"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/types"
)

// DateExpr is a types.Date-valued expression built by AddDays or
// AddDaysExpr. It carries the comparisons of a date column (Eq, Lt, LeExpr,
// ...) and can be aliased with As.
type DateExpr struct {
	momo.Ops[types.Date]
	fn func(*momo.Builder)
}

func newDateExpr(fn func(*momo.Builder)) DateExpr {
	x := DateExpr{fn: fn}
	x.Ops = momo.NewOps[types.Date](x)
	return x
}

// As names the expression in a select list.
func (x DateExpr) As(alias string) momo.Aliased[types.Date] {
	return momo.As[types.Date](x, alias)
}

// AppendSQL renders the expression for the dialect of b.
func (x DateExpr) AppendSQL(b *momo.Builder) {
	if x.fn == nil {
		b.SetErr(timeErr("expression was not built with its constructor"))
		return
	}
	x.fn(b)
}

// DaysExpr is a whole number of days as an int32 expression, built by
// DaysBetween. It carries the comparisons of an int32 column and can be
// aliased with As.
type DaysExpr struct {
	momo.Ops[int32]
	fn func(*momo.Builder)
}

func newDaysExpr(fn func(*momo.Builder)) DaysExpr {
	x := DaysExpr{fn: fn}
	x.Ops = momo.NewOps[int32](x)
	return x
}

// As names the expression in a select list.
func (x DaysExpr) As(alias string) momo.Aliased[int32] {
	return momo.As[int32](x, alias)
}

// AppendSQL renders the expression for the dialect of b.
func (x DaysExpr) AppendSQL(b *momo.Builder) {
	if x.fn == nil {
		b.SetErr(timeErr("expression was not built with its constructor"))
		return
	}
	x.fn(b)
}

// appendDate renders a date operand. On PostgreSQL a bound date is cast to
// date, because the server cannot type a bare parameter next to + or -.
func appendDate(b *momo.Builder, e momo.Expression[types.Date]) {
	if b.Dialect().Name == dialect.Postgres && momo.IsBound(e) {
		b.WriteString("CAST(")
		e.AppendSQL(b)
		b.WriteString(" AS date)")
		return
	}
	e.AppendSQL(b)
}

// checkFinite fails the build when a bound date is infinity or -infinity on a
// dialect without special dates, where the database would turn it into NULL
// or a zero date instead of an error.
func checkFinite(b *momo.Builder, e momo.Expression[types.Date]) {
	if b.Dialect().Name == dialect.Postgres || !momo.IsBound(e) {
		return
	}
	probe := momo.NewBuilder(b.Dialect())
	e.AppendSQL(probe)
	for _, a := range probe.Args() {
		if d, ok := a.(types.Date); ok && !d.IsFinite() {
			b.SetErr(timeErr("a date of infinity or -infinity cannot be used in date arithmetic on %s", b.Dialect().Name))
			return
		}
	}
}

// AddDays is d + n: the date n days later, or earlier for a negative n. n is
// bound as a parameter on PostgreSQL and MySQL and written into the SQL text
// on SQLite. A count outside the int32 range fails the build with
// momo.ErrValidation.
//
// It renders (d + n) on PostgreSQL, DATE(DATE_ADD(d, INTERVAL n DAY)) on MySQL
// and MariaDB and date(d, '+n days') on SQLite, which stores a types.Date as 'YYYY-MM-DD'
// text. A month end or a leap day follows the calendar on every dialect:
// 2028-02-28 plus 1 is 2028-02-29.
//
// Infinity: PostgreSQL keeps 'infinity' and '-infinity' as they are, whatever
// n is. SQLite and MySQL have no special dates, so a bound types.Date with
// Infinity or NegInfinity set fails the build with momo.ErrValidation. A NULL
// date gives NULL.
func AddDays(d momo.Expression[types.Date], n int) DateExpr {
	if int64(n) < math.MinInt32 || int64(n) > math.MaxInt32 {
		return newDateExpr(func(b *momo.Builder) {
			b.SetErr(timeErr("AddDays count %d is outside the int32 range", n))
		})
	}
	lit := int32(n)
	return addDays(d, momo.Value(lit), &lit)
}

// AddDaysExpr is d + n for a count that is an expression: a bound
// momo.Value(int32(3)), or a column read with momo.Typed[int32](col). See
// AddDays for the rendering and for infinity. A NULL count gives NULL. On
// PostgreSQL a count read from a column must be an int4 (integer) column:
// momo.Typed[int32] over a bigint column fails at run time with "operator does
// not exist: date + bigint".
func AddDaysExpr(d momo.Expression[types.Date], n momo.Expression[int32]) DateExpr {
	return addDays(d, n, nil)
}

func addDays(d momo.Expression[types.Date], n momo.Expression[int32], lit *int32) DateExpr {
	return newDateExpr(func(b *momo.Builder) {
		if d == nil || n == nil {
			b.SetErr(timeErr("AddDays needs a date expression and a count"))
			return
		}
		checkFinite(b, d)
		switch b.Dialect().Name {
		case dialect.Postgres:
			b.WriteString("(")
			appendDate(b, d)
			b.WriteString(" + ")
			if momo.IsBound(n) {
				// An untyped parameter next to a date is ambiguous between
				// integer and interval.
				b.WriteString("CAST(")
				n.AppendSQL(b)
				b.WriteString(" AS integer))")
				return
			}
			n.AppendSQL(b)
			b.WriteString(")")
		case dialect.MySQL:
			// DATE() keeps the result a plain 'YYYY-MM-DD': DATE_ADD on a text
			// operand can return a DATETIME string, which types.Date rejects.
			b.WriteString("DATE(DATE_ADD(")
			d.AppendSQL(b)
			b.WriteString(", INTERVAL ")
			n.AppendSQL(b)
			b.WriteString(" DAY))")
		case dialect.SQLite:
			b.WriteString("date(")
			d.AppendSQL(b)
			b.WriteString(", ")
			if lit != nil {
				// %+d gives -3 and +3, never +-3.
				sign := "+"
				if *lit < 0 {
					sign = ""
				}
				b.WriteString("'" + sign + strconv.FormatInt(int64(*lit), 10) + " days')")
				return
			}
			b.WriteString("CAST(")
			n.AppendSQL(b)
			b.WriteString(" AS INTEGER) || ' days')")
		default:
			b.Unsupported("AddDays")
		}
	})
}

// DaysBetween is a - b in whole days: positive when a is later than b,
// negative when it is earlier. It renders (a - b) on PostgreSQL,
// DATEDIFF(a, b) on MySQL and CAST(julianday(a) - julianday(b) AS INTEGER) on
// SQLite. Dates are whole days, so the julianday difference is an exact
// integer and the cast loses nothing, for a negative result too.
//
// Infinity: PostgreSQL refuses a subtraction with an infinite date at run
// time (cannot subtract infinite dates), so the query fails with a database
// error. SQLite and MySQL have no special dates, so a bound types.Date with
// Infinity or NegInfinity set fails the build with momo.ErrValidation.
func DaysBetween(a, b momo.Expression[types.Date]) DaysExpr {
	return newDaysExpr(func(bld *momo.Builder) {
		if a == nil || b == nil {
			bld.SetErr(timeErr("DaysBetween needs two date expressions"))
			return
		}
		checkFinite(bld, a)
		checkFinite(bld, b)
		switch bld.Dialect().Name {
		case dialect.Postgres:
			bld.WriteString("(")
			appendDate(bld, a)
			bld.WriteString(" - ")
			appendDate(bld, b)
			bld.WriteString(")")
		case dialect.MySQL:
			bld.WriteString("DATEDIFF(")
			a.AppendSQL(bld)
			bld.WriteString(", ")
			b.AppendSQL(bld)
			bld.WriteString(")")
		case dialect.SQLite:
			bld.WriteString("CAST(julianday(")
			a.AppendSQL(bld)
			bld.WriteString(") - julianday(")
			b.AppendSQL(bld)
			bld.WriteString(") AS INTEGER)")
		default:
			bld.Unsupported("DaysBetween")
		}
	})
}
