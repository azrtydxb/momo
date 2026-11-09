// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"

	"github.com/azrtydxb/momo/dialect"
)

// Tx is an explicit transaction handle. It is not safe for concurrent use:
// a transaction is bound to one connection, which processes one statement
// at a time.
type Tx struct {
	ex    execer
	dtx   DriverTx
	done  *atomic.Bool // shared with savepoint scopes of the same transaction
	depth int          // savepoint nesting depth for Tx.Tx
	// adopted marks a Tx bound to a transaction the caller began (AdoptTx):
	// Commit and Rollback refuse and leave the transaction to its owner.
	adopted bool
}

// Dialect returns the dialect of the transaction's connection.
func (tx *Tx) Dialect() *dialect.Dialect { return tx.ex.d }
func (tx *Tx) conn() *execer             { return &tx.ex }

// DriverTx returns the driver-level transaction behind tx, for driver
// packages that unwrap it (pgxdriver.UnwrapSQLTx, pgxdriver.UnwrapPgxTx). A
// savepoint scope returns the same outer transaction. Calling Commit or
// Rollback on it directly bypasses Momo's bookkeeping; prefer the unwrap
// helpers of the driver package.
func (tx *Tx) DriverTx() DriverTx { return tx.dtx }

// Commit commits the transaction. On a Tx from AdoptTx it returns an error
// matching ErrAdoptedTx and does nothing.
func (tx *Tx) Commit(ctx context.Context) error {
	if tx.depth > 0 {
		return newError(ErrValidation, "commit", "", errors.New("Commit called on a savepoint scope; return from the Tx callback instead"))
	}
	if tx.adopted {
		return &Error{Kind: ErrAdoptedTx, Op: "commit"}
	}
	if !tx.done.CompareAndSwap(false, true) {
		return ErrTxDone
	}
	ctx, ev := tx.ex.before(ctx, "commit", "", "", "COMMIT", nil, nil)
	err := tx.dtx.Commit(ctx)
	tx.ex.after(ctx, ev, -1, err)
	return tx.ex.wrapErr("commit", "", err)
}

// Rollback aborts the transaction. On a Tx from AdoptTx it returns an error
// matching ErrAdoptedTx and does nothing.
func (tx *Tx) Rollback(ctx context.Context) error {
	if tx.depth > 0 {
		return newError(ErrValidation, "rollback", "", errors.New("Rollback called on a savepoint scope; return an error from the Tx callback instead"))
	}
	if tx.adopted {
		return &Error{Kind: ErrAdoptedTx, Op: "rollback"}
	}
	if !tx.done.CompareAndSwap(false, true) {
		return ErrTxDone
	}
	ctx, ev := tx.ex.before(ctx, "rollback", "", "", "ROLLBACK", nil, nil)
	err := tx.dtx.Rollback(ctx)
	tx.ex.after(ctx, ev, -1, err)
	return tx.ex.wrapErr("rollback", "", err)
}

// inTx runs fn in a new transaction when q is a *DB and in a savepoint when q
// is a *Tx. A *DB pinned with OnConn cannot begin a transaction: the call fails
// with ErrUnsupported (Op "begin") before fn runs.
func inTx(ctx context.Context, q Querier, fn func(tx *Tx) error) error {
	switch qq := q.(type) {
	case *DB:
		return qq.Tx(ctx, TxOptions{}, fn)
	case *Tx:
		return qq.Tx(ctx, fn)
	}
	return &Error{Kind: ErrUnsupported, Op: "begin", Err: errors.New("unknown Querier")}
}

// Tx runs fn inside a savepoint of this transaction: an error or panic
// rolls back to the savepoint (and the panic is re-raised); success releases
// it. The outer transaction stays open either way.
func (tx *Tx) Tx(ctx context.Context, fn func(tx *Tx) error) error {
	if !tx.ex.d.Savepoints {
		return &Error{Kind: ErrUnsupported, Op: "savepoint"}
	}
	name := "momo_sp_" + strconv.Itoa(tx.depth+1)
	if err := tx.Savepoint(ctx, name); err != nil {
		return err
	}
	inner := &Tx{dtx: tx.dtx, done: tx.done, depth: tx.depth + 1, adopted: tx.adopted}
	inner.ex = tx.ex
	ok := false
	defer func() {
		if !ok {
			if p := recover(); p != nil {
				_ = tx.RollbackTo(context.WithoutCancel(ctx), name)
				panic(p)
			}
		}
	}()
	err := fn(inner)
	ok = true
	if err != nil {
		if rbErr := tx.RollbackTo(context.WithoutCancel(ctx), name); rbErr != nil {
			return errors.Join(err, fmt.Errorf("momo: rollback to savepoint failed: %w", rbErr))
		}
		return err
	}
	return tx.Release(ctx, name)
}

// Savepoint creates a named savepoint.
func (tx *Tx) Savepoint(ctx context.Context, name string) error {
	return tx.spExec(ctx, "SAVEPOINT ", name)
}

// RollbackTo rolls back to a named savepoint, keeping it defined.
func (tx *Tx) RollbackTo(ctx context.Context, name string) error {
	return tx.spExec(ctx, "ROLLBACK TO SAVEPOINT ", name)
}

// Release releases a named savepoint.
func (tx *Tx) Release(ctx context.Context, name string) error {
	return tx.spExec(ctx, "RELEASE SAVEPOINT ", name)
}

func (tx *Tx) spExec(ctx context.Context, stmt, name string) error {
	if tx.done.Load() {
		return ErrTxDone
	}
	b := NewBuilder(tx.ex.d)
	b.WriteString(stmt)
	b.Ident(name)
	_, err := tx.ex.exec(ctx, "savepoint", "", Compiled{SQL: b.String()})
	return err
}
