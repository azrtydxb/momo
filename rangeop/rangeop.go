// SPDX-License-Identifier: Apache-2.0

// Package rangeop holds the PostgreSQL range predicates and accessors for a
// column of a types.Range: containment of an element or of a range, overlap,
// adjacency, strictly-left and strictly-right, the lower and upper bound, and
// the emptiness and infinity tests. Each binds its operand as one parameter cast
// to the range or element type, so the server never has to choose between the
// element and the range form of @>, and a range predicate is never built by
// concatenating text.
//
// Range types exist on PostgreSQL alone, so every builder fails the build with
// momo.ErrUnsupported on another dialect.
//
// The package builds on the frozen extension API of the root package plus
// internal/exprkit, as arrayop does.
package rangeop

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprkit"
	"github.com/azrtydxb/momo/types"
)

// elemSQL is the PostgreSQL type of an element of T.
func elemSQL[T types.RangeElem]() string {
	var z T
	switch any(z).(type) {
	case int32:
		return "integer"
	case int64:
		return "bigint"
	case types.Numeric:
		return "numeric"
	case types.Date:
		return "date"
	case types.Timestamp:
		return "timestamp"
	}
	return "timestamptz"
}

// rangeSQL is the PostgreSQL range type of T.
func rangeSQL[T types.RangeElem]() string {
	return types.Range[T]{}.SQLType(dialect.Postgres)
}

// binary returns the predicate left op $n::cast. The left operand renders first
// so that a sensitive column marks the flags that the bind then carries: the
// values compared with it stay masked in hook events. The cast gives the
// parameter its type, which the server cannot infer where an operator has both
// an element and a range form.
func binary(feature string, left momo.Expr, op string, arg any, cast string) momo.Predicate {
	return exprkit.Predicate(func(b *momo.Builder) {
		if b.Dialect().Name != dialect.Postgres {
			exprkit.Unsupported(b, feature)
			return
		}
		fl := exprkit.RenderSensitive(b, left, exprkit.FlagsOf(left))
		b.WriteString(op)
		exprkit.ArgFor(b, fl, arg)
		b.WriteString("::" + cast)
	})
}

// Contains is true when the range column contains the element v (PostgreSQL @>
// with an element). An empty range contains nothing, and an unbounded side
// contains everything on that side.
func Contains[T types.RangeElem](col momo.Expression[types.Range[T]], v T) momo.Predicate {
	return binary("range contains element (@>)", col, " @> ", v, elemSQL[T]())
}

// ContainsRange is true when the range column contains every point of other
// (PostgreSQL @> with a range). Every range contains the empty range.
func ContainsRange[T types.RangeElem](col momo.Expression[types.Range[T]], other types.Range[T]) momo.Predicate {
	return binary("range contains range (@>)", col, " @> ", other, rangeSQL[T]())
}

// ContainedBy is true when every point of the range column is in other
// (PostgreSQL <@). The empty range is contained by every range.
func ContainedBy[T types.RangeElem](col momo.Expression[types.Range[T]], other types.Range[T]) momo.Predicate {
	return binary("range contained by (<@)", col, " <@ ", other, rangeSQL[T]())
}

// Overlaps is true when the range column and other share a point (PostgreSQL
// &&). An empty range overlaps nothing.
func Overlaps[T types.RangeElem](col momo.Expression[types.Range[T]], other types.Range[T]) momo.Predicate {
	return binary("range overlap (&&)", col, " && ", other, rangeSQL[T]())
}

// Adjacent is true when the range column and other share no point and no
// point lies between them, as [1,5) and [5,9) do (PostgreSQL -|-). Ranges that
// overlap are not adjacent.
func Adjacent[T types.RangeElem](col momo.Expression[types.Range[T]], other types.Range[T]) momo.Predicate {
	return binary("range adjacent (-|-)", col, " -|- ", other, rangeSQL[T]())
}

// StrictlyLeft is true when every point of the range column is below every
// point of other (PostgreSQL <<).
func StrictlyLeft[T types.RangeElem](col momo.Expression[types.Range[T]], other types.Range[T]) momo.Predicate {
	return binary("range strictly left (<<)", col, " << ", other, rangeSQL[T]())
}

// StrictlyRight is true when every point of the range column is above every
// point of other (PostgreSQL >>).
func StrictlyRight[T types.RangeElem](col momo.Expression[types.Range[T]], other types.Range[T]) momo.Predicate {
	return binary("range strictly right (>>)", col, " >> ", other, rangeSQL[T]())
}

// unary returns the predicate fn(col).
func unary[T types.RangeElem](feature, fn string, col momo.Expression[types.Range[T]]) momo.Predicate {
	return exprkit.Predicate(func(b *momo.Builder) {
		if b.Dialect().Name != dialect.Postgres {
			exprkit.Unsupported(b, feature)
			return
		}
		b.WriteString(fn + "(")
		exprkit.RenderSensitive(b, col, exprkit.FlagsOf(col))
		b.WriteString(")")
	})
}

// IsEmpty is true when the range column holds the empty range (PostgreSQL
// isempty).
func IsEmpty[T types.RangeElem](col momo.Expression[types.Range[T]]) momo.Predicate {
	return unary("range emptiness (isempty)", "isempty", col)
}

// LowerInfinite is true when the range column has no lower limit (PostgreSQL
// lower_inf). The empty range has none either, so it is false for it.
func LowerInfinite[T types.RangeElem](col momo.Expression[types.Range[T]]) momo.Predicate {
	return unary("range lower_inf", "lower_inf", col)
}

// UpperInfinite is true when the range column has no upper limit (PostgreSQL
// upper_inf). It is false for the empty range.
func UpperInfinite[T types.RangeElem](col momo.Expression[types.Range[T]]) momo.Predicate {
	return unary("range upper_inf", "upper_inf", col)
}

// BoundExpr is the lower or upper bound of a range column, an
// Expression[T] with the comparison and ordering methods of any scalar
// expression. It is NULL for the empty range and for an unbounded side. Build
// it with Lower or Upper.
type BoundExpr[T types.RangeElem] struct {
	momo.Ops[T]
	src     momo.Expr
	fn      string
	feature string
}

func newBound[T types.RangeElem](feature, fn string, col momo.Expression[types.Range[T]]) BoundExpr[T] {
	e := BoundExpr[T]{src: col, fn: fn, feature: feature}
	e.Ops = momo.NewOps[T](e)
	return e
}

// Lower is the lower bound of the range column (PostgreSQL lower). Note that a
// discrete range stores its lower bound inclusive and its upper bound
// exclusive, so Upper of [1,10) is 10. Compare it with the methods of any
// scalar expression, for example Lower(during).Ge(start).
func Lower[T types.RangeElem](col momo.Expression[types.Range[T]]) BoundExpr[T] {
	return newBound("range lower bound (lower)", "lower", col)
}

// Upper is the upper bound of the range column (PostgreSQL upper). See Lower.
func Upper[T types.RangeElem](col momo.Expression[types.Range[T]]) BoundExpr[T] {
	return newBound("range upper bound (upper)", "upper", col)
}

// Flags returns the flags of the column operand, so a sensitive column keeps
// the values it is compared with masked.
func (e BoundExpr[T]) Flags() momo.ColumnFlags { return exprkit.FlagsOf(e.src) }

// As names the expression in a select list.
func (e BoundExpr[T]) As(alias string) momo.Aliased[T] { return momo.As[T](e, alias) }

// AppendSQL renders the bound, or fails the build with ErrUnsupported on a
// dialect without range types.
func (e BoundExpr[T]) AppendSQL(b *momo.Builder) {
	if b.Dialect().Name != dialect.Postgres {
		exprkit.Unsupported(b, e.feature)
		return
	}
	b.WriteString(e.fn + "(")
	exprkit.RenderSensitive(b, e.src, exprkit.FlagsOf(e.src))
	b.WriteString(")")
}
