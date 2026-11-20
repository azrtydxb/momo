// SPDX-License-Identifier: Apache-2.0

// Package exprkit is the bridge the feature packages of this module (agg,
// arrayop, fts, jsonop, rangeop, sqlfn, timeop, vecop) use to build SQL
// expressions on top of package momo. It carries the operations that are not
// part of the frozen extension API of the root package: binding a value with
// the sensitive-column mask, rendering an operand to learn whether it read a
// sensitive column, writing a FILTER clause, recording an unsupported feature,
// and a few helpers.
//
// The frozen extension API is the root package's Expr, Expression, Ops,
// NewOps, Typed, Flagged and the Builder methods Ident, Arg, Dialect, SetErr
// and WriteString. Everything else a feature package needs is here, behind
// internal/, so no third party can depend on it. The functions forward to the
// root package through internal/bridge.
package exprkit

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/bridge"
	"github.com/azrtydxb/momo/schema"
)

// NewBuilder returns a builder for the dialect.
func NewBuilder(d *dialect.Dialect) *momo.Builder {
	return bridge.Get().NewBuilder(d).(*momo.Builder)
}

// ArgFor binds v like Builder.Arg and records it as sensitive when flags
// carries momo.FlagSensitive, so a hook sees momo.RedactedArg instead of the
// value. Take flags from FlagsOf of the operand the value is compared with, or
// from RenderSensitive.
func ArgFor(b *momo.Builder, flags momo.ColumnFlags, v any) {
	bridge.Get().ArgFor(b, uint16(flags), v)
}

// RenderSensitive renders e and returns flags with momo.FlagSensitive added
// when e read a sensitive column. Bind the other operands of the predicate
// with the result, through ArgFor or ExprFor.
func RenderSensitive(b *momo.Builder, e momo.Expr, flags momo.ColumnFlags) momo.ColumnFlags {
	return momo.ColumnFlags(bridge.Get().RenderSensitive(b, e, uint16(flags)))
}

// ExprFor renders e with every argument it binds recorded as sensitive when
// flags carries momo.FlagSensitive, for an expression compared with or
// assigned to such a column.
func ExprFor(b *momo.Builder, flags momo.ColumnFlags, e momo.Expr) {
	bridge.Get().ExprFor(b, uint16(flags), e)
}

// MarkSensitive records that the expression being rendered read a sensitive
// column, so an enclosing predicate masks its other operands. A column
// reference does this by itself; call it from an expression that writes a
// sensitive column's identifier directly.
func MarkSensitive(b *momo.Builder) { bridge.Get().MarkSensitive(b) }

// AppendFilter writes " FILTER (WHERE p)" for an aggregate call that was just
// rendered. A nil predicate fails the build with momo.ErrValidation; MySQL has
// no FILTER clause and fails with momo.ErrUnsupported.
func AppendFilter(b *momo.Builder, p momo.Predicate) { bridge.Get().AppendFilter(b, p) }

// AppendOrder renders the order item as "expr ASC|DESC [NULLS FIRST|LAST]",
// emulating the NULLS clause where the dialect lacks it.
func AppendOrder(b *momo.Builder, o momo.Order) { bridge.Get().AppendOrder(b, o) }

// Unsupported records a momo.ErrUnsupported build error for feature, naming
// the dialect being rendered.
func Unsupported(b *momo.Builder, feature string) { bridge.Get().Unsupported(b, feature) }

// FlagsOf returns the column flags of e, or 0 when e does not implement
// momo.Flagged.
func FlagsOf(e momo.Expr) momo.ColumnFlags {
	if f, ok := e.(momo.Flagged); ok {
		return f.Flags()
	}
	// A wrapper written before the flags had a type returns a uint16. Honour it,
	// or its sensitive flag would be dropped and a value bound in plaintext.
	if f, ok := e.(interface{ Flags() uint16 }); ok {
		return momo.ColumnFlags(f.Flags())
	}
	return 0
}

// IsBound reports whether e is a Go value bound as a parameter by momo.Value,
// possibly wrapped by momo.Typed. It is false for a column, a function call or
// any other expression.
func IsBound(e momo.Expr) bool { return bridge.Get().IsBound(e) }

// Predicate adapts a render function to a momo.Predicate. The function writes
// the condition into the builder with Builder.Ident and Builder.Arg, or with
// ArgFor.
func Predicate(f func(b *momo.Builder)) momo.Predicate { return momo.Typed[bool](predicateFunc(f)) }

type predicateFunc func(b *momo.Builder)

func (f predicateFunc) AppendSQL(b *momo.Builder) { f(b) }

// ValidName checks an identifier-like name: non-empty, at most 63 bytes, a
// letter or underscore first, then letters, digits, underscores or dollar
// signs.
func ValidName(s string) error { return bridge.Get().ValidName(s) }

// ValidTypeName checks a "word word(1,2)" shaped SQL type name, such as bigint,
// double precision or numeric(10,2).
func ValidTypeName(s string) error { return bridge.Get().ValidTypeName(s) }

// WindowCall turns the aggregate call into the window function call OVER (w).
// The result renders identically to a momo.WindowExpr; the caller wraps it in
// its own typed expression.
func WindowCall(call momo.Expr, w momo.WindowSpec) momo.Expr {
	return bridge.Get().WindowCall(call, w).(momo.Expr)
}

// TableSchema returns the normalized table definition of model T from its
// generated metadata, or nil for a projection.
func TableSchema[T any, PT momo.Modeler[T]]() *schema.Table {
	var p PT // nil pointer; generated MomoMeta never dereferences its receiver
	m := p.MomoMeta()
	if m == nil {
		return nil
	}
	return m.TableSchema
}
