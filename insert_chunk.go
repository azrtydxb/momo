// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"slices"
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
	r := q.ref()
	if r.reconcile {
		return q.execReconcile(ctx)
	}
	c, err := q.SQL()
	if err != nil {
		return Result{}, err
	}
	if cols := r.dbDefaultReadBack(); len(cols) > 0 {
		return q.execReadBack(ctx, cols)
	}
	back := q.back
	if r.wantsGeneratedKey(insAccess{hasBack: func(row int) bool { return back[row] != nil }}) {
		n, id, err := q.q.conn().execInsertID(ctx, "insert", q.meta.Table, c)
		if err != nil {
			return Result{}, err
		}
		if n > 0 && id != 0 {
			assignGeneratedKey(q.meta.Ptr(q.back[0], q.meta.Identity), id)
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
		return nil, insertVerr(q.meta.Table, "", "Rows needs a Returning list")
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
	ms, back, meta := q.models, q.back, q.meta
	fp, ferr := newFieldPlan(q.q, meta)
	a := insAccess{
		value:   func(row, ci int) any { return meta.Value(&ms[row], ci) },
		zero:    func(ci int) (any, bool) { return zeroValue(meta, ci) },
		def:     func(ci, row int) any { return fieldDefault(&fp, ms, ci, row) },
		ptr:     func(row, ci int) any { return meta.Ptr(&ms[row], ci) },
		hasBack: func(row int) bool { return row < len(back) && back[row] != nil },
		backPtr: func(row, ci int) any { return meta.Ptr(back[row], ci) },
		clone:   func() { ms = slices.Clone(ms) },
		fields:  fp.ref,
		ferr:    ferr,
	}
	cs, err := q.ref().plan(a)
	if err != nil {
		return nil, err
	}
	q.models = ms
	if cs.many == nil {
		return []InsertQuery[T]{chunkQuery(q, ms, back, cs.one)}, nil
	}
	out := make([]InsertQuery[T], len(cs.many))
	for i, ch := range cs.many {
		out[i] = chunkQuery(q, ms, back, ch)
	}
	return out, nil
}

// chunkQuery is the statement of one planned chunk: q with the state of the
// chunk and, for a split insert, the models (and the caller's models) it holds.
func chunkQuery[T any](q InsertQuery[T], ms []T, back []*T, ch insertChunk) InsertQuery[T] {
	c := q.withRef(ch.r)
	if ch.sub {
		c.models = ms[ch.lo:ch.hi:ch.hi]
		if ch.back {
			c.back = back[ch.lo:ch.hi:ch.hi]
		}
	}
	return c
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
