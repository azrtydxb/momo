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
	if n <= 0 {
		sq.s.err = &Error{Kind: ErrValidation, Op: "build", Table: sq.meta.Table, Err: fmt.Errorf("MaxRows must be positive, got %d", n)}
		return sq
	}
	sq.s.maxRows = n
	return sq
}

func (sq SelectQuery[T]) maxRowsError() error {
	return &Error{Kind: ErrLimitExceeded, Op: "select", Table: sq.meta.Table, Err: fmt.Errorf("result has more than %d rows (MaxRows)", sq.s.maxRows)}
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
