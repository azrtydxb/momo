// SPDX-License-Identifier: Apache-2.0

//go:build integration

package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/sqlitedriver"
	"github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

type execer interface {
	Exec(ctx context.Context, sql string, args []any) (int64, error)
}

func mustExec(t *testing.T, x execer, sql string) {
	t.Helper()
	if _, err := x.Exec(context.Background(), sql, nil); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// pinned pins one connection of db and releases it when the test ends. The
// pool testopen.SQLite opens has one connection, so a test holds at most one
// pin at a time and runs everything else through that pin.
func pinned(t *testing.T, db *momo.DB) momo.PinnedConn {
	t.Helper()
	pn, ok := db.Driver().(momo.Pinner)
	if !ok {
		t.Fatal("driver does not implement momo.Pinner")
	}
	c, err := pn.Pin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Release() }) // Cleanup has no error channel; Release failures do not affect the assertions
	return c
}

// release gives a pin back before the end of the test, for a test that goes on
// to use the one-connection pool through the driver or an Engine.
func release(t *testing.T, c momo.PinnedConn) {
	t.Helper()
	if err := c.Release(); err != nil {
		t.Fatal(err)
	}
}

// openSQLite opens a fresh database file with conns connections, for the tests
// that need two sessions at once (testopen.SQLite opens one).
func openSQLite(t *testing.T, conns int) *momo.DB {
	t.Helper()
	sqldb, drv, err := sqlitedriver.Open(testdb.SQLitePath(t), sqlitedriver.Config{MaxOpenConns: conns})
	if errors.Is(err, momo.ErrUnsupported) {
		t.Skipf("SQLite needs cgo and a non-Windows build: %v", err)
	}
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := sqldb.Close(); err != nil {
			t.Errorf("close sqlite: %v", err)
		}
	})
	return momo.New(drv)
}

// scanOne runs a query on c, requires one row and scans it into dest.
func scanOne(t *testing.T, c momo.PinnedConn, sql string, dest ...any) {
	t.Helper()
	rows, err := c.Query(context.Background(), sql, nil)
	if err != nil {
		t.Fatalf("query %q: %v", sql, err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("no rows for %q", sql)
	}
	if err := rows.Scan(dest...); err != nil {
		t.Fatal(err)
	}
}

// newRebuildFixture returns a pinned connection with foreign_keys ON, a parent
// table items with an index, a view, a trigger on items, a child table links
// with an FK to items and a trigger on links that updates items, and three
// parent rows and two child rows.
func newRebuildFixture(t *testing.T) momo.PinnedConn {
	t.Helper()
	db := testopen.SQLite(t)
	c := pinned(t, db)
	mustExec(t, c, `PRAGMA foreign_keys = ON`)
	mustExec(t, c, `CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT, qty INTEGER)`)
	mustExec(t, c, `CREATE INDEX items_name ON items(name)`)
	mustExec(t, c, `CREATE TRIGGER items_audit AFTER UPDATE ON items BEGIN SELECT 1; END`)
	mustExec(t, c, `CREATE VIEW items_v AS SELECT id, name FROM items`)
	mustExec(t, c, `CREATE TABLE links (id INTEGER PRIMARY KEY, item_id INTEGER NOT NULL REFERENCES items(id))`)
	mustExec(t, c, `CREATE TRIGGER links_touch AFTER INSERT ON links BEGIN UPDATE items SET qty = qty WHERE id = NEW.item_id; END`)
	mustExec(t, c, `INSERT INTO items (name, qty) VALUES ('a', 1), ('b', 2), ('c', 3)`)
	mustExec(t, c, `INSERT INTO links (item_id) VALUES (1), (2)`)
	return c
}
