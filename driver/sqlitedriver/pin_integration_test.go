// SPDX-License-Identifier: Apache-2.0

//go:build integration

package sqlitedriver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
)

func TestBusyIsStructuredError(t *testing.T) {
	dsn := testdb.SQLitePath(t)
	db1, d1, err := Open(dsn, Config{BusyTimeout: 50 * time.Millisecond, MaxOpenConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db1.Close() })
	db2, d2, err := Open(dsn, Config{BusyTimeout: 50 * time.Millisecond, MaxOpenConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db2.Close() })
	ctx := context.Background()
	if _, err := db1.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS t_busy (x int)"); err != nil {
		t.Fatal(err)
	}
	tx, err := d1.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "INSERT INTO t_busy VALUES (1)", nil); err != nil {
		t.Fatal(err)
	}
	_, err = d2.Exec(ctx, "INSERT INTO t_busy VALUES (2)", nil)
	if !errors.Is(err, momo.ErrBusy) {
		t.Fatalf("second writer err = %v, want momo.ErrBusy", err)
	}
}

func TestSQLitePinnedAndStats(t *testing.T) {
	db, d, err := Open(testdb.SQLitePath(t), Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	p, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st := d.(momo.PoolStater).PoolStats(); st.AcquiredConns != 1 {
		t.Fatalf("AcquiredConns = %d", st.AcquiredConns)
	}
	if err := p.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "SELECT 1", nil); err == nil {
		t.Fatal("pinned connection usable after Release")
	}
}
