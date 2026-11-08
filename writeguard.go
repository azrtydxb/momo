// SPDX-License-Identifier: Apache-2.0

package momo

import "strings"

// emptyFilterer is implemented by predicates that can render a constant and so
// restrict nothing: an And or Or with no effective part, or a JoinSQL with
// AND or OR and no parts.
type emptyFilterer interface{ noFilter() bool }

func (j junction) noFilter() bool { return !hasEffectiveFilter(j.parts) }

func (j joinExpr) noFilter() bool {
	if len(j.parts) > 0 {
		return false
	}
	s := joinSeparators[strings.ToUpper(strings.Trim(j.sep, " \t\r\n"))]
	return s.text == " AND " || s.text == " OR "
}

// hasEffectiveFilter reports whether preds restrict the rows of a write. An
// empty And or Or (and a junction holding only empty junctions) renders a
// constant, so it is not a filter. UPDATE and DELETE share this guard.
func hasEffectiveFilter(preds []Predicate) bool {
	for _, p := range preds {
		if e, ok := p.(emptyFilterer); ok && e.noFilter() {
			continue
		}
		if p != nil {
			return true
		}
	}
	return false
}
