// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

// ReturningList is a RETURNING column list shared by the write builders.
type ReturningList struct{ cols []ColumnRef }

// Returning builds a RETURNING list from column descriptors.
func Returning(cols ...ColumnRef) ReturningList {
	return ReturningList{cols: append([]ColumnRef(nil), cols...)}
}

// appendReturningNames renders ` RETURNING "c1", "c2"` with unqualified names.
func appendReturningNames(b *Builder, names []string) {
	if len(names) == 0 {
		return
	}
	b.WriteString(" RETURNING ")
	prev := b.unqualified
	b.unqualified = true
	for i, n := range names {
		if i > 0 {
			b.WriteString(", ")
		}
		b.Ident(n)
	}
	b.unqualified = prev
}

// returningSet is the RETURNING state embedded in the write builders: an
// explicit column list, or every mapped column in metadata order.
type returningSet struct {
	cols []ColumnRef
	all  bool
}

// active reports whether the statement has a RETURNING clause.
func (r returningSet) active() bool { return r.all || len(r.cols) > 0 }

func (r returningSet) with(cols ...ColumnRef) returningSet {
	r.cols = append(slices.Clip(r.cols), cols...)
	return r
}

func (r returningSet) withAll() returningSet { r.all = true; return r }

// returningIndexes resolves the set to column indexes of m, in the order the
// columns are rendered and scanned. For an unmapped column it returns the
// offending name. Under ReturningAll the explicit list is ignored.
func returningIndexes[T any](m *Meta[T], r returningSet) (idx []int, bad string) {
	if r.all {
		idx = make([]int, len(m.Columns))
		for i := range idx {
			idx[i] = i
		}
		return idx, ""
	}
	idx = make([]int, len(r.cols))
	for i, c := range r.cols {
		idx[i] = slices.IndexFunc(m.Columns, func(mc ColumnMeta) bool { return mc.Name == c.ColumnName() })
		if idx[i] < 0 {
			return nil, c.ColumnName()
		}
	}
	return idx, ""
}

// renderReturning gates RETURNING on the dialect, validates the columns
// against m and appends the clause. invalid builds the error for an unmapped
// column. It is the single entry point the Insert, Update and Delete
// builders share.
func renderReturning[T any](b *Builder, m *Meta[T], r returningSet, invalid func(col string) error) {
	if !r.active() {
		return
	}
	if !b.Dialect().Returning {
		b.unsupported("RETURNING")
		return
	}
	idx, bad := returningIndexes(m, r)
	if bad != "" {
		b.SetErr(invalid(bad))
		return
	}
	names := make([]string, len(idx))
	for i, ci := range idx {
		names[i] = m.Columns[ci].Name
	}
	appendReturningNames(b, names)
}

// runReturning executes the compiled statement and wraps its rows.
func runReturning[T any](ctx context.Context, q Querier, op string, m *Meta[T], r returningSet, c Compiled) (*ModelRows[T], error) {
	idx, bad := returningIndexes(m, r)
	if bad != "" { // SQL() validated this; defend against misuse
		return nil, &Error{Kind: ErrValidation, Op: op, Table: m.Table, Err: fmt.Errorf("Returning: column %q is not mapped by %s", bad, m.Table)}
	}
	rows, err := q.conn().query(ctx, op, m.Table, "", c)
	if err != nil {
		return nil, err
	}
	return &ModelRows[T]{rows: rows, meta: m, idx: idx}, nil
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
		return newError(ErrValidation, "scan", "", errors.New("ScanModel into nil model"))
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

// Close releases the cursor. It is safe to call more than once. Deferred
// directly (defer rows.Close()), it also rolls back an implicit transaction
// when a panic unwinds through the caller, so a RETURNING write is never
// committed by a panicking iteration.
func (r *ModelRows[T]) Close() error {
	if p := recover(); p != nil {
		if a, ok := r.rows.(interface{ abort() }); ok {
			a.abort()
		}
		panic(p)
	}
	return r.rows.Close()
}
