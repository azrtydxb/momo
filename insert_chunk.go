// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
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
	return &ModelRows[T]{rows: cr, meta: q.meta, idx: idx}, nil
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
	q, err := q.generateIDs(true)
	if err != nil {
		return nil, err
	}
	q.idsDone = true
	one := []InsertQuery[T]{q}
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
	r.clock = q.q.conn().now()
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
	if r.pre != nil {
		probe.pre = r.pre[:1]
	}
	c1, err := compile(d, probe.render)
	if err != nil {
		return one, nil
	}
	fixed := len(c1.Args) - len(cols) // parameters outside the VALUES rows
	per := (d.MaxParams - fixed) / len(cols)
	if per < 1 {
		return nil, &Error{Kind: ErrLimitExceeded, Op: "insert", Table: q.meta.Table,
			Err: fmt.Errorf("one row binds %d parameters, over the %s limit of %d", len(cols)+fixed, d.Name, d.MaxParams)}
	}
	if len(r.models) <= per {
		return []InsertQuery[T]{r}, nil
	}
	var out []InsertQuery[T]
	for lo := 0; lo < len(r.models); lo += per {
		c := r
		hi := min(lo+per, len(r.models))
		c.models = r.models[lo:hi]
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
