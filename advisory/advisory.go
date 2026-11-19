// SPDX-License-Identifier: Apache-2.0

// Package advisory takes PostgreSQL advisory locks through momo. A writer uses
// one to guard a logical resource that has no row to lock: two sessions that
// pass the same key serialize on it.
//
// Two functions do the work, and options choose the lock:
//
//	Acquire(ctx, q, key, opts...)  // takes the lock; Try() makes it non-blocking
//	Release(ctx, q, key, opts...)  // releases a session lock
//
// The key is an int64 (or int), or a string that the server hashes. The options
// are Xact (transaction-scoped), Shared (the reader half of a reader/writer
// pair), Try (report instead of wait) and WithHash64 (a 64-bit hash for a
// string key). Acquire returns whether the lock is held: always true unless Try
// is passed.
//
// The transaction-scoped lock (Xact) takes a *momo.Tx. The lock is released
// automatically at commit or rollback; there is no Release. A *momo.DB with
// Xact fails with momo.ErrValidation, because the server would release a lock
// taken outside a transaction at the end of that single statement, as does a nil
// *momo.Tx.
//
// The session-scoped lock (no Xact) takes a *momo.DB that came from DB.OnConn, a
// handle whose statements all run on one pinned connection. The lock lasts until
// Release or until that connection closes, so the caller must release before it
// returns the pinned connection to the pool. A *momo.DB over a pool, where two
// statements can reach two connections, fails with momo.ErrValidation, as does a
// nil one.
//
// A shared lock (Shared) is the reader half of a reader/writer pair on one key:
// any number of sessions hold the shared lock together, and none runs while
// another session holds the exclusive lock of the same key.
//
// A string key is hashed on the server. By default the hash is the 32-bit
// hashtext; pass WithHash64 to use the 64-bit hashtextextended(key, seed), the
// form applications that already call it need to keep their keys.
//
// MySQL and MariaDB take the session-scoped exclusive lock (Acquire and
// Release without Xact or Shared) with GET_LOCK, GET_LOCK(name, 0) and
// RELEASE_LOCK on a pinned connection, exactly as on PostgreSQL. The key maps to
// a lock name scoped to the current database, as a PostgreSQL advisory lock is
// (GET_LOCK names are otherwise global to the server): "momo:advisory:", 8 hex
// digits of sha256(DATABASE()) ("nodb" without a default database), a colon,
// then the decimal int64 key or "s:" plus 32 hex digits of the sha256 of a text
// key (WithHash64 has no effect there). Xact and Shared fail with
// momo.ErrUnsupported on MySQL and MariaDB: GET_LOCK locks belong to the
// session, are not released by a commit or rollback, and have no shared mode, so
// emulating them would silently hold a session lock.
//
// Other dialects fail with momo.ErrUnsupported.
//
// The 20 functions of the earlier API (Lock, TryLock, Unlock, XactLock,
// LockShared, their Xact, Try and Shared variants and their String forms) are
// deprecated forwarders to Acquire and Release and run the same statements.
package advisory

import (
	"context"
	"errors"
	"reflect"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// Key is the type of a lock key. It accepts int, int32, int64 and string, and
// any named type whose underlying type is one of them; other integer types
// (uint, int16, uint32 and so on) do not compile, so convert them first. An
// integer is bound as an int64. A string is hashed on the server, and a string
// and an int64 share one bigint key space, so a string collides with the int64
// key equal to its hash value.
type Key interface {
	~int | ~int32 | ~int64 | ~string
}

// Option chooses which lock Acquire or Release works on, or how a string key is
// hashed.
type Option func(*settings)

type settings struct {
	wide   bool
	seed   int64
	shared bool
	try    bool
	xact   bool
}

func resolve(opts []Option) settings {
	var s settings
	for _, o := range opts {
		if o != nil {
			o(&s)
		}
	}
	return s
}

// Shared selects the shared lock (pg_advisory_lock_shared and its siblings)
// instead of the exclusive one. Any number of sessions can hold the shared lock
// on one key at once; it conflicts only with the exclusive lock of the same key,
// which waits for every shared holder and which every shared taker waits for.
// Use it for a reader/writer pair on one key: writers take the shared lock and
// run concurrently, a reconciler takes the exclusive one and runs alone. A shared
// lock has no MySQL counterpart. Release with Shared releases a shared hold; the
// exclusive and the shared locks of one key are counted apart.
func Shared() Option { return func(s *settings) { s.shared = true } }

// Try makes Acquire non-blocking: it reports whether the lock was granted
// instead of waiting for it (pg_try_advisory_lock and its siblings, GET_LOCK(name,
// 0) on MySQL). A true result holds the lock until it is released, or until the
// transaction ends with Xact. Release does not accept it.
func Try() Option { return func(s *settings) { s.try = true } }

// Xact makes the lock transaction-scoped (pg_advisory_xact_lock and its
// siblings): q must be the *momo.Tx of Begin or DB.Tx, and the lock is released
// at commit or rollback. Release does not accept it, because there is nothing to
// release. It needs PostgreSQL.
func Xact() Option { return func(s *settings) { s.xact = true } }

// WithHash64 makes a string key hash with the 64-bit hashtextextended(key, seed)
// instead of the 32-bit hashtext. Use seed 0 to match code that calls
// pg_advisory_xact_lock(hashtextextended(key, 0)). The hash is stable for a given
// PostgreSQL major version only: never store a hashed key. It has no effect on an
// integer key or on MySQL.
func WithHash64(seed int64) Option {
	return func(s *settings) { s.wide, s.seed = true, seed }
}

// Hash64 makes a string key hash with the 64-bit hashtextextended(key, seed)
// instead of the 32-bit hashtext.
//
// Deprecated: use WithHash64.
func Hash64(seed int64) Option { return WithHash64(seed) }

// keyValue turns a Key into the int64 or string the statements bind.
func keyValue[K Key](key K) any {
	v := reflect.ValueOf(key)
	if v.Kind() == reflect.String {
		return v.String()
	}
	return v.Int()
}

// keyExpr returns the SQL for the key argument of a lock function and its
// bound arguments: a plain key, or the hashed form of a text key. Distinct
// strings can collide, which only costs extra serialization, never a missed
// lock. hashtext is not guaranteed to give the same result across PostgreSQL
// major versions: never store a hashed key, and use an integer key when the key
// space is large enough for collisions to matter.
func keyExpr(key any, s settings) (string, []any) {
	k, isString := key.(string)
	if !isString {
		return "?", []any{key}
	}
	if s.wide {
		return "hashtextextended(?, ?)", []any{k, s.seed}
	}
	return "hashtext(?)", []any{k}
}

// lockFunc names the PostgreSQL function Acquire runs for these options.
func lockFunc(s settings) string {
	name := "pg_"
	if s.try {
		name += "try_"
	}
	name += "advisory_"
	if s.xact {
		name += "xact_"
	}
	name += "lock"
	if s.shared {
		name += "_shared"
	}
	return name
}

// unlockFunc names the PostgreSQL function Release runs for these options.
func unlockFunc(s settings) string {
	if s.shared {
		return "pg_advisory_unlock_shared"
	}
	return "pg_advisory_unlock"
}

// Acquire takes the advisory lock for key and reports whether it is held.
//
// Without options it takes the session-scoped exclusive lock (pg_advisory_lock)
// on the pinned connection behind q, which must be the *momo.DB of DB.OnConn,
// waits until it is granted and returns true. The lock is held until Release or
// until the connection closes; a commit does not release it. A key the same
// session already holds is granted at once and counted, so it needs one Release
// per Acquire, as on PostgreSQL.
//
// Xact makes it transaction-scoped and q a *momo.Tx; Shared takes the shared
// lock; Try returns false instead of waiting. The options combine freely
// (Xact, Shared and Try together run pg_try_advisory_xact_lock_shared), except
// that MySQL and MariaDB run only the plain and the Try forms. A string key is
// hashed on the server; see WithHash64.
//
// A nil q, a nil *momo.Tx with Xact, a pooled *momo.DB, a *momo.DB with Xact and
// a *momo.Tx without it fail with momo.ErrValidation. Dialects that cannot run
// the requested lock fail with momo.ErrUnsupported.
//
// On MySQL and MariaDB it runs GET_LOCK on a name scoped to the current database
// (see the package comment), in one-second waits until the lock is granted or
// ctx ends. A NULL result from the server is an error. After Acquire returns an
// error, call Discard on the pinned connection instead of Release: a GET_LOCK
// that the server granted while the call was being abandoned may still be
// attached to the connection, and Discard closes it, which frees the lock.
func Acquire[K Key](ctx context.Context, q momo.Querier, key K, opts ...Option) (bool, error) {
	s := resolve(opts)
	k := keyValue(key)
	if s.xact {
		tx, _ := q.(*momo.Tx)
		if err := check(tx); err != nil {
			return false, err
		}
		return acquire(ctx, tx, s, k)
	}
	db, err := sessionDB(q)
	if err != nil {
		return false, err
	}
	if err := checkSession(db, s.shared); err != nil {
		return false, err
	}
	if isMySQL(db) {
		if s.try {
			return mysqlTry(ctx, db, k)
		}
		if err := mysqlLock(ctx, db, k); err != nil {
			return false, err
		}
		return true, nil
	}
	return acquire(ctx, db, s, k)
}

// acquire runs the PostgreSQL lock function the options name.
func acquire(ctx context.Context, q momo.Querier, s settings, key any) (bool, error) {
	if s.try {
		return try(ctx, q, lockFunc(s), key, s)
	}
	if err := lock(ctx, q, lockFunc(s), key, s); err != nil {
		return false, err
	}
	return true, nil
}

// Release releases one hold of the session-scoped lock for key
// (pg_advisory_unlock, RELEASE_LOCK on MySQL and MariaDB) on the pinned
// connection behind q and reports whether the session held it. Locks are
// counted: a key acquired twice needs two releases. Pass the same key, the same
// WithHash64 option and the same Shared option the Acquire used, or the release
// targets a different lock: Release without Shared does not release a shared
// lock, and Release with Shared does not release an exclusive one.
//
// A transaction-scoped lock has no release, so Xact fails with
// momo.ErrValidation, as does Try. The other refusals are those of Acquire.
func Release[K Key](ctx context.Context, q momo.Querier, key K, opts ...Option) (bool, error) {
	s := resolve(opts)
	if s.xact {
		return false, &momo.Error{Kind: momo.ErrValidation, Op: "advisory lock", Err: errors.New("a transaction-scoped advisory lock has no release: it ends at commit or rollback")}
	}
	if s.try {
		return false, &momo.Error{Kind: momo.ErrValidation, Op: "advisory lock", Err: errors.New("Release does not wait, so Try has no meaning for it")}
	}
	k := keyValue(key)
	db, err := sessionDB(q)
	if err != nil {
		return false, err
	}
	if err := checkSession(db, s.shared); err != nil {
		return false, err
	}
	if isMySQL(db) {
		return mysqlUnlock(ctx, db, k)
	}
	return try(ctx, db, unlockFunc(s), k, s)
}

// sessionDB returns the *momo.DB a session-scoped lock runs on. A nil handle
// comes back as a nil *momo.DB, which checkSession refuses.
func sessionDB(q momo.Querier) (*momo.DB, error) {
	switch v := q.(type) {
	case *momo.DB:
		return v, nil
	case nil:
		return nil, nil
	case *momo.Tx:
		return nil, &momo.Error{Kind: momo.ErrValidation, Op: "advisory lock", Err: errors.New("a *momo.Tx takes only the transaction-scoped lock: pass advisory.Xact(), or use the pinned *momo.DB of DB.OnConn for a session lock")}
	default:
		return nil, &momo.Error{Kind: momo.ErrValidation, Op: "advisory lock", Err: errors.New("a session-level advisory lock needs the *momo.DB of DB.OnConn")}
	}
}

func check(tx *momo.Tx) error {
	if tx == nil {
		return &momo.Error{Kind: momo.ErrValidation, Op: "advisory lock", Err: errors.New("a transaction-level advisory lock needs a transaction; pass the *momo.Tx from Begin or DB.Tx")}
	}
	if tx.Dialect().Name != dialect.Postgres {
		return &momo.Error{Kind: momo.ErrUnsupported, Op: "advisory lock", Err: errors.New("transaction-scoped advisory locks need PostgreSQL: on MySQL and MariaDB GET_LOCK locks belong to the session and a commit does not release them; use the session functions on a pinned connection")}
	}
	return nil
}

func lock(ctx context.Context, q momo.Querier, fn string, key any, s settings) error {
	k, args := keyExpr(key, s)
	_, err := momo.Exec(ctx, q, "SELECT "+fn+"("+k+")", args...)
	return err
}

func try(ctx context.Context, q momo.Querier, fn string, key any, s settings) (bool, error) {
	k, args := keyExpr(key, s)
	got, err := momo.Scalars[bool](ctx, q, "SELECT "+fn+"("+k+")", args...)
	if err != nil {
		return false, err
	}
	if len(got) != 1 {
		return false, &momo.Error{Kind: momo.ErrNotFound, Op: "advisory lock", Err: errors.New(fn + " returned no row")}
	}
	return got[0], nil
}

// checkSession validates a handle for a session-scoped lock. The exclusive
// session locks run on PostgreSQL and on MySQL and MariaDB; the shared ones
// (shared true) have no MySQL counterpart and run on PostgreSQL alone.
func checkSession(db *momo.DB, shared bool) error {
	if db == nil {
		return &momo.Error{Kind: momo.ErrValidation, Op: "advisory lock", Err: errors.New("a session-level advisory lock needs a pinned connection; pass db.OnConn(pc)")}
	}
	switch name := db.Dialect().Name; {
	case name == dialect.Postgres:
	case name == dialect.MySQL && !shared:
	case name == dialect.MySQL:
		return &momo.Error{Kind: momo.ErrUnsupported, Op: "advisory lock", Err: errors.New("shared advisory locks need PostgreSQL: GET_LOCK has no shared mode")}
	default:
		return &momo.Error{Kind: momo.ErrUnsupported, Op: "advisory lock", Err: errors.New("session-scoped advisory locks need PostgreSQL, MySQL or MariaDB")}
	}
	if !db.IsPinned() {
		return &momo.Error{Kind: momo.ErrValidation, Op: "advisory lock", Err: errors.New("a session-level advisory lock needs a pinned connection: a pooled *momo.DB may run the lock and the unlock on different connections; pass db.OnConn(pc)")}
	}
	return nil
}
