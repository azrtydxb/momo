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
// PostgreSQL through FromSQLDB). Model, Models, Values, ValuesRows,
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
	if err := q.copyShape(ms); err != nil {
		return Result{}, err
	}
	if err := q.precheck(); err != nil {
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
	q.models = ms
	q, err := q.generateIDs(false)
	if err != nil {
		return Result{}, err
	}
	names, rows, err := q.copyRows(q.models)
	if err != nil {
		return Result{}, err
	}
	run := func(sq Querier) (Result, error) {
		c, ok := CopierOf(sq)
		if !ok {
			return Result{}, q.noCopy()
		}
		n, err := sq.conn().copyFrom(ctx, c, q.meta.Schema, q.meta.Table, names, rows)
		if err != nil {
			return Result{}, err
		}
		return Result{RowsAffected: n}, nil
	}
	if !bypassesTx(q.q) {
		return run(q.q)
	}
	// An implicit-transaction DB must not take the driver COPY path directly: it
	// would skip WithTxSetup. Run it in a transaction that has been set up. A
	// pinned handle with a setup already failed closed in precheck.
	var res Result
	err = runAtomic(ctx, q.q, "copy", q.meta.Table, func(sq Querier) error {
		var err error
		res, err = run(sq)
		return err
	})
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

func (q InsertQuery[T]) noCopy() error {
	return &Error{Kind: ErrUnsupported, Op: "copy", Table: q.meta.Table,
		Err: errors.New("the driver has no COPY path (momo.Copier)")}
}

// copyShape rejects a query Copy cannot honour, before anything else runs.
func (q InsertQuery[T]) copyShape(ms []T) error {
	i := q.i
	switch {
	case i.err != nil:
		return i.err
	case q.meta.Table == "":
		return q.verr("", "%T has no table", *new(T))
	case len(ms) == 0:
		return q.verr("", "Copy: no rows")
	case len(q.models) > 0 || i.hasValues || i.hasRows || i.sel != nil:
		return q.verr("", "Copy takes its rows as an argument and cannot be combined with Model, Models, Values, ValuesRows or FromSelect")
	case i.conflict != conflictNone || i.upsert || i.returning.active():
		return q.verr("", "Copy cannot be combined with OnConflict, Upsert or Returning")
	}
	return nil
}

// precheck fails with ErrUnsupported when the handle cannot COPY at all, before
// interceptors, hooks, the write mark or a transaction.
func (q InsertQuery[T]) precheck() error {
	switch h := q.q.(type) {
	case *DB:
		if h.pinned {
			if len(h.txSetup) > 0 {
				return errPinnedSetup("copy", q.meta.Table)
			}
			if _, ok := h.ex.q.(Copier); !ok {
				return q.noCopy()
			}
		} else if !hasCopier(h.ex.q) {
			return q.noCopy()
		}
	case *Tx:
		if _, ok := h.dtx.(Copier); !ok {
			return q.noCopy()
		}
	}
	return nil
}

// copyRows resolves and validates the COPY column list and the value rows.
func (q InsertQuery[T]) copyRows(ms []T) ([]string, [][]any, error) {
	fp, err := newFieldPlan(q.q, q.meta)
	if err != nil {
		return nil, nil, err
	}
	r := q
	r.models = ms
	r.i.noRowDefault = true
	r.dbd = nil
	r = r.withDBD() // the plan of the rows being copied
	cols, err := r.modelCols(fp)
	if err != nil {
		return nil, nil, err
	}
	if len(cols) == 0 {
		return nil, nil, q.verr("", "no writable columns to copy")
	}
	for _, ci := range cols {
		if ci != q.meta.CreatedAt && ci != q.meta.UpdatedAt {
			continue
		}
		if err := checkTimestampField(q.meta, ci); err != nil {
			return nil, nil, q.verr(q.meta.Columns[ci].Name, "%v", err)
		}
	}
	now := q.q.conn().now()
	vals := r.modelValues(fp, cols, now)
	if err := r.validateModelRows(fp, cols, vals); err != nil {
		return nil, nil, err
	}
	d := q.q.Dialect()
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
					err = &Error{Kind: ErrUnsupported, Op: "copy", Table: q.meta.Table, Err: err}
				}
				return nil, nil, err
			}
			row[j] = enc
		}
	}
	names := make([]string, len(cols))
	for j, ci := range cols {
		names[j] = q.meta.Columns[ci].Name
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
