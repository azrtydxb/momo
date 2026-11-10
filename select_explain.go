// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"fmt"
	"strings"

	"github.com/azrtydxb/momo/dialect"
)

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
	name := sq.q.Dialect().Name
	var prefix string
	switch name {
	case dialect.Postgres:
		prefix = "EXPLAIN "
	case dialect.SQLite:
		prefix = "EXPLAIN QUERY PLAN "
	default:
		return "", fmt.Errorf("momo: Explain on %s: %w", name, ErrUnsupported)
	}
	c, _, err := sq.compileJoined(ctx)
	if err != nil {
		return "", err
	}
	c.SQL = prefix + c.SQL
	r, err := sq.q.conn().query(ctx, "explain", sq.meta.Table, "", c)
	if err != nil {
		return "", err
	}
	defer r.Close()
	var lines []string
	for r.Next() {
		var line string
		if name == dialect.SQLite {
			// EXPLAIN QUERY PLAN returns (id, parent, notused, detail).
			var id, parent, unused any
			err = r.Scan(&id, &parent, &unused, &line)
		} else {
			err = r.Scan(&line)
		}
		if err != nil {
			return "", err
		}
		lines = append(lines, line)
	}
	if err := r.Err(); err != nil {
		return "", err
	}
	if err := r.Close(); err != nil {
		return "", err
	}
	return strings.Join(lines, "\n"), nil
}
