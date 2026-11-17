// SPDX-License-Identifier: Apache-2.0

package momo

import "context"

// Explain runs EXPLAIN (never EXPLAIN ANALYZE) for the query and returns the plan text,
// one line per row joined by newlines. Parameters are bound, never interpolated.
//
// EXPLAIN plans the statement without executing it, so Explain has no side
// effects on data. It runs on the query's own connection, so inside a Tx the
// plan reflects that transaction's settings (for example SetLocal planner
// switches). PostgreSQL uses EXPLAIN and SQLite uses EXPLAIN QUERY PLAN. MySQL
// and MariaDB return ErrUnsupported: their tabular EXPLAIN has no single plan
// column.
func (sq SelectQuery[T]) Explain(ctx context.Context) (string, error) {
	return explainSelect(ctx, sq.q, infoOf(sq.meta), sq.s)
}
