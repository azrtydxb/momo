// SPDX-License-Identifier: Apache-2.0

package momo

import "errors"

// ReturningList is a RETURNING column list shared by the write builders.
type ReturningList struct{ cols []ColumnRef }

// Returning builds a RETURNING list from column descriptors.
func Returning(cols ...ColumnRef) ReturningList {
	return ReturningList{cols: append([]ColumnRef(nil), cols...)}
}

// appendReturning renders ` RETURNING "c1", "c2"` with unqualified names.
func appendReturning(b *Builder, cols []ColumnRef) {
	if len(cols) == 0 {
		return
	}
	b.WriteString(" RETURNING ")
	prev := b.unqualified
	b.unqualified = true
	for i, c := range cols {
		if i > 0 {
			b.WriteString(", ")
		}
		b.Ident(c.ColumnName())
	}
	b.unqualified = prev
}

// ModelRows is the cursor of a write statement with a RETURNING list. Each
// row scans into the model fields named by the list; other fields are left
// untouched.
type ModelRows[T any] struct {
	rows Rows
	meta *Meta[T]
	idx  []int
	dest []any
}

// Next advances to the next returned row.
func (r *ModelRows[T]) Next() bool { return r.rows.Next() }

// ScanModel scans the current row into the fields of m named by RETURNING.
func (r *ModelRows[T]) ScanModel(m *T) error {
	if m == nil {
		return errors.New("momo: ScanModel into nil model")
	}
	if r.dest == nil {
		r.dest = make([]any, len(r.idx))
	}
	for i, ci := range r.idx {
		r.dest[i] = r.meta.Ptr(m, ci)
	}
	return r.rows.Scan(r.dest...)
}

// Err returns the error that ended iteration, if any.
func (r *ModelRows[T]) Err() error { return r.rows.Err() }

// Close releases the cursor. It is safe to call more than once.
func (r *ModelRows[T]) Close() error { return r.rows.Close() }
