// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
)

// resetTimeout bounds the statement that restores the session, which runs
// detached from the caller's context so a cancelled apply still restores it.
const resetTimeout = 5 * time.Second

// HistoryDDL returns the momo_migrations DDL. SQLite stores the timestamps as
// RFC 3339 text, the same as the history writer sends them.
func (Dialect) HistoryDDL() []string {
	return []string{`CREATE TABLE IF NOT EXISTS ` + historyTable + ` (
version TEXT PRIMARY KEY,
name TEXT NOT NULL,
checksum TEXT NOT NULL,
status TEXT NOT NULL,
statements_applied INTEGER NOT NULL DEFAULT 0,
error TEXT NOT NULL DEFAULT '',
applied_at TEXT NOT NULL,
execution_ms INTEGER NOT NULL DEFAULT 0,
apply_id TEXT NOT NULL DEFAULT '',
kind TEXT NOT NULL DEFAULT 'sql'
)`}
}

// Setup prepares the session for a migration. When s.ForeignKeysOff is set it
// turns foreign-key enforcement off with PRAGMA foreign_keys = OFF, which a
// table rebuild needs (step 1 of the procedure in the SQLite documentation:
// DROP TABLE would otherwise delete or cascade through the rows of the child
// tables). The PRAGMA is a no-op inside a transaction, so Setup must run
// outside one; it reads the setting back and fails when it did not change.
// The returned reset restores the value Setup found. It is idempotent and
// ignores cancellation of ctx; when the restore fails it discards the
// connection (momo.Discarder) so that a session with enforcement off cannot
// return to the pool.
//
// The lock and statement timeouts of SessionOptions have no SQLite
// counterpart here: busy handling belongs to the driver (busy_timeout), so
// they are ignored.
func (Dialect) Setup(ctx context.Context, c momo.PinnedConn, s migrate.SessionOptions) (reset func() error, err error) {
	if !s.ForeignKeysOff {
		return func() error { return nil }, nil
	}
	prev, err := foreignKeys(ctx, c)
	if err != nil {
		return nil, err
	}
	if prev == 0 {
		return func() error { return nil }, nil
	}
	if _, err := c.Exec(ctx, `PRAGMA foreign_keys = OFF`, nil); err != nil {
		return nil, fmt.Errorf("migrate/sqlite: turn foreign keys off: %w", err)
	}
	now, err := foreignKeys(ctx, c)
	if err != nil {
		return nil, errors.Join(err, restoreForeignKeys(ctx, c))
	}
	if now != 0 {
		return nil, errors.Join(errors.New("migrate/sqlite: PRAGMA foreign_keys = OFF had no effect; Setup must run outside a transaction"), restoreForeignKeys(ctx, c))
	}
	done := false
	return func() error {
		if done {
			return nil
		}
		err := restoreForeignKeys(ctx, c)
		if err != nil {
			if d, ok := c.(momo.Discarder); ok {
				err = errors.Join(err, d.Discard())
			} else {
				err = errors.Join(err, errors.New("migrate/sqlite: pinned connection cannot be discarded; foreign-key enforcement stays off on it"))
			}
			return err
		}
		done = true
		return nil
	}, nil
}

// restoreForeignKeys turns enforcement back on, detached from ctx.
func restoreForeignKeys(ctx context.Context, c momo.PinnedConn) error {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), resetTimeout)
	defer cancel()
	if _, err := c.Exec(rctx, `PRAGMA foreign_keys = ON`, nil); err != nil {
		return fmt.Errorf("migrate/sqlite: restore foreign keys: %w", err)
	}
	got, err := foreignKeys(rctx, c)
	if err != nil {
		return err
	}
	if got != 1 {
		return errors.New("migrate/sqlite: foreign keys are still off after PRAGMA foreign_keys = ON; the connection is inside a transaction")
	}
	return nil
}

// foreignKeys reads PRAGMA foreign_keys.
func foreignKeys(ctx context.Context, c momo.PinnedConn) (int, error) {
	var v int
	if err := queryOne(ctx, c, `PRAGMA foreign_keys`, nil, &v); err != nil {
		return 0, fmt.Errorf("migrate/sqlite: read foreign_keys: %w", err)
	}
	return v, nil
}

// queryOne runs a query that returns one row and scans it into dest.
func queryOne(ctx context.Context, c momo.PinnedConn, q string, args []any, dest ...any) error {
	rows, err := c.Query(ctx, q, args)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	return rows.Scan(dest...)
}

// forEach runs a query and calls fn for every row.
func forEach(ctx context.Context, c momo.PinnedConn, q string, args []any, fn func(momo.Rows) error) error {
	rows, err := c.Query(ctx, q, args)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := fn(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}
