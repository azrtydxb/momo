// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
)

// Exec runs the statement and reports the rows it inserted.
//
// A Models call whose rows would exceed the dialect's bind-parameter limit
// is split into several multi-row statements, run in order, and the rows
// affected are summed. The chunks are atomic: on a DB they run in one
// transaction, on a Tx inside a savepoint (when the dialect has them), so a
// failing chunk undoes the earlier ones. A Tx on a dialect without
// savepoints runs the chunks directly in the caller's transaction, which
// stays atomic as a whole. A DB pinned to one connection by OnConn cannot
// begin a transaction, so a chunked insert there fails with ErrUnsupported
// before any statement runs. A panic during a chunk rolls the scope back and
// is re-raised. All chunks share one clock read, so every row gets the same automatic
// timestamp. A single row with more bound columns than the limit fails with
// ErrLimitExceeded.
func (q InsertQuery[T]) Exec(ctx context.Context) (Result, error) {
	q, err := q.beforeWrite(ctx)
	if err != nil {
		return Result{}, err
	}
	chunks, err := q.plan()
	if err != nil {
		return Result{}, err
	}
	var pend []func()
	for i := range chunks {
		chunks[i].pend = &pend
	}
	if len(chunks) == 1 {
		res, err := q.execSingle(ctx, chunks[0])
		if err != nil {
			return Result{}, err
		}
		for _, apply := range pend {
			apply()
		}
		return res, nil
	}
	var total int64
	err = runAtomic(ctx, q.q, "insert", q.meta.Table, func(sq Querier) error {
		for _, c := range chunks {
			c.q = sq
			r, err := c.execOne(ctx)
			if err != nil {
				return err
			}
			total += r.RowsAffected
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	for _, apply := range pend {
		apply()
	}
	return Result{RowsAffected: total}, nil
}

func (q InsertQuery[T]) execOne(ctx context.Context) (Result, error) {
	if q.reconcile {
		return q.execReconcile(ctx)
	}
	c, err := q.SQL()
	if err != nil {
		return Result{}, err
	}
	if cols := q.dbDefaultReadBack(); len(cols) > 0 {
		return q.execReadBack(ctx, cols)
	}
	if q.wantsGeneratedKey() {
		n, id, err := q.q.conn().execInsertID(ctx, "insert", q.meta.Table, c)
		if err != nil {
			return Result{}, err
		}
		if n > 0 && id != 0 {
			q.assignGeneratedKey(id)
		}
		return Result{RowsAffected: n, InsertID: id}, nil
	}
	n, err := q.q.conn().exec(ctx, "insert", q.meta.Table, c)
	if err != nil {
		return Result{}, err
	}
	return Result{RowsAffected: n}, nil
}

// Rows runs the statement and returns its RETURNING rows. It fails with
// ErrValidation when no RETURNING list was set. A chunked bulk insert (see
// Exec) returns the rows of all chunks in insertion order. The first chunk
// runs inside Rows, so its errors are returned here; the remaining chunks run
// as the rows are consumed, and Close runs any that are left so the insert
// stays whole. The caller MUST check the error from Close (and Err): a later
// chunk can fail there, which rolls the whole insert back.
func (q InsertQuery[T]) Rows(ctx context.Context) (*ModelRows[T], error) {
	if !q.i.returning.active() {
		return nil, q.verr("", "Rows needs a Returning list")
	}
	q, err := q.beforeWrite(ctx)
	if err != nil {
		return nil, err
	}
	chunks, err := q.plan()
	if err != nil {
		return nil, err
	}
	for i := range chunks {
		chunks[i].reconcile = false // the caller reads the stored ids from the rows
	}
	if len(chunks) == 1 {
		return chunks[0].rowsOne(ctx)
	}
	sc, err := beginAtomic(ctx, q.q, "insert", q.meta.Table)
	if err != nil {
		return nil, err
	}
	idx, _ := returningIndexes(q.meta, q.i.returning)
	for i := range chunks {
		chunks[i].q = sc.q
	}
	cr := &chunkRows[T]{ctx: ctx, chunks: chunks, sc: sc}
	if err := cr.open(); err != nil {
		cr.done = true
		return nil, sc.finish(context.WithoutCancel(ctx), err)
	}
	return &ModelRows[T]{rows: cr, meta: q.meta, idx: idx, ctx: ctx}, nil
}

func (q InsertQuery[T]) rowsOne(ctx context.Context) (*ModelRows[T], error) {
	c, err := q.SQL()
	if err != nil {
		return nil, err
	}
	return runReturning(ctx, q.q, "insert", q.meta, q.i.returning, c)
}

// plan resolves the query once and splits a bulk model insert into chunks
// that each stay under the dialect's bind-parameter limit. It returns the
// query itself when no split is needed (or when rendering will report the
// error), so validation errors keep their single-statement form.
func (q InsertQuery[T]) plan() ([]InsertQuery[T], error) {
	if err := viewWriteErr("insert", q.meta); err != nil {
		return nil, err
	}
	q, err := q.generateIDs(true)
	if err != nil {
		return nil, err
	}
	q.idsDone = true
	if q.q != nil {
		if err := q.checkExplicit(); err != nil {
			return nil, err
		}
		if err := q.checkExplicitZero(q.q.Dialect()); err != nil { // every row, before the first chunk runs
			return nil, err
		}
	}
	q = q.withDBD() // after hooks and ids: the plan is computed once and sliced per chunk
	runs, err := q.defaultRuns()
	if err != nil {
		return nil, err
	}
	if runs == nil {
		return q.planBulk()
	}
	var out []InsertQuery[T]
	for _, r := range runs {
		cs, err := r.planBulk()
		if err != nil {
			return nil, err
		}
		out = append(out, cs...)
	}
	return out, nil
}

// planBulk splits a bulk model insert over the bind-parameter limit; see plan.
func (q InsertQuery[T]) planBulk() ([]InsertQuery[T], error) {
	one := []InsertQuery[T]{q}
	if q.i.unnest {
		return one, nil // one array parameter per column: the bind count does not grow with the rows
	}
	if q.q == nil || q.i.err != nil || q.meta.Table == "" || len(q.models) < 2 {
		return one, nil
	}
	d := q.q.Dialect()
	if d.MaxParams <= 0 {
		return one, nil
	}
	r, err := q.resolveUpsert()
	if err != nil {
		return nil, err
	}
	r.i.upsert = false // resolved: chunks keep target, set and identity decisions
	fp, err := newFieldPlan(q.q, q.meta)
	if err != nil {
		return one, nil // rendering reports it
	}
	cols, err := r.modelCols(fp)
	if err != nil || len(cols) == 0 {
		return one, nil
	}
	r.i.fixedCols = cols  // every chunk writes the columns of the whole set
	if r.clock.IsZero() { // an updated_at bump in resolveUpsert already read it
		r.clock = q.q.conn().now()
	}
	// Compute and validate every row before the first chunk runs, so a bad
	// row late in a bulk insert executes nothing. The computed values are
	// kept and bound by the chunks, so a non-deterministic default cannot
	// pass here and change afterwards.
	if len(fp.hooks) > 0 {
		r.pre = r.modelValues(fp, cols, r.clock)
		if err := r.validateModelRows(fp, cols, r.pre); err != nil {
			return nil, err
		}
		one[0].pre = r.pre // a fallback to the single statement binds the same values
		one[0].clock = r.clock
	}
	probe := r
	probe.models = r.models[:1]
	probe.dbd = r.dbd.slice(0, 1)
	if r.pre != nil {
		probe.pre = r.pre[:1]
	}
	c1, err := compile(d, probe.render)
	if err != nil {
		return one, nil
	}
	bound := 0 // cells the probe row binds: a dbdefault cell writes DEFAULT, no parameter
	for _, ci := range cols {
		if !probe.dbDefaultRow(0, ci) {
			bound++
		}
	}
	fixed := len(c1.Args) - bound // parameters outside the VALUES rows
	per := (d.MaxParams - fixed) / len(cols)
	if per < 1 {
		return nil, &Error{Kind: ErrLimitExceeded, Op: "insert", Table: q.meta.Table,
			Err: fmt.Errorf("one row binds %d parameters, over the %s limit of %d", len(cols)+fixed, d.Name, d.MaxParams)}
	}
	if len(r.models) <= per {
		return []InsertQuery[T]{r}, nil
	}
	if len(r.i.with) > 0 {
		// A WITH clause would run once per chunk, repeating a data-modifying CTE.
		return nil, r.verr("", "With: the insert needs %d statements at the %s parameter limit and a WITH clause would repeat in each; insert fewer rows", (len(r.models)+per-1)/per, d.Name)
	}
	var out []InsertQuery[T]
	for lo := 0; lo < len(r.models); lo += per {
		c := r
		hi := min(lo+per, len(r.models))
		c.models = r.models[lo:hi]
		c.dbd = r.dbd.slice(lo, hi)
		if len(r.back) == len(r.models) {
			c.back = r.back[lo:hi]
		}
		if r.pre != nil {
			c.pre = r.pre[lo:hi]
		}
		out = append(out, c)
	}
	return out, nil
}

// defaultRuns splits a multi-row insert on a dialect whose VALUES list cannot
// hold DEFAULT (SQLite) into runs of consecutive rows that leave the same
// columns to the database default. Within a run every column is either default
// in every row, so it leaves the column list and the column default applies, or
// given in every row, so each run is an ordinary statement. Runs are
// consecutive, never regrouped, so rows are stored and returned in the order
// the caller gave them. It returns nil when no split is needed: the dialect
// takes DEFAULT, Unnest, a single row or a uniform statement.
func (q InsertQuery[T]) defaultRuns() ([]InsertQuery[T], error) {
	if q.q == nil || q.i.unnest || q.i.err != nil || q.meta.Table == "" || q.q.Dialect().DefaultInValues {
		return nil, nil
	}
	var sigs [][]bool
	if q.i.hasRows {
		sigs = q.rowsDefaultSigs()
	} else if len(q.models) > 1 {
		sigs = q.dbd.rows
	}
	if len(sigs) < 2 {
		return nil, nil
	}
	var bounds []int // the start of each run
	for r := range sigs {
		if r == 0 || !slices.Equal(sigs[r], sigs[r-1]) {
			bounds = append(bounds, r)
		}
	}
	if len(bounds) < 2 {
		return nil, nil
	}
	if len(q.i.with) > 0 {
		return nil, q.verr("", "With: the rows leave different columns to the database default, so the insert needs %d statements on %s and a WITH clause would repeat in each; give every row the same columns", len(bounds), q.q.Dialect().Name)
	}
	if q.clock.IsZero() {
		q.clock = q.q.conn().now() // one statement time for every run
	}
	out := make([]InsertQuery[T], 0, len(bounds))
	for i, lo := range bounds {
		hi := len(sigs)
		if i+1 < len(bounds) {
			hi = bounds[i+1]
		}
		c := q
		if q.i.hasRows {
			c.i.rows = q.i.rows[lo:hi:hi]
		} else {
			c.models = q.models[lo:hi:hi]
			c.dbd = q.dbd.slice(lo, hi)
			if len(q.back) == len(q.models) {
				c.back = q.back[lo:hi:hi]
			}
			if q.pre != nil {
				c.pre = q.pre[lo:hi:hi]
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// rowsDefaultSigs returns, per ValuesRows row, which columns of the statement
// the row leaves to the database default: a column some row assigns that this
// row omits or sets with SetDefault, unless a Go-side default or an automatic
// timestamp fills it. It returns nil when the rows cannot be resolved; render
// reports that.
func (q InsertQuery[T]) rowsDefaultSigs() [][]bool {
	if len(q.i.rows) < 2 {
		return nil
	}
	fp, err := newFieldPlan(q.q, q.meta)
	if err != nil {
		return nil
	}
	n := len(q.meta.Columns)
	kinds := make([]map[int]setKind, len(q.i.rows))
	union := make([]bool, n)
	for r, row := range q.i.rows {
		kinds[r] = map[int]setKind{}
		for _, a := range row {
			ci := q.column(a.col)
			if ci < 0 {
				return nil
			}
			kinds[r][ci] = a.kind
			union[ci] = true
		}
	}
	sigs := make([][]bool, len(q.i.rows))
	for r := range q.i.rows {
		seen := make(map[int]struct{}, len(kinds[r]))
		for ci := range kinds[r] {
			seen[ci] = struct{}{}
		}
		stamped, _, err := q.timestampAssigns(seen, time.Time{}) // presence only
		if err != nil {
			return nil
		}
		sig := make([]bool, n)
		for ci := range n {
			kind, assigned := kinds[r][ci]
			switch {
			case assigned:
				sig[ci] = kind == setDefault
			case !union[ci]:
			case fp.def[ci] != nil && q.writable(ci):
			case slices.Contains(stamped, ci):
			default:
				sig[ci] = true
			}
		}
		sigs[r] = sig
	}
	return sigs
}

// chunkRows is the Rows of a chunked insert with RETURNING: it queries the
// chunks one after another and finishes the atomic scope at the end.
type chunkRows[T any] struct {
	ctx    context.Context
	chunks []InsertQuery[T]
	sc     atomicScope
	cur    Rows
	next   int
	err    error
	done   bool
}

func (r *chunkRows[T]) finish(err error) {
	if r.done {
		return
	}
	r.done = true
	if err = r.sc.finish(r.ctx, err); err != nil && r.err == nil {
		r.err = err
	}
}

// open queries the next chunk and makes it the current cursor.
func (r *chunkRows[T]) open() error {
	c := r.chunks[r.next]
	r.next++
	cmp, err := c.SQL()
	if err == nil {
		r.cur, err = c.q.conn().query(r.ctx, "insert", c.meta.Table, "", cmp)
	}
	return err
}

func (r *chunkRows[T]) Next() bool {
	for !r.done {
		if r.cur == nil {
			if r.next >= len(r.chunks) {
				r.finish(nil)
				return false
			}
			if err := r.open(); err != nil {
				r.err = err
				r.finish(err)
				return false
			}
		}
		if r.cur.Next() {
			return true
		}
		err := r.cur.Err()
		_ = r.cur.Close()
		r.cur = nil
		if err != nil {
			r.err = err
			r.finish(err)
			return false
		}
	}
	return false
}

func (r *chunkRows[T]) Scan(dest ...any) error {
	if r.cur == nil {
		return newError(ErrValidation, "scan", "", errors.New("Scan without a current row"))
	}
	return r.cur.Scan(dest...)
}

func (r *chunkRows[T]) Err() error { return r.err }

// Close runs the chunks that were not read yet, so the insert is whole, and
// ends the atomic scope.
func (r *chunkRows[T]) Close() error {
	if r.done {
		return r.err
	}
	if r.cur != nil {
		if err := r.cur.Close(); err != nil && r.err == nil {
			r.err = err
		}
		r.cur = nil
	}
	for r.err == nil && r.next < len(r.chunks) {
		c := r.chunks[r.next]
		r.next++
		_, r.err = c.execOne(r.ctx)
	}
	r.finish(r.err)
	return r.err
}
