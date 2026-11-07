// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/azrtydxb/momo/dialect"
)

// ------------------------------------------------------- aggregate FILTER

// FilteredAgg is an aggregate with a FILTER (WHERE ...) clause, built with
// Agg.Filter. It is an Expression[T], carries the comparison methods HAVING
// needs, and can be turned into a window function with Over.
type FilteredAgg[T any] struct {
	exprOps[T]
	agg   Agg[T]
	where Predicate
}

// Filter restricts the rows the aggregate sees: COUNT(*) FILTER (WHERE p).
// PostgreSQL and SQLite support it; MySQL has no FILTER clause and fails the
// build with ErrUnsupported (write SUM(CASE WHEN ...) there).
func (a Agg[T]) Filter(p Predicate) FilteredAgg[T] {
	f := FilteredAgg[T]{agg: a, where: p}
	f.exprOps = selfOps[T](f)
	return f
}

// As names the aggregate in a select list.
func (f FilteredAgg[T]) As(alias string) Aliased[T] { return As[T](f, alias) }

// Over turns the filtered aggregate into a window function.
func (f FilteredAgg[T]) Over(w WindowSpec) WindowExpr[T] {
	return newWindowExpr[T](f, "", false, false, w)
}

func (FilteredAgg[T]) resultType() T { var z T; return z }

func (f FilteredAgg[T]) AppendSQL(b *Builder) {
	if b.d.Name == dialect.MySQL {
		b.unsupported("aggregate FILTER")
		return
	}
	if f.where == nil {
		b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: errors.New("Filter needs a predicate")})
		return
	}
	f.agg.AppendSQL(b)
	b.WriteString(" FILTER (WHERE ")
	f.where.AppendSQL(b)
	b.writeByte(')')
}

// -------------------------------------------------------------- windows

type frameKind uint8

const (
	boundUnboundedPreceding frameKind = iota
	boundPreceding
	boundCurrentRow
	boundFollowing
	boundUnboundedFollowing
)

// FrameBound is one end of a window frame.
type FrameBound struct {
	kind frameKind
	n    int
}

// UnboundedPreceding is UNBOUNDED PRECEDING (a start bound).
func UnboundedPreceding() FrameBound { return FrameBound{kind: boundUnboundedPreceding} }

// Preceding is n PRECEDING. n must not be negative.
func Preceding(n int) FrameBound { return FrameBound{kind: boundPreceding, n: n} }

// CurrentRow is CURRENT ROW.
func CurrentRow() FrameBound { return FrameBound{kind: boundCurrentRow} }

// Following is n FOLLOWING. n must not be negative.
func Following(n int) FrameBound { return FrameBound{kind: boundFollowing, n: n} }

// UnboundedFollowing is UNBOUNDED FOLLOWING (an end bound).
func UnboundedFollowing() FrameBound { return FrameBound{kind: boundUnboundedFollowing} }

func (f FrameBound) offset() bool { return f.kind == boundPreceding || f.kind == boundFollowing }

func (f FrameBound) appendSQL(b *Builder) {
	switch f.kind {
	case boundUnboundedPreceding:
		b.WriteString("UNBOUNDED PRECEDING")
	case boundPreceding:
		b.WriteString(strconv.Itoa(f.n) + " PRECEDING")
	case boundCurrentRow:
		b.WriteString("CURRENT ROW")
	case boundFollowing:
		b.WriteString(strconv.Itoa(f.n) + " FOLLOWING")
	case boundUnboundedFollowing:
		b.WriteString("UNBOUNDED FOLLOWING")
	}
}

type windowFrame struct {
	unit       string // ROWS, RANGE or GROUPS
	start, end FrameBound
}

// WindowSpec is the OVER (...) clause: PARTITION BY, ORDER BY and a frame.
// It is an immutable value; every method returns a new one.
type WindowSpec struct {
	partition []Expr
	order     []Order
	frame     *windowFrame
	err       error
}

// Over starts an empty window specification: OVER ().
func Over() WindowSpec { return WindowSpec{} }

// PartitionBy adds PARTITION BY expressions.
func (w WindowSpec) PartitionBy(es ...Expr) WindowSpec {
	for _, e := range es {
		rejectAliased(&w.err, "PARTITION BY", e)
	}
	w.partition = append(slices.Clip(w.partition), es...)
	return w
}

// OrderBy adds ORDER BY items, exactly like SelectQuery.OrderBy.
func (w WindowSpec) OrderBy(items ...Orderable) WindowSpec {
	o := slices.Clip(w.order)
	for _, it := range items {
		o = append(o, it.order())
	}
	w.order = o
	return w
}

// Rows sets a ROWS BETWEEN start AND end frame.
func (w WindowSpec) Rows(start, end FrameBound) WindowSpec { return w.withFrame("ROWS", start, end) }

// Range sets a RANGE BETWEEN start AND end frame. An offset bound needs an
// ORDER BY.
func (w WindowSpec) Range(start, end FrameBound) WindowSpec { return w.withFrame("RANGE", start, end) }

// Groups sets a GROUPS BETWEEN start AND end frame (PostgreSQL and SQLite;
// MySQL fails the build with ErrUnsupported). It needs an ORDER BY.
func (w WindowSpec) Groups(start, end FrameBound) WindowSpec {
	return w.withFrame("GROUPS", start, end)
}

func (w WindowSpec) withFrame(unit string, start, end FrameBound) WindowSpec {
	w.frame = &windowFrame{unit, start, end}
	return w
}

func (w WindowSpec) validate(d *dialect.Dialect) error {
	if w.err != nil {
		return w.err
	}
	f := w.frame
	if f == nil {
		return nil
	}
	if f.unit == "GROUPS" && d.Name == dialect.MySQL {
		return &Error{Kind: ErrUnsupported, Op: "build", Err: fmt.Errorf("GROUPS window frame is not supported by %s", d.Name)}
	}
	bad := func(msg string) error {
		return &Error{Kind: ErrValidation, Op: "build", Err: errors.New(msg)}
	}
	for _, bound := range []FrameBound{f.start, f.end} {
		if bound.offset() && bound.n < 0 {
			return bad("window frame offset must not be negative")
		}
	}
	switch {
	case f.start.kind == boundUnboundedFollowing:
		return bad("window frame cannot start at UNBOUNDED FOLLOWING")
	case f.end.kind == boundUnboundedPreceding:
		return bad("window frame cannot end at UNBOUNDED PRECEDING")
	case f.start.kind > f.end.kind:
		return bad("window frame start must not come after its end")
	case f.unit == "GROUPS" && len(w.order) == 0:
		return bad("a GROUPS window frame needs ORDER BY")
	case f.unit == "RANGE" && (f.start.offset() || f.end.offset()):
		if len(w.order) != 1 {
			return bad("a RANGE window frame with an offset needs exactly one ORDER BY item")
		}
	}
	return nil
}

func (w WindowSpec) appendSQL(b *Builder) {
	b.WriteString("OVER (")
	sep := ""
	if len(w.partition) > 0 {
		b.WriteString("PARTITION BY ")
		appendList(b, w.partition)
		sep = " "
	}
	if len(w.order) > 0 {
		b.WriteString(sep + "ORDER BY ")
		for i, o := range w.order {
			if i > 0 {
				b.WriteString(", ")
			}
			o.appendSQL(b)
		}
		sep = " "
	}
	if f := w.frame; f != nil {
		b.WriteString(sep + f.unit + " BETWEEN ")
		f.start.appendSQL(b)
		b.WriteString(" AND ")
		f.end.appendSQL(b)
	}
	b.writeByte(')')
}

// WindowFunc is a window function that needs its OVER clause: RowNumber,
// Rank, DenseRank, Lag, Lead, FirstValue and LastValue. Aggregates take
// theirs through Agg.Over.
type WindowFunc[T any] struct {
	call Expr
	name string
	// needOrder: the function is meaningless without ORDER BY and rejects
	// a frame (ranking and offset functions).
	needOrder bool
	noFrame   bool
}

// Over attaches the window specification.
func (f WindowFunc[T]) Over(w WindowSpec) WindowExpr[T] {
	return newWindowExpr[T](f.call, f.name, f.needOrder, f.noFrame, w)
}

func rankingFn(name string, needOrder bool) WindowFunc[int64] {
	return WindowFunc[int64]{call: fn{name: name}, name: name, needOrder: needOrder, noFrame: true}
}

// RowNumber is row_number(): the row's 1-based position in its partition.
func RowNumber() WindowFunc[int64] { return rankingFn("row_number", false) }

// Rank is rank(): the rank with gaps after ties. It needs an ORDER BY.
func Rank() WindowFunc[int64] { return rankingFn("rank", true) }

// DenseRank is dense_rank(): the rank without gaps. It needs an ORDER BY.
func DenseRank() WindowFunc[int64] { return rankingFn("dense_rank", true) }

// Lag is lag(e, offset): the value offset rows before the current row, NULL
// when there is none (scan into a nullable destination). It needs an
// ORDER BY and rejects a frame.
func Lag[T any](e Expression[T], offset int) WindowFunc[T] {
	return offsetFn[T]("lag", e, offset, nil)
}

// LagDefault is lag(e, offset, def): def replaces the NULL of a missing row.
func LagDefault[T any](e Expression[T], offset int, def T) WindowFunc[T] {
	return offsetFn[T]("lag", e, offset, argExpr{def})
}

// Lead is lead(e, offset): the value offset rows after the current row.
func Lead[T any](e Expression[T], offset int) WindowFunc[T] {
	return offsetFn[T]("lead", e, offset, nil)
}

// LeadDefault is lead(e, offset, def).
func LeadDefault[T any](e Expression[T], offset int, def T) WindowFunc[T] {
	return offsetFn[T]("lead", e, offset, argExpr{def})
}

func offsetFn[T any](name string, e Expr, offset int, def Expr) WindowFunc[T] {
	args := []Expr{e, intLit(offset)}
	if def != nil {
		args = append(args, def)
	}
	f := WindowFunc[T]{call: fn{name: name, args: args}, name: name, needOrder: true, noFrame: true}
	if offset < 0 {
		f.call = invalidExpr{fmt.Errorf("%s offset must not be negative, got %d", name, offset)}
	}
	return f
}

// FirstValue is first_value(e) over the window frame.
func FirstValue[T any](e Expression[T]) WindowFunc[T] {
	return WindowFunc[T]{call: fn{name: "first_value", args: []Expr{e}}, name: "first_value", needOrder: true}
}

// LastValue is last_value(e) over the window frame. The default frame ends
// at the current row; add Rows(UnboundedPreceding(), UnboundedFollowing())
// for the last row of the partition.
func LastValue[T any](e Expression[T]) WindowFunc[T] {
	return WindowFunc[T]{call: fn{name: "last_value", args: []Expr{e}}, name: "last_value", needOrder: true}
}

// invalidExpr fails the build with a validation error.
type invalidExpr struct{ err error }

func (e invalidExpr) AppendSQL(b *Builder) {
	b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: e.err})
}

// WindowExpr is call OVER (...). It is an Expression[T]; name it with As.
type WindowExpr[T any] struct {
	exprOps[T]
	call      Expr
	name      string
	spec      WindowSpec
	needOrder bool
	noFrame   bool
}

func newWindowExpr[T any](call Expr, name string, needOrder, noFrame bool, w WindowSpec) WindowExpr[T] {
	e := WindowExpr[T]{call: call, name: name, spec: w, needOrder: needOrder, noFrame: noFrame}
	e.exprOps = selfOps[T](e)
	return e
}

// Over turns an aggregate into a window function: SUM(x) OVER (...).
func (a Agg[T]) Over(w WindowSpec) WindowExpr[T] { return newWindowExpr[T](a, "", false, false, w) }

// As names the window expression in a select list.
func (e WindowExpr[T]) As(alias string) Aliased[T] { return As[T](e, alias) }

func (WindowExpr[T]) resultType() T { var z T; return z }

func (e WindowExpr[T]) AppendSQL(b *Builder) {
	if !b.d.WindowFunctions {
		b.unsupported("window functions")
		return
	}
	if err := e.spec.validate(b.d); err != nil {
		b.SetErr(err)
		return
	}
	if e.needOrder && len(e.spec.order) == 0 {
		b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: fmt.Errorf("%s needs an ORDER BY in its window", e.name)})
		return
	}
	if e.noFrame && e.spec.frame != nil {
		b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: fmt.Errorf("%s does not take a window frame", e.name)})
		return
	}
	e.call.AppendSQL(b)
	b.writeByte(' ')
	e.spec.appendSQL(b)
}
