// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
)

// HistoryDDL returns the momo_migrations DDL. The table is created unqualified,
// so it lives in the first schema of the session's search_path.
func (Dialect) HistoryDDL() []string {
	return []string{`CREATE TABLE IF NOT EXISTS momo_migrations (
version TEXT PRIMARY KEY,
name TEXT NOT NULL,
checksum TEXT NOT NULL,
status TEXT NOT NULL,
statements_applied INTEGER NOT NULL DEFAULT 0,
error TEXT NOT NULL DEFAULT '',
applied_at TEXT NOT NULL,
execution_ms BIGINT NOT NULL DEFAULT 0,
apply_id TEXT NOT NULL DEFAULT '',
kind TEXT NOT NULL DEFAULT 'sql'
)`}
}

// Setup applies lock_timeout and statement_timeout at session scope on the
// pinned connection; a zero duration leaves that setting untouched. The
// returned reset restores both to the session defaults (RESET), so nothing
// leaks into the pool when the connection is released. reset is idempotent and
// ignores cancellation of ctx. If Setup itself fails part-way, the settings it
// already applied are reset before it returns.
func (Dialect) Setup(ctx context.Context, c momo.PinnedConn, s migrate.SessionOptions) (reset func() error, err error) {
	reset = func() error {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), unlockTimeout)
		defer cancel()
		_, e1 := c.Exec(rctx, `RESET lock_timeout`, nil)
		_, e2 := c.Exec(rctx, `RESET statement_timeout`, nil)
		return errors.Join(e1, e2)
	}
	for _, st := range []struct {
		name string
		d    time.Duration
	}{{"lock_timeout", s.LockTimeout}, {"statement_timeout", s.StatementTimeout}} {
		if st.d <= 0 {
			continue
		}
		// SET takes no bind parameters; the value is a formatted integer.
		if _, err := c.Exec(ctx, fmt.Sprintf("SET %s = %d", st.name, st.d.Milliseconds()), nil); err != nil {
			return nil, errors.Join(fmt.Errorf("migrate/postgres: set %s: %w", st.name, err), reset())
		}
	}
	return reset, nil
}
