// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
)

// scalarQuery returns sq selecting only the expression e, for helpers whose
// destination is a single value instead of the model.
func scalarQuery[T any](sq SelectQuery[T], e Expr) SelectQuery[T] {
	q := sq.Columns(e)
	q.s.keysOnly = true // the select list is not the model's column list
	return q
}

// collectScalars runs q and scans its single column into []V. It honors MaxRows.
func collectScalars[T, V any](ctx context.Context, q SelectQuery[T]) ([]V, error) {
	r, _, err := q.rows(ctx)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var out []V
	for r.Next() {
		if q.s.maxRows > 0 && len(out) >= q.s.maxRows {
			return nil, q.maxRowsError()
		}
		var v V
		if err := r.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	if err := r.Close(); err != nil {
		return nil, err
	}
	return out, nil
}

// Pluck runs sq selecting only the expression c and returns that column of
// every row. The query keeps its WHERE, ORDER BY, LIMIT and MaxRows; Preload
// does not apply. An empty result is a nil slice and a nil error.
func Pluck[T, V any](ctx context.Context, sq SelectQuery[T], c Expression[V]) ([]V, error) {
	return collectScalars[T, V](ctx, scalarQuery(sq, c))
}

// pkColumn returns the single primary-key column of sq's model as a selectable
// expression, or a validation error for models without one or with a
// composite key.
func pkColumn[T any](sq SelectQuery[T]) (Expr, error) {
	if len(sq.meta.PK) != 1 {
		return nil, &Error{Kind: ErrValidation, Op: "select", Table: sq.meta.Table, Err: fmt.Errorf("IDs and FirstID need a single-column primary key, the model has %d", len(sq.meta.PK))}
	}
	return qcol{sq.qualifier(), sq.meta.Columns[sq.meta.PK[0]].Name}, nil
}

// IDs returns the primary-key value of every row sq matches, in the query's
// order. The model needs a single-column integer primary key.
func IDs[T any](ctx context.Context, sq SelectQuery[T]) ([]int64, error) {
	pk, err := pkColumn(sq)
	if err != nil {
		return nil, err
	}
	return collectScalars[T, int64](ctx, scalarQuery(sq, pk))
}

// FirstID returns the primary key of the first row sq matches, or ErrNotFound
// when none matches.
func FirstID[T any](ctx context.Context, sq SelectQuery[T]) (int64, error) {
	ids, err := IDs(ctx, sq.Limit(1))
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, &Error{Kind: ErrNotFound, Op: "select", Table: sq.meta.Table}
	}
	return ids[0], nil
}

// MapRows renders sql through the raw path (see Exec) and returns every row as
// a map from column name to value. Values have the driver's native Go types.
// It needs a driver whose rows implement ColumnNamer, and returns
// ErrUnsupported otherwise. An empty result is a nil slice and a nil error.
func MapRows(ctx context.Context, q Querier, sql string, args ...any) ([]map[string]any, error) {
	c, err := compile(q.Dialect(), func(b *Builder) { b.appendRaw(sql, args) })
	if err != nil {
		return nil, err
	}
	r, err := q.conn().query(ctx, "select", "", "", c)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	cn, ok := r.(ColumnNamer)
	if !ok {
		return nil, &Error{Kind: ErrUnsupported, Op: "select", Err: errors.New("the driver's rows cannot report column names")}
	}
	var out []map[string]any
	for r.Next() {
		names := cn.Columns()
		if len(names) == 0 {
			return nil, &Error{Kind: ErrUnsupported, Op: "select", Err: errors.New("the driver's rows cannot report column names")}
		}
		vals := make([]any, len(names))
		ptrs := make([]any, len(names))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := r.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(names))
		for i, n := range names {
			row[n] = vals[i]
		}
		out = append(out, row)
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	if err := r.Close(); err != nil {
		return nil, err
	}
	return out, nil
}

// FindAndCount returns the rows of sq (with its LIMIT and OFFSET) and the
// total number of rows it matches without them, using two statements. The two
// are not one snapshot: run them in a transaction when a consistent pair is
// required.
func FindAndCount[T any](ctx context.Context, sq SelectQuery[T]) ([]T, int64, error) {
	page, err := sq.All(ctx)
	if err != nil {
		return nil, 0, err
	}
	whole := sq
	whole.s.limit = -1
	whole.s.offset = 0
	total, err := whole.Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	return page, total, nil
}

// pkRef is a primary-key column of the query's own table as an ORDER BY item.
// It is a ColumnRef, so keyset pagination recognizes it as the key instead of
// appending the primary key a second time.
type pkRef struct{ qcol }

func (r pkRef) ColumnName() string { return r.name }
func (r pkRef) Table() string      { return r.table }

// Batches visits every row sq matches in batches of at most n rows, in
// primary-key order, calling fn for each batch. It pages with keyset
// pagination on the primary key, never OFFSET, so each row is visited exactly
// once even while rows with higher keys are inserted, and a deep batch costs
// the same as the first. sq must not set OrderBy, After, Limit or Offset:
// Batches owns the order and the page window. An error from fn stops the
// iteration and is returned.
func Batches[T any](ctx context.Context, sq SelectQuery[T], n int, fn func([]T) error) error {
	if n <= 0 {
		return &Error{Kind: ErrValidation, Op: "select", Table: sq.meta.Table, Err: fmt.Errorf("Batches size must be positive, got %d", n)}
	}
	if len(sq.s.order) > 0 || len(sq.s.keyset) > 0 || sq.s.limit >= 0 || sq.s.offset > 0 {
		return &Error{Kind: ErrValidation, Op: "select", Table: sq.meta.Table, Err: errors.New("Batches owns ORDER BY, After, Limit and Offset; remove them from the query")}
	}
	if len(sq.meta.PK) == 0 || sq.meta.Value == nil {
		return &Error{Kind: ErrValidation, Op: "select", Table: sq.meta.Table, Err: errors.New("Batches needs a model with a primary key")}
	}
	q := sq.qualifier()
	items := make([]Orderable, len(sq.meta.PK))
	for i, ci := range sq.meta.PK {
		items[i] = Order{Expr: pkRef{qcol{q, sq.meta.Columns[ci].Name}}}
	}
	base := sq.OrderBy(items...).Limit(int64(n))
	page := base
	for {
		rows, err := page.All(ctx)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		if err := fn(rows); err != nil {
			return err
		}
		if len(rows) < n {
			return nil
		}
		last := &rows[len(rows)-1]
		after := make([]any, len(sq.meta.PK))
		for i, ci := range sq.meta.PK {
			after[i] = sq.meta.Value(last, ci)
		}
		page = base.After(after...)
	}
}
