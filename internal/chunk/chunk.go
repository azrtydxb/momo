// SPDX-License-Identifier: Apache-2.0

// Package chunk holds the bind-limit arithmetic that splits a long key list
// into statements. It is pure: it takes a dialect and plain Go values, and
// imports nothing from the momo root package.
package chunk

import "github.com/azrtydxb/momo/dialect"

// Composite-key tuple caps per statement. Long composite lists fail long before
// the bind limit: PostgreSQL parses a row-value IN list recursively and rejects
// it with "stack depth limit exceeded" (SQLSTATE 54001) at roughly 10000
// two-column tuples (the exact point depends on max_stack_depth; 32767 always
// fails), and SQLite renders composite keys as a chain of ORs whose expression
// depth is limited to 1000 by default ("Expression tree is too large").
// Single-column lists are flat and use the full bind budget.
const (
	// MaxCompositeTuples caps composite tuples per statement on PostgreSQL,
	// MySQL and any dialect without a specific cap.
	MaxCompositeTuples = 2000
	// MaxCompositeTuplesSQLite is the cap for the SQLite OR-of-ANDs form.
	MaxCompositeTuplesSQLite = 400
)

// KeysPerStatement returns how many key tuples fit in one statement under the
// dialect's bind limit, leaving reserved parameters for other predicates.
// Composite keys (ncols > 1) are additionally capped (see MaxCompositeTuples).
// A dialect without a bind limit (MaxParams <= 0) is limited only by the cap
// for composite keys and returns a large bound for single-column keys.
func KeysPerStatement(d *dialect.Dialect, ncols, reserved int) int {
	if ncols < 1 {
		ncols = 1
	}
	n := 1 << 20
	if d.MaxParams > 0 {
		n = max((d.MaxParams-reserved)/ncols, 1)
	}
	if ncols > 1 {
		limit := MaxCompositeTuples
		if d.Name == dialect.SQLite {
			limit = MaxCompositeTuplesSQLite
		}
		n = min(n, limit)
	}
	return n
}

// Each splits total items into consecutive ranges of at most n and calls fn
// with the half-open range [lo, hi) of each, in order. It stops at the first
// error fn returns. A total of zero never calls fn; n below one is treated as
// one.
func Each(total, n int, fn func(lo, hi int) error) error {
	n = max(n, 1)
	for lo := 0; lo < total; lo += n {
		if err := fn(lo, min(lo+n, total)); err != nil {
			return err
		}
	}
	return nil
}
