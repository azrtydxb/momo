// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
)

// unlockTimeout bounds the release statement, which runs detached from the
// caller's context so a cancelled apply still frees the lock.
const unlockTimeout = 5 * time.Second

// Lock takes the migration lock: one row (id = 1) in the table
// momo_migration_lock, which Lock creates when it is missing. SQLite has no
// advisory locks, and a lock held by a transaction would not survive the
// autocommit statements of a migration that opts out of one, so the lock is a
// row that outlives statements and is removed by unlock.
//
// The key is not used: a SQLite database file has one migration history, so it
// has one lock. When another holder has the row, Lock returns migrate.ErrLocked
// with the holder named ("pid=N host=H at=T") and a holder function that reads
// it again. Otherwise it returns unlock, which deletes the row only while it
// still carries this holder text, so a lock released with ForceUnlock and
// taken by another process is not removed. unlock ignores cancellation of ctx.
//
// A process that dies while it holds the lock leaves the row behind. After
// confirming that no process is migrating, remove it with ForceUnlock.
func (Dialect) Lock(ctx context.Context, c momo.PinnedConn, _ string) (unlock func() error, holder func(context.Context) (string, error), err error) {
	holder = func(ctx context.Context) (string, error) { return lockHolder(ctx, c) }
	if _, err := c.Exec(ctx, `CREATE TABLE IF NOT EXISTS `+lockTable+` (
id INTEGER PRIMARY KEY CHECK (id = 1),
holder TEXT NOT NULL,
acquired_at TEXT NOT NULL
)`, nil); err != nil {
		return nil, nil, fmt.Errorf("migrate/sqlite: create the lock table: %w", err)
	}
	host, herr := os.Hostname()
	if herr != nil || host == "" {
		host = "unknown"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	me := fmt.Sprintf("pid=%d host=%s at=%s", os.Getpid(), host, now)
	_, err = c.Exec(ctx, `INSERT INTO `+lockTable+` (id, holder, acquired_at) VALUES (1, ?, ?)`, []any{me, now})
	if errors.Is(err, momo.ErrUniqueViolation) {
		text, lerr := holder(ctx)
		if lerr != nil {
			return nil, holder, fmt.Errorf("%w (holder lookup failed: %v)", migrate.ErrLocked, lerr)
		}
		if text == "" {
			text = "released while it was being read; retry"
		}
		return nil, holder, fmt.Errorf("%w: held by %s", migrate.ErrLocked, text)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("migrate/sqlite: acquire the migration lock: %w", err)
	}
	unlock = func() error {
		uctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), unlockTimeout)
		defer cancel()
		n, err := c.Exec(uctx, `DELETE FROM `+lockTable+` WHERE id = 1 AND holder = ?`, []any{me})
		if err != nil {
			return fmt.Errorf("migrate/sqlite: release the migration lock: %w", err)
		}
		if n == 0 {
			return errors.New("migrate/sqlite: the migration lock was no longer held by this process (it was removed or taken over)")
		}
		return nil
	}
	return unlock, holder, nil
}

// lockHolder returns the holder text of the lock row, or "" when there is none.
func lockHolder(ctx context.Context, c momo.PinnedConn) (string, error) {
	var h string
	err := queryOne(ctx, c, `SELECT holder FROM `+lockTable+` WHERE id = 1`, nil, &h)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return h, nil
}

// ForceUnlock removes the lock row whoever holds it and returns the holder text
// it removed ("" when nobody held the lock). It is for a lock left behind by a
// process that died; calling it while a migration runs lets a second process
// start. A database without the lock table has nothing to remove.
func ForceUnlock(ctx context.Context, c momo.PinnedConn) (previousHolder string, err error) {
	previousHolder, err = lockHolder(ctx, c)
	if err != nil {
		if isNoSuchTable(err) {
			return "", nil
		}
		return "", fmt.Errorf("migrate/sqlite: read the migration lock: %w", err)
	}
	if _, err := c.Exec(ctx, `DELETE FROM `+lockTable+` WHERE id = 1`, nil); err != nil {
		return "", fmt.Errorf("migrate/sqlite: remove the migration lock: %w", err)
	}
	return previousHolder, nil
}
