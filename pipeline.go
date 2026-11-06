// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
)

// Pipelined relation loading (S-51).
//
// When the connection behind a Querier implements Batcher (the pgx driver, on
// the pool, on a transaction and on a pinned connection), the child statements
// of all relations loaded together by LoadAll, including every bind-limit chunk
// of one relation, are sent in one round trip instead of one round trip each.
// Everything else keeps the sequential path: drivers without a Batcher (the
// database/sql adapters, SQLite, MySQL), a single statement, and loaders that
// are not batchable. The result is identical to sequential loading; only the
// number of round trips changes.
//
// Order of events for a batched level, which is also the order the sequential
// path would use:
//
//  1. BeforeRelation for each relation, in loader order.
//  2. BeforeQuery for each statement, in loader then chunk order, then one
//     driver round trip. The batch runs under the context of the first
//     statement's hooks, so a cancelled context stops the whole batch.
//  3. Result sets are read in statement order (AfterQuery fires per statement
//     as its rows close).
//  4. Per relation, in loader order: nested loads and assignment, then
//     AfterRelation. A nested level is pipelined the same way.
//
// A batch is all or nothing. When one statement fails the driver cancels the
// rest, every statement's AfterQuery reports the error and so does every
// relation's AfterRelation; the parents keep the empty associations that
// preparing a load leaves. A driver that rejects the batch as unsupported
// (ErrUnsupported with Op "batch", before sending anything, for example a pgx
// batch under a non-default exec mode) makes the level fall back to sequential
// loading; the rejected attempt is visible to hooks as failed statements.

// batchable is a pendingLoad whose statements can be pipelined.
type batchable interface {
	pendingLoad
	// statements returns the compiled child statements, in execution order.
	statements() []Compiled
	// target returns the hook names of the load: relation "Parent.Name" and
	// the child table.
	target() (rel, table string)
	// begin reports the start of the load to relation hooks.
	begin(ctx context.Context, q Querier) (context.Context, func(error))
	// consumeRows reads the result sets of statements(), in order.
	consumeRows(rows []Rows) error
	// runQueries runs the statements one at a time (the sequential fallback).
	runQueries(ctx context.Context, q Querier) error
	// finish runs nested loads and assigns the children.
	finish(ctx context.Context) error
}

func (pd *pending[P, C]) statements() []Compiled { return pd.queries }

func (pd *pending[P, C]) target() (string, string) {
	return pd.r.Parent.Name + "." + pd.r.Name, pd.r.Child.Name
}

// consumeRows reads this relation's result sets in statement order. Nested
// loads and assignment happen later, in finish.
func (pd *pending[P, C]) consumeRows(rows []Rows) error {
	for _, rs := range rows {
		if err := pd.consume(rs); err != nil {
			return err
		}
	}
	return nil
}

// runPending runs prepared loads: pipelined when every load is batchable, the
// connection implements Batcher and there is more than one statement in all,
// sequentially otherwise.
func runPending(ctx context.Context, q Querier, pend []pendingLoad) error {
	if bs, b, ok := pipelinePlan(q, pend); ok {
		return runBatched(ctx, q, b, bs)
	}
	for _, p := range pend {
		if err := p.run(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

// pipelinePlan reports whether pend can be sent as one batch on q.
func pipelinePlan(q Querier, pend []pendingLoad) ([]batchable, Batcher, bool) {
	b, ok := q.conn().q.(Batcher)
	if !ok {
		return nil, nil, false
	}
	bs := make([]batchable, len(pend))
	n := 0
	for i, p := range pend {
		bp, ok := p.(batchable)
		if !ok {
			return nil, nil, false
		}
		bs[i] = bp
		n += len(bp.statements())
	}
	return bs, b, n > 1
}

// runBatched sends the statements of every load in one round trip, reads every
// result set in order, and only then runs nested loads and assignments (a
// nested level may send its own batch). See the file comment for the event
// order and failure semantics.
func runBatched(ctx context.Context, q Querier, b Batcher, pend []batchable) error {
	ctxs := make([]context.Context, len(pend))
	ends := make([]func(error), len(pend))
	for i, p := range pend {
		ctxs[i], ends[i] = p.begin(ctx, q)
	}
	// abort ends the relation events from index i on with err.
	abort := func(from int, err error) error {
		for j := from; j < len(pend); j++ {
			ends[j](err)
		}
		return err
	}
	if err := ctx.Err(); err != nil {
		return abort(0, err)
	}

	var stmts []stmt
	counts := make([]int, len(pend))
	for i, p := range pend {
		rel, table := p.target()
		cs := p.statements()
		counts[i] = len(cs)
		for _, c := range cs {
			stmts = append(stmts, stmt{ctx: ctxs[i], rel: rel, table: table, c: c})
		}
	}
	rows, err := q.conn().batch(b, stmts)
	if err != nil {
		var me *Error
		if errors.As(err, &me) && me.Kind == ErrUnsupported && me.Op == "batch" {
			return runUnbatched(q, pend, ctxs, ends)
		}
		return abort(0, err)
	}

	off := 0
	for i, p := range pend {
		n := counts[i]
		if err := p.consumeRows(rows[off : off+n]); err != nil {
			closeRows(rows[off:])
			return abort(i, err)
		}
		off += n
	}
	for i, p := range pend {
		err := p.finish(ctxs[i])
		ends[i](err)
		if err != nil {
			return abort(i+1, err)
		}
	}
	return nil
}

// runUnbatched is the fallback for a rejected batch: the relation events are
// already open, so each load runs its statements one at a time inside them.
func runUnbatched(q Querier, pend []batchable, ctxs []context.Context, ends []func(error)) error {
	for i, p := range pend {
		err := p.runQueries(ctxs[i], q)
		if err == nil {
			err = p.finish(ctxs[i])
		}
		ends[i](err)
		if err != nil {
			for j := i + 1; j < len(pend); j++ {
				ends[j](err)
			}
			return err
		}
	}
	return nil
}

// stmt is one compiled statement of a batch, tagged with the hook context and
// names of the relation it belongs to.
type stmt struct {
	ctx        context.Context
	rel, table string
	c          Compiled
}

// batch sends every statement in one round trip and returns one Rows per
// statement, in order. Each statement gets its own hook pair, like query. The
// round trip runs under the first statement's context.
func (e *execer) batch(b Batcher, stmts []stmt) ([]Rows, error) {
	cs := make([]Compiled, len(stmts))
	ctxs := make([]context.Context, len(stmts))
	evs := make([]*QueryEvent, len(stmts))
	for i, s := range stmts {
		cs[i] = s.c
		ctxs[i], evs[i] = e.before(s.ctx, "select", s.table, s.rel, s.c.SQL, s.c.Args)
	}
	rows, err := b.SendBatch(ctxs[0], cs)
	if err == nil && len(rows) != len(stmts) {
		closeRows(rows)
		err = errors.New("momo: driver returned the wrong number of batch results")
	}
	if err != nil {
		err = e.wrapErr("select", "", err)
		for i := range stmts {
			e.after(ctxs[i], evs[i], -1, err)
		}
		return nil, err
	}
	out := make([]Rows, len(rows))
	for i := range rows {
		if evs[i] == nil && e.classify == nil {
			out[i] = rows[i]
			continue
		}
		out[i] = &hookedRows{Rows: rows[i], e: e, ctx: ctxs[i], ev: evs[i], op: "select", table: stmts[i].table}
	}
	return out, nil
}

// closeRows closes every result set; Close is safe to call more than once.
func closeRows(rs []Rows) {
	for _, r := range rs {
		_ = r.Close()
	}
}
