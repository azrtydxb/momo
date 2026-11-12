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
// and scans every row into a T. T is a model, a projection struct, one of
// the tuple rows Row1 to Row10, or a plain struct. It is the multi-column
// counterpart of Scalars for a hand-written SELECT or a statement with RETURNING:
//
//	rows, err := momo.Query[momo.Row2[string, int64]](ctx, db,
//		`SELECT title, count(*) FROM documents WHERE tenant_id = $1 GROUP BY title`, tenant)
//
// Columns are matched as for a typed select. A tuple row scans positionally,
// and the result must have exactly as many columns as the row has fields.
//
// A plain struct, one with no generated metadata, scans by column name too, with
// no generator run: its exported fields are the columns, named by a
// `momo:"column:x"` tag or else the column name the DB's naming preset gives
// the field name (WithNaming; snake_case without one, so UserID is user_id); `momo:"-"` skips a field; the fields of an embedded struct count as
// the outer struct's own, unless it is a time.Time, a sql.Scanner or carries a
// column tag; an embedded pointer struct is allocated for every row
// and stays non-nil even when all of its columns are NULL (an embedding cycle
// is an ErrValidation); a pointer field receives NULL as nil (a non-pointer
// field that meets NULL is the driver's scan error). The same rule as below
// applies: a result column no field maps to, or a field no result column
// fills, is an ErrValidation that names it. The reflection plan is built once
// per type and cached, and the generated metadata of a model, projection or
// tuple row is always preferred when the type has it.
//
// A model or projection scans by column name, whatever order the SQL lists
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
func Query[T any](ctx context.Context, q Querier, sql string, args ...any) ([]T, error) {
	c, err := compile(q.Dialect(), func(b *Builder) { b.appendRaw(sql, args) })
	if err != nil {
		return nil, err
	}
	return scanCompiled[T](ctx, q, "raw", "", c)
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
func QueryRows[T any](ctx context.Context, q Querier, sql string, args ...any) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		r, sc, err := openRaw[T](ctx, q, sql, args)
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

// ColumnRows is the cursor RawRows returns: a Rows that also names its result
// columns.
type ColumnRows interface {
	Rows
	ColumnNamer
}

// RawRows runs sql through the raw path and returns the open cursor, for a
// result whose shape is only known at run time: a statement whose column
// count or types depend on its input, which Query and QueryRows cannot type
// and MapRows would buffer into maps. Placeholders, hooks (op "raw"),
// interceptors, WithTxSetup, WithImplicitTx, error classification and
// argument redaction are those of Query, on a *DB, a *Tx or an AdoptTx handle.
//
//	rows, err := momo.RawRows(ctx, db, sql, args...)
//	if err != nil {
//		return err
//	}
//	defer rows.Close()
//	for rows.Next() {
//		if err := rows.Scan(dest...); err != nil {
//			return err
//		}
//	}
//	return rows.Err()
//
// The caller closes the cursor, always: a cursor that is never closed holds
// its connection, and on a *DB with WithImplicitTx the transaction behind it.
// Close is safe to call more than once. Raw SQL may write, so under
// WithImplicitTx momo treats the statement as a write: the transaction commits
// when the rows are read to the end and Close returns nil, and it is rolled
// back, with Close returning ErrValidation, when Close comes before the end of
// the rows (including a Close with no Next), after a Scan or driver error, or
// after a canceled context. Without WithImplicitTx an early Close is just a
// Close. A canceled context ends the iteration: Next returns false, the cursor
// is closed and Err reports ErrCanceled; a context canceled after the last row
// was read can still report ErrCanceled, as in QueryRows. Scan errors go
// through the DB's error classifier like any other statement error and are
// recorded on the hook event. The rows bypass soft-delete filters, AfterLoad
// hooks and timestamps, and MaxRows does not apply. Columns reports the result
// column names as the driver gives them: an empty list means the statement
// returns no columns (an UPDATE without RETURNING) or the driver cannot name
// them, and ErrUnsupported means the cursor does not implement ColumnNamer.
func RawRows(ctx context.Context, q Querier, sql string, args ...any) (ColumnRows, error) {
	c, err := compile(q.Dialect(), func(b *Builder) { b.appendRaw(sql, args) })
	if err != nil {
		return nil, err
	}
	e := q.conn()
	r, err := e.query(ctx, "raw", "", "", c)
	if err != nil {
		return nil, err
	}
	return &rawCursor{r: r, ctx: ctx, e: e}, nil
}

// rawCursor adds column names and context cancellation to the rows of a raw
// statement.
type rawCursor struct {
	r      Rows
	ctx    context.Context
	e      *execer
	cancel error
}

func (c *rawCursor) Next() bool {
	if c.cancel != nil {
		return false
	}
	if err := c.ctx.Err(); err != nil {
		c.cancel = newError(ErrCanceled, "raw", "", err)
		abortRawRows(c.r, c.cancel)
		return false
	}
	return c.r.Next()
}

func (c *rawCursor) Scan(dest ...any) error {
	return c.e.wrapErr("raw", "", c.r.Scan(dest...))
}

func (c *rawCursor) Err() error {
	if c.cancel != nil {
		return c.cancel
	}
	return c.r.Err()
}

func (c *rawCursor) Close() error { return c.r.Close() }

func (c *rawCursor) Columns() ([]string, error) {
	if cn, ok := c.r.(ColumnNamer); ok {
		names, err := cn.Columns()
		return names, c.e.wrapErr("raw", "", err)
	}
	return nil, newError(ErrUnsupported, "raw", "", errors.New("the driver cannot report result column names"))
}

// openRaw runs sql and binds the result columns to T's fields. On an error
// the rows are already closed.
func openRaw[T any](ctx context.Context, q Querier, sql string, args []any) (Rows, *rawScan[T], error) {
	c, err := compile(q.Dialect(), func(b *Builder) { b.appendRaw(sql, args) })
	if err != nil {
		return nil, nil, err
	}
	return openCompiled[T](ctx, q, "raw", "", c)
}

// openCompiled runs a compiled statement under hook operation op and binds the
// result columns to T's fields. On an error the rows are already closed.
func openCompiled[T any](ctx context.Context, q Querier, op, table string, c Compiled) (Rows, *rawScan[T], error) {
	meta, err := metaFor[T](q.conn().db)
	if err != nil {
		return nil, nil, err
	}
	r, err := q.conn().query(ctx, op, table, "", c)
	if err != nil {
		return nil, nil, err
	}
	var names []string
	if cn, ok := r.(ColumnNamer); ok {
		if names, err = cn.Columns(); err != nil {
			abortRawRows(r, err)
			return nil, nil, err
		}
	}
	sc, err := newRawScan(meta, names)
	if err != nil {
		abortRawRows(r, err)
		return nil, nil, err
	}
	return r, sc, nil
}

// abortRawRows closes r after a failure that happened before any row was read.
// Rows opened under a hook report err to AfterQuery; a plain cursor is closed.
func abortRawRows(r Rows, err error) {
	if t, ok := r.(*txRows); ok {
		// WithImplicitTx: report to the hook pair inside, then end the
		// transaction (txRows.Close rolls it back, the context is done).
		if h, ok := t.Rows.(*hookedRows); ok {
			h.abort(err)
		}
		_ = t.Close()
		return
	}
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
