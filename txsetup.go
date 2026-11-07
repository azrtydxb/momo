// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// WithTxSetup registers fn to run right after every transaction momo begins,
// before any other statement of that transaction. It covers DB.Begin and
// DB.Tx, and every transaction momo opens on its own: chunked inserts, graph
// creation, relation writes, WithSearch and the short transactions of
// WithImplicitTx. Typical use is row-level-security scoping:
//
//	db := momo.New(drv, momo.WithTxSetup(func(ctx context.Context, tx *momo.Tx) error {
//		return momo.SetConfigLocal(ctx, tx, "app.tenant_id", tenantFrom(ctx))
//	}))
//
// fn runs once per real transaction. It does not run again for a savepoint
// (Tx.Tx, or the savepoint a chunked insert opens on a Tx), because a
// savepoint stays inside the transaction whose setting is already applied.
// It does not run for AdoptTx either: momo did not begin that transaction, so
// the caller owns its setup.
//
// When fn returns an error the transaction is rolled back and Begin (or the
// call that needed the transaction) returns that error; nothing else runs.
// fn must not commit or roll back tx. Use transaction-local settings only
// (SetConfigLocal, Tx.SetLocal): a session-level SET would outlive the
// transaction and, behind PgBouncer in transaction mode, leak to another
// client. With several options the functions run in registration order.
func WithTxSetup(fn func(ctx context.Context, tx *Tx) error) Option {
	return func(db *DB) {
		if fn != nil {
			db.txSetup = append(db.txSetup, fn)
		}
	}
}

// WithImplicitTx makes every auto-commit statement on a *DB run inside its own
// short transaction, so WithTxSetup applies to it too. This covers Select,
// Insert, Update, Delete, Exec, Scalars and the relation loads. Statements
// on a *Tx are unaffected, and a *DB pinned with OnConn keeps running on its
// connection without a transaction.
//
// A streaming read (Iter, Each, Rows) holds its transaction, and so its
// connection, until the rows are exhausted or closed, then commits. It rolls
// back instead when iteration failed (a scan or driver error, or a cancelled
// context). Close the rows, or range to the end or break out of Iter, as for
// any rows, so the connection returns to the pool.
//
// Each statement is its own transaction: a relation load or a multi-statement
// call is no longer one snapshot unless you wrap it in DB.Tx. Pipelined
// (Batch) relation loading and the COPY fast path are not used on the *DB,
// because they would bypass the setup; bulk inserts use chunked multi-row
// INSERTs instead. Hooks see the BEGIN and COMMIT of each implicit
// transaction, and its statements report InTx.
func WithImplicitTx() Option { return func(db *DB) { db.implicitTx = true } }

// SetConfigLocal sets the configuration parameter key to value for the rest of
// the current transaction only (set_config with is_local = true). It is the
// building block for tenant scoping inside WithTxSetup. Because the setting
// ends with the transaction it is safe behind PgBouncer in transaction mode;
// never use a session-level SET for this. key must not come from untrusted
// input (see Tx.SetLocal). It returns ErrUnsupported on dialects without
// transaction-scoped settings.
func SetConfigLocal(ctx context.Context, tx *Tx, key, value string) error {
	return tx.SetLocal(ctx, key, value)
}

// runTxSetup applies the setup functions to a freshly begun tx and rolls the
// transaction back when one fails.
func (db *DB) runTxSetup(ctx context.Context, tx *Tx) error {
	for _, fn := range db.txSetup {
		if err := fn(ctx, tx); err != nil {
			if rb := tx.Rollback(context.WithoutCancel(ctx)); rb != nil && !errors.Is(rb, ErrTxDone) {
				return errors.Join(err, fmt.Errorf("momo: rollback after tx setup failed: %w", rb))
			}
			return err
		}
	}
	return nil
}

// implicit reports that a statement on e must run in its own transaction.
func (e *execer) implicit() bool {
	return e.db != nil && e.db.implicitTx && !e.inTx && !e.db.pinned
}

func (e *execer) implicitExec(ctx context.Context, op, table string, c Compiled) (int64, error) {
	tx, err := e.db.Begin(ctx, TxOptions{})
	if err != nil {
		return -1, err
	}
	n, err := tx.ex.exec(ctx, op, table, c)
	if err != nil {
		if rb := tx.Rollback(context.WithoutCancel(ctx)); rb != nil && !errors.Is(rb, ErrTxDone) {
			err = errors.Join(err, rb)
		}
		return -1, err
	}
	if err := tx.Commit(ctx); err != nil {
		return -1, err
	}
	return n, nil
}

func (e *execer) implicitQuery(ctx context.Context, op, table, rel string, c Compiled) (Rows, error) {
	tx, err := e.db.Begin(ctx, TxOptions{})
	if err != nil {
		return nil, err
	}
	rows, err := tx.ex.query(ctx, op, table, rel, c)
	if err != nil {
		if rb := tx.Rollback(context.WithoutCancel(ctx)); rb != nil && !errors.Is(rb, ErrTxDone) {
			err = errors.Join(err, rb)
		}
		return nil, err
	}
	return &txRows{Rows: rows, tx: tx, ctx: ctx}, nil
}

// txRows ends the implicit transaction behind a read: it commits when the
// rows are exhausted or closed cleanly and rolls back when anything failed.
type txRows struct {
	Rows
	tx      *Tx
	ctx     context.Context
	once    sync.Once
	scanErr error
	endErr  error
}

func (r *txRows) Scan(dest ...any) error {
	err := r.Rows.Scan(dest...)
	if err != nil && r.scanErr == nil {
		r.scanErr = err
	}
	return err
}

func (r *txRows) Next() bool {
	if r.Rows.Next() {
		return true
	}
	// Exhausted (or failed): release the rows, then end the transaction.
	_ = r.Close()
	return false
}

func (r *txRows) Err() error {
	if err := r.Rows.Err(); err != nil {
		return err
	}
	return r.endErr
}

func (r *txRows) Close() error {
	var err error
	r.once.Do(func() {
		err = r.Rows.Close()
		failed := err != nil || r.Rows.Err() != nil || r.scanErr != nil || r.ctx.Err() != nil
		if failed {
			if rb := r.tx.Rollback(context.WithoutCancel(r.ctx)); rb != nil && !errors.Is(rb, ErrTxDone) && err == nil {
				err = rb
			}
			return
		}
		if cerr := r.tx.Commit(r.ctx); cerr != nil {
			r.endErr = cerr
			err = cerr
		}
	})
	return err
}

// bypassesTx reports that q is a *DB whose statements must not take a driver
// path (COPY, Batch) that skips the implicit transaction.
func bypassesTx(q Querier) bool {
	db, ok := q.(*DB)
	return ok && db.implicitTx && !db.pinned
}
