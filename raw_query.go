// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
)

// Query renders sql through the raw path (see Exec: '?' binds, "??" is a
// literal '?', Expr arguments render inline, an argument-count mismatch is a
// build error; on PostgreSQL native $n placeholders bind argument n, see SQL)
// and scans every row into a T. T is a model, a projection struct, or one of
// the tuple rows Row1 to Row10. It is the multi-column counterpart of Scalars
// for a hand-written SELECT or a statement with RETURNING:
//
//	rows, err := momo.Query[momo.Row2[string, int64]](ctx, db,
//		`SELECT title, count(*) FROM documents WHERE tenant_id = $1 GROUP BY title`, tenant)
//
// Columns are matched as for a typed select. A tuple row scans positionally,
// and the result must have exactly as many columns as the row has fields. A
// model or projection scans by column name, whatever order the SQL lists
// them (names compare exactly, then case-insensitively, since PostgreSQL
// folds unquoted names): a result column that no field maps to, a mapped
// column the result lacks, or two result columns with the same name is an
// ErrValidation. A driver whose rows cannot report column names falls back to
// the positional order of the type's columns.
//
// The statement runs with the same hooks, interceptors, error classification
// and argument redaction as Exec, on a *DB, a *Tx or a pinned handle; its
// hook event has the op "raw". The rows bypass soft-delete filters, AfterLoad
// hooks and timestamps. Query buffers the whole result in memory and ignores
// MaxRows, so bound the SQL with LIMIT or use QueryRows. An empty result is a
// nil slice and a nil error.
func Query[T any, PT Modeler[T]](ctx context.Context, q Querier, sql string, args ...any) ([]T, error) {
	r, sc, err := openRaw[T, PT](ctx, q, sql, args)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	var out []T
	for {
		if err := ctx.Err(); err != nil {
			return nil, newError(ErrCanceled, "raw", "", err)
		}
		if !r.Next() {
			break
		}
		var v T
		if err := sc.scan(r, &v); err != nil {
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

// QueryRows is the streaming form of Query: it yields one T per row without
// buffering the result. Breaking out of the loop closes the cursor, and an
// error (a build, driver, scan or column-matching error, or a canceled
// context as ErrCanceled) is yielded once, as the final element. The
// placeholder, matching, hook and redaction rules are those of Query.
//
//	for row, err := range momo.QueryRows[momo.Row2[int64, string]](ctx, db,
//		`SELECT id, title FROM documents ORDER BY id`) {
//		if err != nil {
//			return err
//		}
//		use(row.C1, row.C2)
//	}
func QueryRows[T any, PT Modeler[T]](ctx context.Context, q Querier, sql string, args ...any) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		r, sc, err := openRaw[T, PT](ctx, q, sql, args)
		if err != nil {
			yield(zero, err)
			return
		}
		defer func() { _ = r.Close() }()
		for {
			if err := ctx.Err(); err != nil {
				yield(zero, newError(ErrCanceled, "raw", "", err))
				return
			}
			if !r.Next() {
				break
			}
			var v T
			if err := sc.scan(r, &v); err != nil {
				yield(zero, err)
				return
			}
			if !yield(v, nil) {
				return
			}
		}
		if err := r.Err(); err != nil {
			yield(zero, err)
			return
		}
		if err := r.Close(); err != nil {
			yield(zero, err)
		}
	}
}

// openRaw runs sql and binds the result columns to T's fields. On an error
// the rows are already closed.
func openRaw[T any, PT Modeler[T]](ctx context.Context, q Querier, sql string, args []any) (Rows, *rawScan[T], error) {
	c, err := compile(q.Dialect(), func(b *Builder) { b.appendRaw(sql, args) })
	if err != nil {
		return nil, nil, err
	}
	r, err := q.conn().query(ctx, "raw", "", "", c)
	if err != nil {
		return nil, nil, err
	}
	meta := metaOf[T, PT]()
	var names []string
	if cn, ok := r.(ColumnNamer); ok {
		if names, err = cn.Columns(); err != nil {
			abortRows(r, err)
			return nil, nil, err
		}
	}
	sc, err := newRawScan(meta, names)
	if err != nil {
		abortRows(r, err)
		return nil, nil, err
	}
	return r, sc, nil
}

// abortRows closes r after a failure that happened before any row was read.
// Rows opened under a hook report err to AfterQuery; a plain cursor is closed.
func abortRows(r Rows, err error) {
	if h, ok := r.(*hookedRows); ok {
		h.abort(err)
		return
	}
	_ = r.Close()
}

// rawScan scans raw result rows into a T through the type's metadata.
type rawScan[T any] struct {
	meta *Meta[T]
	// order[j] is the index of the meta column that result column j fills.
	order []int
	dest  []any
}

func newRawScan[T any](meta *Meta[T], names []string) (*rawScan[T], error) {
	n := len(meta.Columns)
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	switch {
	case len(names) == 0:
		// The driver cannot name its columns: positional, and the driver's
		// Scan reports a count mismatch.
	case loadTupleMeta[T]() == meta:
		if len(names) != n {
			return nil, &Error{Kind: ErrValidation, Op: "raw", Err: fmt.Errorf("the result has %d columns but the tuple row has %d", len(names), n)}
		}
	default:
		var err error
		if order, err = matchRawColumns(meta.Table, meta.Columns, names); err != nil {
			return nil, err
		}
	}
	return &rawScan[T]{meta: meta, order: order, dest: make([]any, len(order))}, nil
}

func (s *rawScan[T]) scan(r Rows, m *T) error {
	for j, i := range s.order {
		s.dest[j] = s.meta.Ptr(m, i)
	}
	return r.Scan(s.dest...)
}

// matchRawColumns maps each result column to the meta column it fills.
func matchRawColumns(table string, cols []ColumnMeta, names []string) ([]int, error) {
	exact := make(map[string]int, len(cols))
	folded := make(map[string]int, len(cols))
	for i, c := range cols {
		exact[c.Name] = i
		folded[strings.ToLower(c.Name)] = i
	}
	order := make([]int, len(names))
	used := make([]bool, len(cols))
	var unknown, dup []string
	for j, name := range names {
		i, ok := exact[name]
		if !ok {
			i, ok = folded[strings.ToLower(name)]
		}
		switch {
		case !ok:
			unknown = append(unknown, name)
		case used[i]:
			dup = append(dup, name)
		default:
			used[i] = true
			order[j] = i
		}
	}
	var missing []string
	for i, u := range used {
		if !u {
			missing = append(missing, cols[i].Name)
		}
	}
	if len(unknown)+len(dup)+len(missing) == 0 {
		return order, nil
	}
	var parts []string
	if len(unknown) > 0 {
		parts = append(parts, "result columns that map to no field: "+strings.Join(unknown, ", "))
	}
	if len(dup) > 0 {
		parts = append(parts, "result columns named twice: "+strings.Join(dup, ", "))
	}
	if len(missing) > 0 {
		parts = append(parts, "fields the result lacks: "+strings.Join(missing, ", "))
	}
	return nil, &Error{Kind: ErrValidation, Op: "raw", Table: table, Err: errors.New(strings.Join(parts, "; "))}
}
