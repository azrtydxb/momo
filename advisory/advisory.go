// SPDX-License-Identifier: Apache-2.0

// Package advisory takes PostgreSQL advisory locks through momo. A writer uses
// one to guard a logical resource that has no row to lock: two sessions that
// pass the same key serialize on it.
//
// The transaction-scoped functions (XactLock, TryXactLock and their String
// forms) take a *momo.Tx. The lock is released automatically at commit or
// rollback; there is no unlock. The parameter is a *momo.Tx, not a
// momo.Querier, so passing a *momo.DB does not compile: the server would
// release a lock taken outside a transaction at the end of that single
// statement. A nil *momo.Tx fails with momo.ErrValidation.
//
// The session-scoped functions (Lock, TryLock, Unlock and their String forms)
// take a *momo.DB that came from DB.OnConn, a handle whose statements all run
// on one pinned connection. The lock lasts until Unlock or until that
// connection closes, so the caller must unlock before it releases the pinned
// connection to the pool. A *momo.DB over a pool, where two statements can
// reach two connections, fails with momo.ErrValidation, as does a nil one.
//
// The String forms hash the key on the server. By default they use the 32-bit
// hashtext; pass Hash64 to use the 64-bit hashtextextended(key, seed), the form
// applications that already call it need to keep their keys.
//
// Other dialects than PostgreSQL fail with momo.ErrUnsupported.
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
// A nil tx fails with momo.ErrValidation. Other dialects than PostgreSQL fail
// with momo.ErrUnsupported.
func XactLock(ctx context.Context, tx *momo.Tx, key int64) error {
	if err := check(tx); err != nil {
		return err
	}
	return lock(ctx, tx, "pg_advisory_xact_lock", key, nil)
}

// Option changes how the String functions hash their text key.
type Option func(*hashing)

type hashing struct {
	wide bool
	seed int64
}

// Hash64 makes a String function hash its key with the 64-bit
// hashtextextended(key, seed) instead of the 32-bit hashtext. Use seed 0 to
// match code that calls pg_advisory_xact_lock(hashtextextended(key, 0)). The
// hash is stable for a given PostgreSQL major version only; see
// XactLockString.
func Hash64(seed int64) Option {
	return func(h *hashing) { h.wide, h.seed = true, seed }
}

// keyExpr returns the SQL for the key argument of a lock function and its
// bound arguments: a plain key, or the hashed form of a text key.
func keyExpr(key any, opts []Option) (string, []any) {
	k, isString := key.(string)
	if !isString {
		return "?", []any{key}
	}
	var h hashing
	for _, o := range opts {
		if o != nil {
			o(&h)
		}
	}
	if h.wide {
		return "hashtextextended(?, ?)", []any{k, h.seed}
	}
	return "hashtext(?)", []any{k}
}

// XactLockString is XactLock with a text key, hashed by the server with
// hashtext into a 32-bit key (or with hashtextextended into a 64-bit key when
// Hash64 is passed). Distinct strings can collide, which only costs
// extra serialization, never a missed lock. String and int64 keys share one
// bigint key space, so a string collides with the int64 key equal to its
// hashtext value. hashtext is not guaranteed to give the same result across
// PostgreSQL major versions: never store a hashed key, and use the int64 form
// when the key space is large enough for collisions to matter.
func XactLockString(ctx context.Context, tx *momo.Tx, key string, opts ...Option) error {
	if err := check(tx); err != nil {
		return err
	}
	return lock(ctx, tx, "pg_advisory_xact_lock", key, opts)
}

// TryXactLock is the non-blocking form of XactLock: it reports whether the
// lock was granted instead of waiting for it (pg_try_advisory_xact_lock). A
// true result holds the lock until the transaction ends.
func TryXactLock(ctx context.Context, tx *momo.Tx, key int64) (bool, error) {
	if err := check(tx); err != nil {
		return false, err
	}
	return try(ctx, tx, "pg_try_advisory_xact_lock", key, nil)
}

// TryXactLockString is TryXactLock with a hashtext text key. See
// XactLockString for the key space.
func TryXactLockString(ctx context.Context, tx *momo.Tx, key string, opts ...Option) (bool, error) {
	if err := check(tx); err != nil {
		return false, err
	}
	return try(ctx, tx, "pg_try_advisory_xact_lock", key, opts)
}

func check(tx *momo.Tx) error {
	if tx == nil {
		return &momo.Error{Kind: momo.ErrValidation, Op: "advisory lock", Err: errors.New("a transaction-level advisory lock needs a transaction; pass the *momo.Tx from Begin or DB.Tx")}
	}
	if tx.Dialect().Name != dialect.Postgres {
		return &momo.Error{Kind: momo.ErrUnsupported, Op: "advisory lock", Err: errors.New("transaction-scoped advisory locks need PostgreSQL")}
	}
	return nil
}

func lock(ctx context.Context, q momo.Querier, fn string, key any, opts []Option) error {
	k, args := keyExpr(key, opts)
	_, err := momo.Exec(ctx, q, "SELECT "+fn+"("+k+")", args...)
	return err
}

func try(ctx context.Context, q momo.Querier, fn string, key any, opts []Option) (bool, error) {
	k, args := keyExpr(key, opts)
	got, err := momo.Scalars[bool](ctx, q, "SELECT "+fn+"("+k+")", args...)
	if err != nil {
		return false, err
	}
	if len(got) != 1 {
		return false, &momo.Error{Kind: momo.ErrNotFound, Op: "advisory lock", Err: errors.New(fn + " returned no row")}
	}
	return got[0], nil
}

func checkSession(db *momo.DB) error {
	if db == nil {
		return &momo.Error{Kind: momo.ErrValidation, Op: "advisory lock", Err: errors.New("a session-level advisory lock needs a pinned connection; pass db.OnConn(pc)")}
	}
	if db.Dialect().Name != dialect.Postgres {
		return &momo.Error{Kind: momo.ErrUnsupported, Op: "advisory lock", Err: errors.New("session-scoped advisory locks need PostgreSQL")}
	}
	if !db.IsPinned() {
		return &momo.Error{Kind: momo.ErrValidation, Op: "advisory lock", Err: errors.New("a session-level advisory lock needs a pinned connection: a pooled *momo.DB may run the lock and the unlock on different connections; pass db.OnConn(pc)")}
	}
	return nil
}

// Lock takes the session-scoped advisory lock for key (pg_advisory_lock) on
// the pinned connection behind db and waits until it is granted. The lock is
// held until Unlock or until the connection closes; it is not released by a
// commit. db must come from DB.OnConn.
func Lock(ctx context.Context, db *momo.DB, key int64) error {
	if err := checkSession(db); err != nil {
		return err
	}
	return lock(ctx, db, "pg_advisory_lock", key, nil)
}

// LockString is Lock with a text key, hashed on the server; see
// XactLockString for the hash and Hash64 for the 64-bit form.
func LockString(ctx context.Context, db *momo.DB, key string, opts ...Option) error {
	if err := checkSession(db); err != nil {
		return err
	}
	return lock(ctx, db, "pg_advisory_lock", key, opts)
}

// TryLock is the non-blocking form of Lock (pg_try_advisory_lock): it reports
// whether the lock was granted. A true result holds the lock until Unlock or
// until the connection closes.
func TryLock(ctx context.Context, db *momo.DB, key int64) (bool, error) {
	if err := checkSession(db); err != nil {
		return false, err
	}
	return try(ctx, db, "pg_try_advisory_lock", key, nil)
}

// TryLockString is TryLock with a text key, hashed on the server.
func TryLockString(ctx context.Context, db *momo.DB, key string, opts ...Option) (bool, error) {
	if err := checkSession(db); err != nil {
		return false, err
	}
	return try(ctx, db, "pg_try_advisory_lock", key, opts)
}

// Unlock releases one hold of the session-scoped lock for key
// (pg_advisory_unlock) and reports whether the session held it. Locks are
// counted: a key locked twice needs two unlocks.
func Unlock(ctx context.Context, db *momo.DB, key int64) (bool, error) {
	if err := checkSession(db); err != nil {
		return false, err
	}
	return try(ctx, db, "pg_advisory_unlock", key, nil)
}

// UnlockString is Unlock for a text key. Pass the same Hash64 option the lock
// used, or the unlock targets a different key.
func UnlockString(ctx context.Context, db *momo.DB, key string, opts ...Option) (bool, error) {
	if err := checkSession(db); err != nil {
		return false, err
	}
	return try(ctx, db, "pg_advisory_unlock", key, opts)
}
