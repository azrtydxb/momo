// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"

	"github.com/azrtydxb/momo/dialect"
)

// AdvisoryXactLock takes the transaction-scoped advisory lock for key
// (pg_advisory_xact_lock) and waits until it is granted. The lock is released
// automatically at commit or rollback; there is no unlock. Two transactions
// that pass the same key serialize on it, which is how a writer guards a
// logical resource that has no row to lock.
//
// PostgreSQL only; other dialects return ErrUnsupported.
func (tx *Tx) AdvisoryXactLock(ctx context.Context, key int64) error {
	return advisoryLock(ctx, tx, "SELECT pg_advisory_xact_lock(?)", key)
}

// AdvisoryXactLockString is AdvisoryXactLock with a text key, hashed by the
// server with hashtext into a 32-bit key. Distinct strings can collide, which
// only costs extra serialization, never a missed lock. String and int64 keys
// share one bigint key space, so a string collides with the int64 key equal
// to its hashtext value. hashtext is not guaranteed to give the same result
// across PostgreSQL major versions: never store a hashed key, and use the
// int64 form when the key space is large enough for collisions to matter.
func (tx *Tx) AdvisoryXactLockString(ctx context.Context, key string) error {
	return advisoryLock(ctx, tx, "SELECT pg_advisory_xact_lock(hashtext(?))", key)
}

// TryAdvisoryXactLock is the non-blocking form: it reports whether the lock
// was granted instead of waiting for it (pg_try_advisory_xact_lock). A true
// result holds the lock until the transaction ends.
func (tx *Tx) TryAdvisoryXactLock(ctx context.Context, key int64) (bool, error) {
	return advisoryTry(ctx, tx, "SELECT pg_try_advisory_xact_lock(?)", key)
}

// TryAdvisoryXactLockString is TryAdvisoryXactLock with a hashtext text key.
func (tx *Tx) TryAdvisoryXactLockString(ctx context.Context, key string) (bool, error) {
	return advisoryTry(ctx, tx, "SELECT pg_try_advisory_xact_lock(hashtext(?))", key)
}

// The same methods exist on DB so that a call outside a transaction fails
// with a clear ErrValidation instead of silently taking a lock that the
// server releases at the end of the single autocommit statement.

// AdvisoryXactLock is refused: a transaction-level lock outside a
// transaction would be released at once. Call it on a *Tx.
func (db *DB) AdvisoryXactLock(context.Context, int64) error { return errAdvisoryOutsideTx() }

// AdvisoryXactLockString is refused outside a transaction. See AdvisoryXactLock.
func (db *DB) AdvisoryXactLockString(context.Context, string) error { return errAdvisoryOutsideTx() }

// TryAdvisoryXactLock is refused outside a transaction. See AdvisoryXactLock.
func (db *DB) TryAdvisoryXactLock(context.Context, int64) (bool, error) {
	return false, errAdvisoryOutsideTx()
}

// TryAdvisoryXactLockString is refused outside a transaction. See AdvisoryXactLock.
func (db *DB) TryAdvisoryXactLockString(context.Context, string) (bool, error) {
	return false, errAdvisoryOutsideTx()
}

func errAdvisoryOutsideTx() error {
	return &Error{Kind: ErrValidation, Op: "advisory lock", Err: errors.New("a transaction-level advisory lock outside a transaction is released immediately; call it on a *Tx from Begin or DB.Tx")}
}

func advisoryCheck(tx *Tx) error {
	if tx.ex.d.Name != dialect.Postgres {
		return &Error{Kind: ErrUnsupported, Op: "advisory lock", Err: errors.New("transaction-scoped advisory locks need PostgreSQL")}
	}
	return nil
}

func advisoryLock(ctx context.Context, tx *Tx, sql string, key any) error {
	if err := advisoryCheck(tx); err != nil {
		return err
	}
	_, err := Exec(ctx, tx, sql, key)
	return err
}

func advisoryTry(ctx context.Context, tx *Tx, sql string, key any) (bool, error) {
	if err := advisoryCheck(tx); err != nil {
		return false, err
	}
	got, err := Scalars[bool](ctx, tx, sql, key)
	if err != nil {
		return false, err
	}
	if len(got) != 1 {
		return false, &Error{Kind: ErrUnsupported, Op: "advisory lock", Err: errors.New("pg_try_advisory_xact_lock returned no row")}
	}
	return got[0], nil
}
