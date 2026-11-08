// SPDX-License-Identifier: Apache-2.0

package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
)

// unlockTimeout bounds the release statements, which run detached from the
// caller's context so a cancelled apply still frees the session.
const unlockTimeout = 5 * time.Second

// maxLockName is the longest name GET_LOCK accepts.
const maxLockName = 64

// Lock takes a named lock on the pinned connection with GET_LOCK. It never
// blocks: GET_LOCK waits zero seconds and the engine owns the wait loop, which
// retries until ApplyOptions.LockWait expires. A negative timeout would wait
// forever on MySQL and return NULL on MariaDB, so it is not used.
//
// When another session holds the lock, Lock returns migrate.ErrLocked naming
// the connection that holds it (IS_USED_LOCK), and a holder function that
// repeats that lookup. Otherwise it returns unlock, which releases the lock on
// the same connection and ignores cancellation of ctx. The lock belongs to the
// session, so the connection stays pinned for the lifetime of the lock; a
// server restart or a lost connection frees it. Lock names are server-wide, so
// the engine key carries the database.
//
// Order: run the reset returned by Setup before unlock.
func (Dialect) Lock(ctx context.Context, c momo.PinnedConn, key string) (unlock func() error, holder func(context.Context) (string, error), err error) {
	name := lockName(key)
	holder = func(ctx context.Context) (string, error) {
		id, ok, err := scalar(ctx, c, `SELECT IS_USED_LOCK(?)`, name)
		if err != nil {
			return "", err
		}
		if !ok {
			return "no connection holds it any more", nil
		}
		return "held by connection " + strconv.FormatInt(id, 10), nil
	}
	got, ok, err := scalar(ctx, c, `SELECT GET_LOCK(?, 0)`, name)
	if err != nil {
		return nil, nil, fmt.Errorf("migrate/mysql: acquire lock: %w", err)
	}
	if !ok {
		return nil, nil, errors.New("migrate/mysql: GET_LOCK returned NULL; the server could not take the lock")
	}
	if got != 1 {
		text, herr := holder(ctx)
		if herr != nil {
			return nil, holder, fmt.Errorf("%w (holder lookup failed: %v)", migrate.ErrLocked, herr)
		}
		return nil, holder, fmt.Errorf("%w: %s", migrate.ErrLocked, text)
	}
	unlock = func() error {
		uctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), unlockTimeout)
		defer cancel()
		released, ok, err := scalar(uctx, c, `SELECT RELEASE_LOCK(?)`, name)
		if err != nil {
			// Closing the session frees the lock, so a connection that cannot
			// release it is destroyed rather than returned to the pool.
			return errors.Join(fmt.Errorf("migrate/mysql: release lock: %w", err), discard(c))
		}
		if !ok || released != 1 {
			return errors.New("migrate/mysql: the lock was not held by this session")
		}
		return nil
	}
	return unlock, holder, nil
}

// lockName maps an engine key to a GET_LOCK name. MySQL limits names to 64
// characters; a longer key keeps its first 55 and ends in a dash and eight hex
// digits of its fnv32a hash, so two long keys that differ only in the tail
// still get different names.
func lockName(key string) string {
	if len(key) <= maxLockName {
		return key
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	n := maxLockName - 9
	for n > 0 && !utf8.RuneStart(key[n]) {
		n-- // never cut a multi-byte character in half
	}
	return fmt.Sprintf("%s-%08x", key[:n], h.Sum32())
}

// scalar returns the first column of the first row as an integer. ok is false
// for a NULL value or no row.
func scalar(ctx context.Context, c momo.PinnedConn, query string, args ...any) (v int64, ok bool, err error) {
	rows, err := c.Query(ctx, query, args)
	if err != nil {
		return 0, false, err
	}
	defer func() {
		if cerr := rows.Close(); err == nil {
			err = cerr
		}
	}()
	var n sql.NullInt64
	if rows.Next() {
		if err := rows.Scan(&n); err != nil {
			return 0, false, err
		}
	}
	if err := rows.Err(); err != nil {
		return 0, false, err
	}
	return n.Int64, n.Valid, nil
}
