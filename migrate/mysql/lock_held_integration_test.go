// SPDX-License-Identifier: Apache-2.0

//go:build integration

package mysql_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
)

// TestMySQLApplyRefusedWhileLockHeld holds the migration lock on a pinned
// connection, so the outcome does not depend on timing: Apply with no wait is
// refused with ErrLocked and applies nothing, and once the lock is released
// the same engine applies every version.
//
// Catches: an Apply that runs without the lock, or one that keeps failing
// after the holder is gone.
func TestMySQLApplyRefusedWhileLockHeld(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		names, err := momo.Scalars[string](ctx, db, "SELECT DATABASE()")
		if err != nil || len(names) != 1 {
			t.Fatalf("DATABASE(): %v, %v", names, err)
		}
		d := mmysql.New()
		key := d.HistoryLockKey("`" + names[0] + "`.`momo_migrations`")

		pc, err := db.Driver().(momo.Pinner).Pin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = pc.Release() }()
		unlock, _, err := d.Lock(ctx, pc, key)
		if err != nil {
			t.Fatalf("hold the lock: %v", err)
		}

		e, err := migrate.NewEngine(db.Driver(), concurrentVersions(), migrate.WithDialect(d), migrate.WithoutSum(), migrate.WithLockWait(0))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Apply(ctx, migrate.ApplyOptions{}); !errors.Is(err, migrate.ErrLocked) {
			t.Fatalf("apply while the lock is held: err = %v, want migrate.ErrLocked", err)
		}
		if n := tableCount(t, db, "a", "b"); n != 0 {
			t.Fatalf("tables present after a refused apply = %d, want 0", n)
		}

		if err := unlock(); err != nil {
			t.Fatal(err)
		}
		if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
			t.Fatalf("apply after the lock is released: %v", err)
		}
		if got := appliedVersions(t, db); len(got) != 2 {
			t.Fatalf("applied versions = %v, want 2", got)
		}
	})
}
