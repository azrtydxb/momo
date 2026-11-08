// SPDX-License-Identifier: Apache-2.0

//go:build integration

package sqlite_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/migrate"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
)

func TestSQLiteLockTable(t *testing.T) {
	// Two sessions, so two connections: testopen.SQLite opens a pool of one.
	db := openSQLite(t, 2)
	ctx := context.Background()
	a, b := pinned(t, db), pinned(t, db)
	unlockA, _, err := msqlite.New().Lock(ctx, a, "dir")
	if err != nil {
		t.Fatal(err)
	}
	_, holderB, err := msqlite.New().Lock(ctx, b, "dir")
	if !errors.Is(err, migrate.ErrLocked) {
		t.Fatalf("second lock must fail with ErrLocked, got %v", err)
	}
	h, herr := holderB(ctx)
	if herr != nil {
		t.Fatal(herr)
	}
	if h == "" || !strings.Contains(h, "pid=") || !strings.Contains(h, "host=") || !strings.Contains(h, "at=") {
		t.Fatalf("holder must name the first owner as pid=N host=H at=T, got %q", h)
	}
	if !strings.Contains(err.Error(), h) {
		t.Fatalf("the ErrLocked message must name the holder %q, got %v", h, err)
	}
	if err := unlockA(); err != nil {
		t.Fatal(err)
	}
	if h, _ := holderB(ctx); h != "" {
		t.Fatalf("no holder after unlock, got %q", h)
	}
	unlockC, _, err := msqlite.New().Lock(ctx, b, "dir")
	if err != nil {
		t.Fatalf("lock must be reacquirable after unlock: %v", err)
	}
	if err := unlockC(); err != nil {
		t.Fatal(err)
	}
}

// TestSQLiteUnlockOnlyOwnRow checks that an unlock after a ForceUnlock and a
// new acquisition leaves the new holder's row alone and reports the loss.
func TestSQLiteUnlockOnlyOwnRow(t *testing.T) {
	db := openSQLite(t, 2)
	ctx := context.Background()
	a, b := pinned(t, db), pinned(t, db)
	unlockA, _, err := msqlite.New().Lock(ctx, a, "dir")
	if err != nil {
		t.Fatal(err)
	}
	prev, err := msqlite.ForceUnlock(ctx, b)
	if err != nil || prev == "" {
		t.Fatalf("ForceUnlock must return the previous holder, got %q, %v", prev, err)
	}
	unlockB, _, err := msqlite.New().Lock(ctx, b, "dir")
	if err != nil {
		t.Fatal(err)
	}
	if err := unlockA(); err == nil {
		t.Fatal("unlock of a lock that was taken over must report it")
	}
	var holder string
	scanOne(t, b, `SELECT holder FROM momo_migration_lock WHERE id = 1`, &holder)
	if holder == "" {
		t.Fatal("the new holder's row must survive the stale unlock")
	}
	if err := unlockB(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteForceUnlockWithoutTable(t *testing.T) {
	db := openSQLite(t, 1)
	prev, err := msqlite.ForceUnlock(context.Background(), pinned(t, db))
	if err != nil || prev != "" {
		t.Fatalf("a database without the lock table has nothing to remove, got %q, %v", prev, err)
	}
}

func TestSQLiteForeignKeysOffDuringRebuild(t *testing.T) {
	db := openSQLite(t, 1)
	ctx := context.Background()
	c := pinned(t, db)
	mustExec(t, c, `PRAGMA foreign_keys = ON`)
	reset, err := msqlite.New().Setup(ctx, c, migrate.SessionOptions{ForeignKeysOff: true})
	if err != nil {
		t.Fatal(err)
	}
	var fk int
	scanOne(t, c, `PRAGMA foreign_keys`, &fk)
	if fk != 0 {
		t.Fatal("foreign_keys must be off during rebuild")
	}
	if err := reset(); err != nil {
		t.Fatal(err)
	}
	scanOne(t, c, `PRAGMA foreign_keys`, &fk)
	if fk != 1 {
		t.Fatal("reset must turn foreign_keys back on")
	}
	if err := reset(); err != nil {
		t.Fatalf("reset must be idempotent: %v", err)
	}
}

func TestSQLiteSetupWithoutForeignKeysOffChangesNothing(t *testing.T) {
	db := openSQLite(t, 1)
	ctx := context.Background()
	c := pinned(t, db)
	reset, err := msqlite.New().Setup(ctx, c, migrate.SessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var fk int
	scanOne(t, c, `PRAGMA foreign_keys`, &fk)
	if fk != 1 {
		t.Fatalf("Setup without ForeignKeysOff must leave enforcement on, got %d", fk)
	}
	if err := reset(); err != nil {
		t.Fatal(err)
	}
}
