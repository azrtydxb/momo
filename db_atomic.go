// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"strconv"
)

// atomicScope is an atomic scope opened on a Querier: a transaction on a DB, a
// savepoint on a Tx, or nothing when the caller's transaction is already
// atomic as a whole. q is the Querier to run the scope's statements on and
// finish ends the scope: nil commits or releases it, an error rolls it back
// and is returned (joined with any rollback failure).
type atomicScope struct {
	q      Querier
	finish func(ctx context.Context, err error) error
}

// beginAtomic opens a transaction on a DB or a savepoint on a Tx, and returns
// the Querier the statements run on. On a Tx without savepoints it opens
// nothing: the caller's transaction is atomic as a whole. A pinned connection
// cannot begin a transaction, so it fails with ErrUnsupported. op and table
// name the multi-statement operation (a chunked insert) in that error.
func beginAtomic(ctx context.Context, q Querier, op, table string) (atomicScope, error) {
	noop := atomicScope{q: q, finish: func(_ context.Context, err error) error { return err }}
	switch h := q.(type) {
	case *DB:
		if h.pinned {
			return atomicScope{}, &Error{Kind: ErrUnsupported, Op: op, Table: table,
				Err: fmt.Errorf("chunked %s needs a transaction; wrap the call in db.Tx or call on a Tx", op)}
		}
		tx, err := h.Begin(ctx, TxOptions{})
		if err != nil {
			return atomicScope{}, err
		}
		return atomicScope{q: tx, finish: func(ctx context.Context, err error) error {
			if err != nil {
				if rb := tx.Rollback(context.WithoutCancel(ctx)); rb != nil && !errors.Is(rb, ErrTxDone) {
					return errors.Join(err, rb)
				}
				return err
			}
			return tx.Commit(ctx)
		}}, nil
	case *Tx:
		if !h.ex.d.Savepoints {
			return noop, nil
		}
		name := "momo_chunk_" + strconv.Itoa(h.depth+1)
		if err := h.Savepoint(ctx, name); err != nil {
			return atomicScope{}, err
		}
		return atomicScope{q: h, finish: func(ctx context.Context, err error) error {
			if err != nil {
				ctx = context.WithoutCancel(ctx)
				if rb := h.RollbackTo(ctx, name); rb != nil {
					return errors.Join(err, rb)
				}
			}
			if rel := h.Release(ctx, name); rel != nil && err == nil {
				return rel
			}
			return err
		}}, nil
	}
	return noop, nil
}

// runAtomic runs fn in an atomic scope on q (see beginAtomic) and finishes it:
// a nil result commits, an error rolls back. A panic in fn rolls the scope
// back and is re-raised.
func runAtomic(ctx context.Context, q Querier, op, table string, fn func(Querier) error) error {
	sc, err := beginAtomic(ctx, q, op, table)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = sc.finish(context.WithoutCancel(ctx), newError(ErrValidation, op, table, fmt.Errorf("panic during chunked %s: %v", op, p)))
			panic(p)
		}
	}()
	return sc.finish(ctx, fn(sc.q))
}
