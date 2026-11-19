// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/azrtydxb/momo/vector"
)

// Copy streams ms through the driver's Copier (PostgreSQL COPY) and reports
// the rows written. It writes the same columns as a model insert (Omit
// applies, identity and read-only columns are never written, an unset
// created-at or updated-at field takes the statement time, Go-side defaults
// and field validators run) but binds no parameters, so there is no
// parameter limit and no chunking: one COPY is one atomic statement.
//
// Copy returns ErrUnsupported when the driver has no COPY path (SQLite, and
// PostgreSQL through pgxdriver.NewSQL). Model, Models, Values, ValuesRows,
// FromSelect, Returning, OnConflict and Upsert cannot be combined with it
// (ErrValidation): COPY has no conflict handling and returns no rows.
//
// Copy respects WithTxSetup. On a DB made with WithImplicitTx it runs inside
// its own short transaction, so the setup functions run before the COPY. On a
// DB pinned with OnConn that has WithTxSetup it fails closed with
// ErrUnsupported, because the setup could not run. On a Tx it uses the
// transaction's own COPY path. Query hooks see one event with Op "copy" and no
// arguments, so no value reaches a hook.
//
// Copy runs the insert interceptors (they can deny it, but a predicate they
// add is ignored, as for any insert) and the BeforeInsert hook of the model
// over a copy of ms, so a hook that stamps a tenant applies to every row. It
// skips OnConflict handling, RETURNING and the chunking of Exec. It counts as a
// write for NewUnitOfWork, so later reads on a Replica handle in the same unit
// of work stay on the primary.
func (q InsertQuery[T]) Copy(ctx context.Context, ms []T) (Result, error) {
	if q.q == nil {
		return Result{}, noQuerier("copy", q.meta.Table)
	}
	if err := viewWriteErr("copy", q.meta); err != nil {
		return Result{}, err
	}
	if q.i.err == nil && q.meta.Table == "" {
		return Result{}, insertVerr("", "", "%T has no table", *new(T))
	}
	r := q.ref()
	if err := r.copyShape(len(ms)); err != nil {
		return Result{}, err
	}
	if err := copyPrecheck(q.q, q.meta.Table); err != nil {
		return Result{}, err
	}
	if _, err := runInterceptors(ctx, q.q, "insert", q.meta.Table); err != nil {
		return Result{}, err
	}
	if fn := q.meta.Hooks.BeforeInsert; fn != nil {
		// Run the hook over copies, like Models does, so the caller's slice is
		// untouched and a tenant-stamping hook applies to every copied row.
		ms = slices.Clone(ms)
		for i := range ms {
			if err := runModelHook(ctx, "insert", q.meta.Table, fn, &ms[i]); err != nil {
				return Result{}, err
			}
		}
	}
	// Fill the Go-side ids (cuid, uuid, ...) like Models and Unnest do. It
	// works on a clone, so the caller's slice is untouched.
	meta := q.meta
	fp, ferr := newFieldPlan(q.q, meta)
	a := insAccess{
		value:   func(row, ci int) any { return meta.Value(&ms[row], ci) },
		zero:    func(ci int) (any, bool) { return zeroValue(meta, ci) },
		def:     func(ci, row int) any { return fieldDefault(&fp, ms, ci, row) },
		ptr:     func(row, ci int) any { return meta.Ptr(&ms[row], ci) },
		hasBack: func(int) bool { return false },
		clone:   func() { ms = slices.Clone(ms) },
		fields:  fp.refOf(len(meta.Columns)),
	}
	r.ferr = ferr
	r.n, r.nBack = len(ms), 0
	r, err := r.generateIDs(false, a)
	if err != nil {
		return Result{}, err
	}
	names, rows, err := r.copyRows(a)
	if err != nil {
		return Result{}, err
	}
	return r.copyExec(ctx, names, rows)
}

// noCopyErr is the error of a handle without a COPY path.
func noCopyErr(table string) error {
	return &Error{Kind: ErrUnsupported, Op: "copy", Table: table,
		Err: errors.New("the driver has no COPY path (momo.Copier)")}
}

// copyShape rejects a query Copy cannot honour, before anything else runs.
func (r insertRef) copyShape(rows int) error {
	i := r.i
	switch {
	case i.err != nil:
		return i.err
	case rows == 0:
		return r.verr("", "Copy: no rows")
	case r.n > 0 || i.hasValues || i.hasRows || i.sel != nil:
		return r.verr("", "Copy takes its rows as an argument and cannot be combined with Model, Models, Values, ValuesRows or FromSelect")
	case len(i.explicit) > 0:
		return r.verr("", "Copy cannot be combined with Explicit")
	case i.conflict != conflictNone || i.upsert || i.returning.active():
		return r.verr("", "Copy cannot be combined with OnConflict, Upsert or Returning")
	}
	return nil
}

// copyPrecheck fails with ErrUnsupported when the handle cannot COPY at all,
// before interceptors, hooks, the write mark or a transaction.
func copyPrecheck(q Querier, table string) error {
	switch h := q.(type) {
	case *DB:
		if h.pinned {
			if len(h.txSetup) > 0 {
				return errPinnedSetup("copy", table)
			}
			if _, ok := h.ex.q.(Copier); !ok {
				return noCopyErr(table)
			}
		} else if !hasCopier(h.ex.q) {
			return noCopyErr(table)
		}
	case *Tx:
		if _, ok := h.dtx.(Copier); !ok {
			return noCopyErr(table)
		}
	}
	return nil
}

// copyExec runs the COPY of the resolved column names and value rows.
func (r insertRef) copyExec(ctx context.Context, names []string, rows [][]any) (Result, error) {
	run := func(sq Querier) (Result, error) {
		c, ok := CopierOf(sq)
		if !ok {
			return Result{}, noCopyErr(r.m.Table)
		}
		n, err := sq.conn().copyFrom(ctx, c, r.m.Schema, r.m.Table, names, rows)
		if err != nil {
			return Result{}, err
		}
		return Result{RowsAffected: n}, nil
	}
	if !bypassesTx(r.q) {
		return run(r.q)
	}
	// An implicit-transaction DB must not take the driver COPY path directly: it
	// would skip WithTxSetup. Run it in a transaction that has been set up. A
	// pinned handle with a setup already failed closed in precheck.
	var res Result
	err := runAtomic(ctx, r.q, "copy", r.m.Table, func(sq Querier) error {
		var err error
		res, err = run(sq)
		return err
	})
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

// copyRows resolves and validates the COPY column list and the value rows.
func (r insertRef) copyRows(a insAccess) ([]string, [][]any, error) {
	if r.ferr != nil {
		return nil, nil, r.ferr
	}
	r.i.noRowDefault = true
	r.dbd = nil
	r = r.withDBD(a) // the plan of the rows being copied
	cols, err := r.modelCols(a)
	if err != nil {
		return nil, nil, err
	}
	if len(cols) == 0 {
		return nil, nil, r.verr("", "no writable columns to copy")
	}
	for _, ci := range cols {
		if ci != r.m.CreatedAt && ci != r.m.UpdatedAt {
			continue
		}
		if err := r.checkTimestamp(ci, a); err != nil {
			return nil, nil, r.verr(r.m.Columns[ci].Name, "%v", err)
		}
	}
	now := r.q.conn().now()
	vals := r.modelValues(cols, now, a)
	if err := r.validateModelRows(cols, vals, a); err != nil {
		return nil, nil, err
	}
	d := r.q.Dialect()
	for _, row := range vals {
		for j, v := range row {
			if isNilVectorPointer(v) {
				row[j] = nil // a nil pointer would panic in the promoted DialectValue
				continue
			}
			dv, ok := v.(DialectValuer)
			if !ok {
				continue
			}
			enc, err := dv.DialectValue(d.Name)
			if err != nil {
				if errors.Is(err, vector.ErrDialect) {
					err = &Error{Kind: ErrUnsupported, Op: "copy", Table: r.m.Table, Err: err}
				}
				return nil, nil, err
			}
			row[j] = enc
		}
	}
	names := make([]string, len(cols))
	for j, ci := range cols {
		names[j] = r.m.Columns[ci].Name
	}
	return names, vals, nil
}

// copyFrom runs a COPY on c and reports it to the hooks. The event carries a
// descriptive statement and no arguments, so a hook never sees a value.
func (e *execer) copyFrom(ctx context.Context, c Copier, schema, model string, cols []string, rows [][]any) (int64, error) {
	ident := func(s string) string { return string(e.d.AppendIdent(nil, s)) }
	table, quoted := model, ident(model)
	if schema != "" {
		table, quoted = schema+"."+model, ident(schema)+"."+quoted
	}
	quotedCols := make([]string, len(cols))
	for i, n := range cols {
		quotedCols[i] = ident(n)
	}
	sql := "COPY " + quoted + " (" + strings.Join(quotedCols, ", ") + ") FROM STDIN"
	markWrite(ctx) // read-your-writes: later reads in this unit of work stay on the primary
	if err := ctx.Err(); err != nil {
		return -1, e.wrapErr("copy", model, err)
	}
	ctx, ev := e.before(ctx, "copy", model, "", sql, nil, nil)
	n, err := c.CopyFrom(ctx, table, cols, rows)
	err = e.wrapErr("copy", model, err)
	if err != nil {
		n = -1
	}
	e.after(ctx, ev, n, err)
	return n, err
}
