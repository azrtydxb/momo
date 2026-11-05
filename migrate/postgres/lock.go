// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
)

// unlockTimeout bounds the release statements, which run detached from the
// caller's context so a cancelled apply still frees the session.
const unlockTimeout = 5 * time.Second

// lockID derives the advisory lock id from the lock key (fnv64a).
func lockID(key string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return int64(h.Sum64())
}

// Lock takes a session-level advisory lock on the pinned connection with the
// non-blocking pg_try_advisory_lock; the engine owns the wait loop. The key is
// hashed with fnv64a. Advisory locks are database-wide, so the engine passes
// "momo:<history schema>.<history table>" to keep schemas independent.
//
// When another session holds the lock, Lock returns migrate.ErrLocked with the
// holder named ("held by pid=N application_name=X") and a holder function that
// repeats that query. Otherwise it returns unlock, which releases the lock on
// the same connection and is safe to call from a defer on any path: it ignores
// cancellation of ctx. The lock is session-scoped, so it is useless behind a
// transaction-mode pooler such as PgBouncer; run migrations on a direct or
// session-pooled connection.
//
// Order: run the reset returned by Setup before unlock, so a small
// statement_timeout cannot cancel the unlock statement.
func (Dialect) Lock(ctx context.Context, c momo.PinnedConn, key string) (unlock func() error, holder func(context.Context) (string, error), err error) {
	id := lockID(key)
	holder = func(ctx context.Context) (string, error) { return holderText(ctx, c, id) }
	var got bool
	if err := queryOne(ctx, c, `SELECT pg_try_advisory_lock($1)`, []any{id}, &got); err != nil {
		return nil, nil, fmt.Errorf("migrate/postgres: acquire advisory lock: %w", err)
	}
	if !got {
		text, herr := holder(ctx)
		if herr != nil {
			return nil, holder, fmt.Errorf("%w (holder lookup failed: %v)", migrate.ErrLocked, herr)
		}
		return nil, holder, fmt.Errorf("%w: %s", migrate.ErrLocked, text)
	}
	unlock = func() error {
		uctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), unlockTimeout)
		defer cancel()
		var released bool
		if err := queryOne(uctx, c, `SELECT pg_advisory_unlock($1)`, []any{id}, &released); err != nil {
			// Last resort: drop every session lock so none outlives the apply.
			// This also drops session advisory locks a Go migration took on
			// the same connection, which is acceptable here. If even that
			// fails, destroy the session so the pool cannot hand it out
			// still holding the lock.
			_, aerr := c.Exec(uctx, `SELECT pg_advisory_unlock_all()`, nil)
			if aerr != nil {
				aerr = errors.Join(aerr, discard(c))
			}
			return errors.Join(fmt.Errorf("migrate/postgres: release advisory lock: %w", err), aerr)
		}
		if !released {
			return errors.New("migrate/postgres: advisory lock was not held by this session")
		}
		return nil
	}
	return unlock, holder, nil
}

// holderText names the session holding lock id.
func holderText(ctx context.Context, c momo.PinnedConn, id int64) (string, error) {
	rows, err := c.Query(ctx, `SELECT a.pid, COALESCE(a.application_name, '') FROM pg_locks l JOIN pg_stat_activity a ON a.pid = l.pid
WHERE l.locktype = 'advisory' AND l.granted AND l.objsubid = 1
AND l.classid = (($1::bigint >> 32) & 4294967295)::oid AND l.objid = ($1::bigint & 4294967295)::oid`, []any{id})
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", err
		}
		return "held by an unknown session", nil
	}
	var pid int32
	var app string
	if err := rows.Scan(&pid, &app); err != nil {
		return "", err
	}
	return fmt.Sprintf("held by pid=%d application_name=%s", pid, app), nil
}
