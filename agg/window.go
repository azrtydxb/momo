// SPDX-License-Identifier: Apache-2.0

package agg

import (
	"errors"

	"github.com/azrtydxb/momo"
)

// overCall renders a Call that is used as a window function, refusing the
// clauses PostgreSQL does not allow with OVER.
type overCall struct{ spec spec }

func (o overCall) AppendSQL(b *momo.Builder) {
	switch {
	case len(o.spec.within) > 0 || o.spec.ordered:
		fail(b, errors.New("an ordered-set aggregate (WITHIN GROUP) cannot be used with Over"))
		return
	case o.spec.distinct:
		fail(b, errors.New("an aggregate with Distinct cannot be used with Over: PostgreSQL does not allow DISTINCT in a window function"))
		return
	case len(o.spec.orderBy) > 0:
		fail(b, errors.New("an aggregate with an ORDER BY inside the call cannot be used with Over; order the window instead"))
		return
	}
	o.spec.appendSQL(b) // agg(...) FILTER (WHERE ...), before OVER
}

// Over turns the aggregate into a window function:
// agg(...) [FILTER (WHERE ...)] OVER (...). An aggregate with an ORDER BY
// inside the call, or a WITHIN GROUP ordered-set aggregate, fails the build
// with ErrValidation, as PostgreSQL does not allow them as window functions.
// Window functions are gated per dialect like the other window functions.
func (a Call[T]) Over(w momo.WindowSpec) momo.WindowExpr[T] {
	return momo.WindowOf[T](overCall{a.spec}, w)
}
