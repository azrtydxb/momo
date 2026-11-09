// SPDX-License-Identifier: Apache-2.0

// Package advisory takes PostgreSQL transaction-scoped advisory locks through
// a momo transaction. A writer uses one to guard a logical resource that has
// no row to lock: two transactions that pass the same key serialize on it.
//
// Every function takes the transaction as its second argument. The lock is
// released automatically at commit or rollback; there is no unlock. A call
// with a *momo.DB instead of a *momo.Tx fails with momo.ErrValidation, because
// the server would release a lock taken outside a transaction at the end of
// that single statement. Other dialects than PostgreSQL fail with
// momo.ErrUnsupported.
package advisory

import (
	"context"
	"errors"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// XactLock takes the transaction-scoped advisory lock for key
// (pg_advisory_xact_lock) and waits until it is granted. The lock is released
// at commit or rollback.
//
// q must be a *momo.Tx; a *momo.DB fails with momo.ErrValidation. Other
// dialects than PostgreSQL fail with momo.ErrUnsupported.
func XactLock(ctx context.Context, q momo.Querier, key int64) error {
	return lock(ctx, q, "SELECT pg_advisory_xact_lock(?)", key)
}

// XactLockString is XactLock with a text key, hashed by the server with
// hashtext into a 32-bit key. Distinct strings can collide, which only costs
// extra serialization, never a missed lock. String and int64 keys share one
// bigint key space, so a string collides with the int64 key equal to its
// hashtext value. hashtext is not guaranteed to give the same result across
// PostgreSQL major versions: never store a hashed key, and use the int64 form
// when the key space is large enough for collisions to matter.
func XactLockString(ctx context.Context, q momo.Querier, key string) error {
	return lock(ctx, q, "SELECT pg_advisory_xact_lock(hashtext(?))", key)
}

// TryXactLock is the non-blocking form of XactLock: it reports whether the
// lock was granted instead of waiting for it (pg_try_advisory_xact_lock). A
// true result holds the lock until the transaction ends.
func TryXactLock(ctx context.Context, q momo.Querier, key int64) (bool, error) {
	return try(ctx, q, "SELECT pg_try_advisory_xact_lock(?)", key)
}

// TryXactLockString is TryXactLock with a hashtext text key. See
// XactLockString for the key space.
func TryXactLockString(ctx context.Context, q momo.Querier, key string) (bool, error) {
	return try(ctx, q, "SELECT pg_try_advisory_xact_lock(hashtext(?))", key)
}

func check(q momo.Querier) error {
	if _, ok := q.(*momo.Tx); !ok {
		return &momo.Error{Kind: momo.ErrValidation, Op: "advisory lock", Err: errors.New("a transaction-level advisory lock outside a transaction is released immediately; pass the *momo.Tx from Begin or DB.Tx")}
	}
	if q.Dialect().Name != dialect.Postgres {
		return &momo.Error{Kind: momo.ErrUnsupported, Op: "advisory lock", Err: errors.New("transaction-scoped advisory locks need PostgreSQL")}
	}
	return nil
}

func lock(ctx context.Context, q momo.Querier, sql string, key any) error {
	if err := check(q); err != nil {
		return err
	}
	_, err := momo.Exec(ctx, q, sql, key)
	return err
}

func try(ctx context.Context, q momo.Querier, sql string, key any) (bool, error) {
	if err := check(q); err != nil {
		return false, err
	}
	got, err := momo.Scalars[bool](ctx, q, sql, key)
	if err != nil {
		return false, err
	}
	if len(got) != 1 {
		return false, &momo.Error{Kind: momo.ErrNotFound, Op: "advisory lock", Err: errors.New("pg_try_advisory_xact_lock returned no row")}
	}
	return got[0], nil
}
