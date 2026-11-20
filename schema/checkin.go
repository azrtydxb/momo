// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"errors"
	"fmt"
	"slices"
	"unicode"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/schemautil"
)

// CheckIn describes a CHECK constraint that restricts a text column to a set of
// values: CHECK (column IN ('a', 'b')). A Check that carries one is diffed as a
// set of values, so two edits that add different values to the same list do not
// conflict and the order of the values never matters. Check.Expr still holds
// the canonical rendering of the sorted values, so a snapshot read by an older
// version keeps working as a plain check.
type CheckIn struct {
	Column string   `json:"column"`
	Values []string `json:"values"`
}

// Validate reports whether the list can be rendered: it names a column, holds
// at least one value, and no value holds a backslash or a control character
// (MySQL reads a backslash in a string literal as an escape, so no spelling of
// such a value is portable).
func (c CheckIn) Validate() error {
	if c.Column == "" {
		return errors.New("the value list names no column")
	}
	if len(c.Values) == 0 {
		return fmt.Errorf("the value list of column %q is empty", c.Column)
	}
	for _, v := range c.Values {
		for _, r := range v {
			if r == '\\' || unicode.IsControl(r) {
				return fmt.Errorf("the value %q of column %q holds a backslash or a control character", v, c.Column)
			}
		}
	}
	return nil
}

// ExprFor returns the expression a renderer writes for the check on dialect d:
// the rendering of its value list when it has one, otherwise Expr.
func (c Check) ExprFor(d dialect.Name) string {
	if c.In != nil && c.In.Column != "" && len(c.In.Values) > 0 {
		return schemautil.CheckInExpr(d, c.In.Column, c.In.Values)
	}
	return c.Expr
}

// ValueList returns the value list the check holds: In when it is set, else the
// list read from Expr when that is a value-list expression in any spelling the
// supported databases report (what an inspector returns carries no In).
func (c Check) ValueList() (CheckIn, bool) {
	if c.In != nil && c.In.Column != "" && len(c.In.Values) > 0 {
		return *c.In, true
	}
	col, vals, ok := schemautil.ParseCheckIn(c.Expr)
	if !ok {
		return CheckIn{}, false
	}
	return CheckIn{Column: col, Values: vals}, true
}

// sameCheck reports whether two checks of one name are the same constraint.
// When at least one declares a value list and both hold one, the column and the
// set of values are compared, whatever the order, repeats or spelling of the
// expression. Otherwise the expressions are compared as written. Diff and Equal
// both decide on this, so they agree on a value-list check.
func sameCheck(a, b Check) bool {
	if a.In != nil || b.In != nil {
		la, okA := a.ValueList()
		lb, okB := b.ValueList()
		if okA && okB {
			return la.Column == lb.Column && slices.Equal(schemautil.SortedValues(la.Values), schemautil.SortedValues(lb.Values))
		}
	}
	return a.Expr == b.Expr
}
