// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/bridge"
)

// The feature packages of this module (agg, arrayop, fts, jsonop, rangeop,
// sqlfn, timeop, vecop) build expressions on the frozen extension API plus a
// few root internals. The root registers those internals with internal/bridge
// here; internal/exprkit wraps them in typed functions. Nothing below is
// exported, and the registry lives under internal/, so no code outside this
// module can reach it.
func init() {
	bridge.Register(&bridge.Kit{
		NewBuilder: func(d *dialect.Dialect) any { return newBuilder(d) },
		ArgFor: func(b any, flags uint16, v any) {
			b.(*Builder).argFor(ColumnFlags(flags), v)
		},
		RenderSensitive: func(b, e any, flags uint16) uint16 {
			return uint16(b.(*Builder).renderSens(e.(Expr), ColumnFlags(flags)))
		},
		ExprFor: func(b any, flags uint16, e any) {
			b.(*Builder).exprFor(ColumnFlags(flags), e.(Expr))
		},
		MarkSensitive: func(b any) { b.(*Builder).markSensitive() },
		AppendFilter: func(b, p any) {
			pred, _ := p.(Predicate) // a nil predicate is reported by appendFilter
			appendFilter(b.(*Builder), pred)
		},
		Unsupported: func(b any, feature string) { b.(*Builder).unsupported(feature) },
		IsBound: func(e any) bool {
			x, _ := e.(Expr) // a nil expression is not bound
			return isBound(x)
		},
		WindowCall: func(call, w any) any {
			return newWindowExpr[any](call.(Expr), "", false, false, w.(WindowSpec))
		},
		ValidName:     validName,
		ValidTypeName: validTypeName,
	})
}
