// SPDX-License-Identifier: Apache-2.0

package momo

// selectFrom builds an unfiltered SelectQuery over a generic meta value,
// reading FROM the given table.
func selectFrom[T any](q Querier, meta *Meta[T], from TableRef) SelectQuery[T] {
	return SelectQuery[T]{q: q, meta: meta, s: selectCore{limit: -1, from: from}}
}
