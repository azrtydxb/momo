// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"context"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
)

func pinned(t *testing.T, db *momo.DB) momo.PinnedConn {
	t.Helper()
	pc, err := db.Driver().(momo.Pinner).Pin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Release() })
	return pc
}

func mustExec(t *testing.T, c momo.PinnedConn, sql string) {
	t.Helper()
	if _, err := c.Exec(context.Background(), sql, nil); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func historyRow(t *testing.T, db *momo.DB, version string) migrate.HistoryRow {
	t.Helper()
	c := pinned(t, db)
	rows, err := migrate.ReadHistory(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Version == version {
			return r
		}
	}
	t.Fatalf("no history row for %s in %+v", version, rows)
	return migrate.HistoryRow{}
}

func tableExists(t *testing.T, db *momo.DB, name string) bool {
	t.Helper()
	rows, err := pinned(t, db).Query(context.Background(), `SELECT to_regclass($1) IS NOT NULL`, []any{name})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var ok bool
	if !rows.Next() {
		t.Fatal("no row")
	}
	if err := rows.Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

// cancelWhenRunning calls cancel once a statement containing marker is active
// on the server, or after 20s if none shows up. Cancelling on a wall-clock
// delay instead raced slow starts under load: the cancel could land before the
// history table existed. marker should be unique to the test (a SQL comment
// carrying the schema name) so another session running the same test against
// the shared database cannot trigger it. It pins one connection for the
// watcher; the watcher is stopped before that connection is released.
func cancelWhenRunning(t *testing.T, db *momo.DB, marker string, cancel context.CancelFunc) {
	t.Helper()
	// Pin once, not per poll: a pin per iteration is never returned to the
	// pool until the test ends, so a slow start exhausts the pool and blocks
	// the watcher and the code under test.
	watch := pinned(t, db)
	stop := make(chan struct{})
	done := make(chan struct{})
	t.Cleanup(func() { close(stop); <-done }) // runs before pinned's release (LIFO)
	go func() {
		defer close(done)
		defer cancel()
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			select {
			case <-stop:
				return
			default:
			}
			rows, err := watch.Query(context.Background(), `SELECT count(*) FROM pg_stat_activity WHERE state = 'active' AND position($1 in query) > 0 AND pid <> pg_backend_pid()`, []any{marker})
			if err == nil {
				n := 0
				if rows.Next() {
					_ = rows.Scan(&n)
				}
				_ = rows.Close()
				if n > 0 {
					return
				}
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
}
