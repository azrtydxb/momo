// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/types"
)

// Integer is the constraint of SumInt: the signed integer column types.
type Integer interface{ ~int16 | ~int32 | ~int64 }

// Averageable is the constraint of Avg: an integer, a float or a
// types.Numeric column.
type Averageable interface {
	~int16 | ~int32 | ~int64 | ~float32 | ~float64 | types.Numeric
}

// Exact is the constraint of SumNumeric and AvgNumeric: the column types whose
// aggregate the database can compute exactly, integers and types.Numeric.
type Exact interface {
	~int16 | ~int32 | ~int64 | types.Numeric
}

// SumInt is SUM(expr) over an int16, int32 or int64 column with an int64
// result, whatever the column width. It is NULL over zero rows: scan it into
// *int64 (or sql.NullInt64), or wrap it in Coalesce.
//
// PostgreSQL returns bigint for SUM of smallint and integer but numeric for
// SUM of bigint, so Momo writes CAST(SUM(x) AS bigint) there. MySQL, MariaDB
// and SQLite need no cast: MySQL returns an integral DECIMAL, which
// database/sql reads into an int64, and SQLite returns an integer. A sum that
// does not fit an int64 is an error on each of them (PostgreSQL "bigint out of
// range", SQLite "integer overflow", MySQL a scan error); use SumNumeric when
// it can.
func SumInt[T Integer](e Expression[T]) Agg[int64] {
	return newFnAgg[int64](fn{name: "SUM", args: []Expr{e}, pgCast: "bigint"})
}

// SumNumeric is SUM(expr) as an exact types.Numeric over an integer or a
// types.Numeric column. It cannot overflow and loses no digits. It is NULL
// over zero rows: scan it into *types.Numeric, or wrap it in Coalesce.
//
// PostgreSQL sums as numeric (an integer argument is cast to numeric first,
// so even SUM over bigint cannot overflow). MySQL and MariaDB sum an integer
// column as an integral DECIMAL. A types.Numeric column is TEXT there, which
// SUM would read as a DOUBLE, so Momo casts it to DECIMAL(65,30): the result
// has 30 decimal places (trailing zeros included) and at most 35 integer
// digits, and an input with more than 30 decimals is rounded. SQLite has no
// exact decimal sum: over an integer column it returns an exact integer (an
// overflow is an error), but over a types.Numeric column, which is TEXT, it
// would return a floating point value, so that case fails the build with
// ErrUnsupported.
func SumNumeric[T Exact](e Expression[T]) Agg[types.Numeric] {
	return newFnAgg[types.Numeric](fn{name: "SUM", args: []Expr{exactArg[T]{e: e, sqlite: true}}})
}

// AvgNumeric is AVG(expr) as an exact types.Numeric over an integer or a
// types.Numeric column. It is NULL over zero rows: scan it into
// *types.Numeric, or wrap it in Coalesce. PostgreSQL returns numeric with
// scale of at least 16 digits and MySQL and MariaDB a DECIMAL with four
// digits more scale than the argument (a types.Numeric column is cast to
// DECIMAL(65,30) first, see SumNumeric). SQLite's AVG is always a floating
// point value, so AvgNumeric fails the build there with ErrUnsupported; use
// Avg.
func AvgNumeric[T Exact](e Expression[T]) Agg[types.Numeric] {
	return newFnAgg[types.Numeric](fn{name: "AVG", args: []Expr{exactArg[T]{e: e}}})
}

// exactArg renders the argument of an exact aggregate for the dialect: a cast
// that makes the database compute in numeric or DECIMAL, or ErrUnsupported
// where it cannot be exact.
type exactArg[T Exact] struct {
	e Expr
	// sqlite reports that SQLite can aggregate an integer argument exactly.
	sqlite bool
}

func (a exactArg[T]) AppendSQL(b *Builder) {
	_, fromText := any(*new(T)).(types.Numeric)
	switch b.d.Name {
	case dialect.Postgres:
		if fromText {
			a.e.AppendSQL(b)
			return
		}
		b.WriteString("CAST(")
		a.e.AppendSQL(b)
		b.WriteString(" AS numeric)")
	case dialect.MySQL:
		if !fromText {
			a.e.AppendSQL(b)
			return
		}
		b.WriteString("CAST(")
		a.e.AppendSQL(b)
		b.WriteString(" AS DECIMAL(65,30))")
	default:
		if fromText || !a.sqlite {
			b.unsupported("exact aggregate over this argument")
			return
		}
		a.e.AppendSQL(b)
	}
}
