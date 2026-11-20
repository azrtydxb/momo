// SPDX-License-Identifier: Apache-2.0

package schemaops

import (
	"slices"

	"github.com/azrtydxb/momo/internal/schemautil"
	"github.com/azrtydxb/momo/schema"
)

// SameCheck reports whether two checks of one name are the same constraint.
// When at least one declares a value list and both hold one, the column and the
// set of values are compared, whatever the order, repeats or spelling of the
// expression. Otherwise the expressions are compared as written. Diff and Equal
// both decide on this, so they agree on a value-list check.
func SameCheck(a, b schema.Check) bool {
	if a.In != nil || b.In != nil {
		la, okA := a.ValueList()
		lb, okB := b.ValueList()
		if okA && okB {
			return la.Column == lb.Column && slices.Equal(schemautil.SortedValues(la.Values), schemautil.SortedValues(lb.Values))
		}
	}
	return a.Expr == b.Expr
}
