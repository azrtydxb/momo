// SPDX-License-Identifier: Apache-2.0

package momo

// hasEffectiveFilter reports whether preds restrict the rows of a write. An
// empty And or Or (and a junction holding only empty junctions) renders a
// constant, so it is not a filter. UPDATE and DELETE share this guard.
func hasEffectiveFilter(preds []Predicate) bool {
	for _, p := range preds {
		if j, ok := p.(junction); ok {
			if hasEffectiveFilter(j.parts) {
				return true
			}
			continue
		}
		if p != nil {
			return true
		}
	}
	return false
}
