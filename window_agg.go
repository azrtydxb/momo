// SPDX-License-Identifier: Apache-2.0

package momo

import "errors"

// overAgg renders an AggExpr that is used as a window function, refusing the
// clauses PostgreSQL does not allow with OVER.
type overAgg struct{ spec aggSpec }

func (o overAgg) AppendSQL(b *Builder) {
	switch {
	case len(o.spec.within) > 0 || o.spec.ordered:
		aggErr(b, errors.New("an ordered-set aggregate (WITHIN GROUP) cannot be used with Over"))
		return
	case len(o.spec.orderBy) > 0:
		aggErr(b, errors.New("an aggregate with an ORDER BY inside the call cannot be used with Over; order the window instead"))
		return
	}
	o.spec.appendSQL(b) // agg(...) FILTER (WHERE ...), before OVER
}

// Over turns the aggregate into a window function:
// agg(...) [FILTER (WHERE ...)] OVER (...). An aggregate with an ORDER BY
// inside the call, or a WITHIN GROUP ordered-set aggregate, fails the build
// with ErrValidation, as PostgreSQL does not allow them as window functions.
// Window functions are gated per dialect like the other window functions.
func (a AggExpr[T]) Over(w WindowSpec) WindowExpr[T] {
	return newWindowExpr[T](overAgg{a.spec}, "", false, false, w)
}
