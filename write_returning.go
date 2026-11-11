// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/azrtydxb/momo/dialect"
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
	if !b.Dialect().Returning && !(b.insertStmt && b.Dialect().InsertReturning) {
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
	return &ModelRows[T]{rows: rows, meta: m, idx: idx, ctx: ctx}, nil
}

// ModelRows is the cursor of a write statement with a RETURNING list. Each
// row scans into the model fields named by the list; other fields are left
// untouched.
type ModelRows[T any] struct {
	rows Rows
	meta *Meta[T]
	idx  []int
	dest []any
	// ctx is the statement's context, passed to the AfterLoad hook.
	ctx context.Context
	// noHooks skips the AfterLoad hook: internal reads of RETURNING rows (the
	// id-generator reconcile) scan half-filled models that no hook may see.
	noHooks bool
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
	if err := r.rows.Scan(r.dest...); err != nil {
		return err
	}
	if r.noHooks {
		return nil
	}
	return afterLoad(r.ctx, r.meta, m)
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
		} else {
			// Not an implicit transaction: still release the cursor, the
			// connection and the AfterQuery report while the panic unwinds.
			_ = r.rows.Close()
		}
		panic(p)
	}
	return r.rows.Close()
}

// oneRow reads the single row of a RETURNING statement into a new T. It
// returns ErrNotFound when no row came back and ErrTooManyRows when more than
// one did; in that case an implicit transaction is rolled back, so a write
// that touched several rows is not committed. Inside an explicit transaction
// the caller decides. The rows are always closed.
func oneRow[T any](r *ModelRows[T], op, table string) (T, error) {
	var zero, m T
	defer func() { _ = r.Close() }()
	if !r.Next() {
		if err := r.Err(); err != nil {
			return zero, err
		}
		return zero, &Error{Kind: ErrNotFound, Op: op, Table: table}
	}
	if err := r.ScanModel(&m); err != nil {
		return zero, err
	}
	if r.Next() {
		if a, ok := r.rows.(interface{ abort() }); ok {
			a.abort()
		}
		return zero, &Error{Kind: ErrTooManyRows, Op: op, Table: table}
	}
	if err := r.Err(); err != nil {
		return zero, err
	}
	if err := r.Close(); err != nil {
		return zero, err
	}
	return m, nil
}

// One runs a single-row INSERT and returns its RETURNING row: ErrNotFound when
// no row came back (ON CONFLICT DO NOTHING, or a DO UPDATE whose WHERE did
// not hold) and ErrTooManyRows when more than one did. Without a Returning
// list it returns every column, as ReturningAll does. On a dialect without
// RETURNING (MySQL, and MariaDB for UPDATE and DELETE, where momo has no
// RETURNING form) it fails with ErrUnsupported.
//
// A multi-row Models insert is refused with ErrValidation before anything runs:
// a bulk insert can be split into chunks that commit together, which One
// cannot roll back after the fact. Use Rows for a bulk insert. AfterLoad runs
// on the returned row before the write commits, so an AfterLoad error rolls an
// implicit transaction back, unlike SelectQuery.One.
func (q InsertQuery[T]) One(ctx context.Context) (T, error) {
	var zero T
	if len(q.models) > 1 {
		return zero, q.verr("", "One reads one row; use Rows for a bulk insert")
	}
	if !q.i.returning.active() {
		q = q.ReturningAll()
	}
	rows, err := q.Rows(ctx)
	if err != nil {
		return zero, err
	}
	return oneRow(rows, "insert", q.meta.Table)
}

// One runs the UPDATE and returns the single RETURNING row: ErrNotFound when
// no row matched (a guarded update) and ErrTooManyRows when more than one
// did. Without a Returning list it returns every column. It is not supported
// with a version-checked Model update (ErrValidation; use Exec) or on a
// dialect without RETURNING (ErrUnsupported: MySQL, and MariaDB, where momo
// has no RETURNING form for UPDATE).
//
// Without WithImplicitTx or an explicit transaction the statement has already
// changed every matched row when One returns ErrTooManyRows; run One inside
// db.Tx and roll back on ErrTooManyRows. AfterLoad runs on the returned row
// before the write commits, so an AfterLoad error rolls an implicit
// transaction back, unlike SelectQuery.One.
func (u UpdateQuery[T]) One(ctx context.Context) (T, error) {
	var zero T
	if !u.returning.active() {
		u = u.ReturningAll()
	}
	rows, err := u.Rows(ctx)
	if err != nil {
		return zero, err
	}
	return oneRow(rows, "update", u.meta.Table)
}

// One runs the DELETE and returns the single RETURNING row: ErrNotFound when
// no row matched and ErrTooManyRows when more than one did. Without a
// Returning list it returns every column. On a dialect without RETURNING
// (MySQL, and MariaDB, where momo has no RETURNING form for DELETE) it fails
// with ErrUnsupported.
//
// Without WithImplicitTx or an explicit transaction the statement has already
// changed every matched row when One returns ErrTooManyRows; run One inside
// db.Tx and roll back on ErrTooManyRows. AfterLoad runs on the returned row
// before the write commits, so an AfterLoad error rolls an implicit
// transaction back, unlike SelectQuery.One.
func (d DeleteQuery[T]) One(ctx context.Context) (T, error) {
	var zero T
	if !d.returning.active() {
		d = d.ReturningAll()
	}
	rows, err := d.Rows(ctx)
	if err != nil {
		return zero, err
	}
	return oneRow(rows, "delete", d.meta.Table)
}

// isSourceColumn reports whether c belongs to a source other than the target
// table: its qualifier is set and is not the table's name.
func isSourceColumn(c ColumnRef, table string) bool {
	rc, ok := c.(interface{ ref() colRef })
	if !ok {
		return false
	}
	q := rc.ref().table
	return q != "" && q != table
}

// hasSource reports whether the list names a column of another source.
func (r returningSet) hasSource(table string) bool {
	return slices.ContainsFunc(r.cols, func(c ColumnRef) bool { return isSourceColumn(c, table) })
}

// renderWriteReturning is renderReturning for UPDATE and DELETE, which can
// have FROM / USING sources (hasSources). The list may then name source
// columns, rendered with their own qualifier, in the order given; under
// ReturningAll the target's columns come first. On PostgreSQL a statement with
// sources qualifies the target's columns too, since a name the source shares
// would be ambiguous. Elsewhere a source column is ErrUnsupported: SQLite's
// RETURNING reads the modified table only.
func renderWriteReturning[T any](b *Builder, m *Meta[T], r returningSet, hasSources bool, invalid func(col string) error) {
	if !r.active() {
		return
	}
	if !b.Dialect().Returning {
		b.unsupported("RETURNING")
		return
	}
	pg := b.d.Name == dialect.Postgres
	qual := ""
	if hasSources && pg {
		qual = m.Table
	}
	if r.hasSource(m.Table) {
		if !hasSources {
			for _, c := range r.cols {
				if isSourceColumn(c, m.Table) {
					b.SetErr(invalid(c.ColumnName()))
					return
				}
			}
		}
		if !pg {
			b.unsupported("RETURNING columns of a FROM or USING source")
			return
		}
	}
	prev := b.unqualified
	b.unqualified = false
	defer func() { b.unqualified = prev }()
	b.WriteString(" RETURNING ")
	n := 0
	next := func() {
		if n > 0 {
			b.WriteString(", ")
		}
		n++
	}
	if r.all {
		for _, c := range m.Columns {
			next()
			b.QualifiedIdent(qual, c.Name)
		}
	}
	for _, c := range r.cols {
		switch {
		case isSourceColumn(c, m.Table):
			next()
			c.AppendSQL(b)
		case r.all:
		default:
			name := c.ColumnName()
			if !slices.ContainsFunc(m.Columns, func(mc ColumnMeta) bool { return mc.Name == name }) {
				b.SetErr(invalid(name))
				return
			}
			next()
			b.QualifiedIdent(qual, name)
		}
	}
}

// scanCompiled runs a RETURNING statement and scans every row into an R by
// column name, as Query does for raw SQL.
func scanCompiled[R any](ctx context.Context, q Querier, op, table string, c Compiled) ([]R, error) {
	r, sc, err := openCompiled[R](ctx, q, op, table, c)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	var out []R
	for {
		if err := ctx.Err(); err != nil {
			return nil, newError(ErrCanceled, op, table, err)
		}
		if !r.Next() {
			break
		}
		var v R
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

// sourceQualifiers lists the names the FROM / USING sources expose, for the
// statement's scope. A source without a Qualifier has none.
func sourceQualifiers(srcs []TableSource) []string {
	var out []string
	for _, s := range srcs {
		if q, ok := s.(interface{ Qualifier() string }); ok {
			out = append(out, q.Qualifier())
		}
	}
	return out
}

// appendSources writes the sources as a comma list.
func appendSources(b *Builder, srcs []TableSource) {
	for i, s := range srcs {
		if i > 0 {
			b.WriteString(", ")
		}
		s.appendFrom(b)
	}
}

// unconstrainedSource returns the qualifier of the first of several FROM /
// USING sources that no predicate references, or "". A single source is not
// checked. Of several, a source with no Qualifier cannot be shown to be
// referenced, so it counts as unconstrained, and is reported as "an unnamed
// source".
func unconstrainedSource(d *dialect.Dialect, ps []Predicate, srcs []TableSource) string {
	if len(srcs) < 2 {
		return ""
	}
	for _, s := range srcs {
		q, ok := s.(interface{ Qualifier() string })
		if !ok || q.Qualifier() == "" {
			return "an unnamed source"
		}
		if !referencesTable(d, ps, q.Qualifier()) {
			return q.Qualifier()
		}
	}
	return ""
}
