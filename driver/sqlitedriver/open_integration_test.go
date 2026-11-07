// SPDX-License-Identifier: Apache-2.0

//go:build integration

package sqlitedriver

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
)

// TestSQLiteRecommendedConfig is S-57: the adapter's connections run WAL, the
// busy timeout, NORMAL synchronous and foreign keys.
func TestSQLiteRecommendedConfig(t *testing.T) {
	db, _, err := Open(testdb.SQLitePath(t), Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	var mode string
	var syncMode, fk, busy int
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&syncMode); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busy); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" || syncMode != 1 || fk != 1 || busy != 5000 {
		t.Fatalf("journal=%s synchronous=%d foreign_keys=%d busy_timeout=%d", mode, syncMode, fk, busy)
	}
}

// TestSQLiteConcurrentWritersAreClassified runs writers from two separate
// handles on one file. Every statement either succeeds or fails with the
// structured busy error; a raw SQLITE_BUSY must never surface (S-57).
func TestSQLiteConcurrentWritersAreClassified(t *testing.T) {
	path := testdb.SQLitePath(t)
	ctx := context.Background()
	var drivers []momo.Driver
	for range 2 {
		db, d, err := Open(path, Config{BusyTimeout: 20 * time.Millisecond})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		drivers = append(drivers, d)
	}
	if _, err := drivers[0].Exec(ctx, "CREATE TABLE w (n integer)", nil); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 200)
	for _, d := range drivers {
		for g := range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range 25 {
					if _, err := d.Exec(ctx, "INSERT INTO w VALUES (?)", []any{g*100 + i}); err != nil {
						errs <- err
					}
				}
			}()
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if !errors.Is(err, momo.ErrBusy) {
			t.Fatalf("writer failed with an unclassified error: %v", err)
		}
	}
}
