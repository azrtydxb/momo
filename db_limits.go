// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"fmt"
	"slices"
)

// MaxRows caps the rows a collecting read (All, Pluck) may return. When the
// query yields more than n rows the read fails with ErrLimitExceeded; rows are
// never silently truncated. Streaming reads (Each, Iter) and Count are not
// capped, and n must be positive.
func (sq SelectQuery[T]) MaxRows(n int) SelectQuery[T] {
	sq.s.setMaxRows(tableOf(sq.meta), n)
	return sq
}

// WithMaxInList caps the number of values in one typed IN or NOT IN list, and
// in one In argument of raw SQL, at n. A longer list fails the build with
// ErrLimitExceeded instead of reaching the database. Tuple IN lists that
// momo builds itself for relation loading are not capped here: they are
// chunked under the bind-parameter limit (WithMaxParams) instead. n must be positive: WithMaxInList panics otherwise, like
// WithMaxParams. The dialect value is copied; the shared dialect is never
// mutated.
func WithMaxInList(n int) Option {
	return func(db *DB) {
		if n <= 0 {
			panic(fmt.Sprintf("momo: WithMaxInList(%d): the limit must be positive", n))
		}
		d := *db.ex.d
		d.MaxInList = n
		d.Vector.Metrics = slices.Clone(d.Vector.Metrics)
		d.Vector.Indexes = slices.Clone(d.Vector.Indexes)
		db.ex.d = &d
	}
}
