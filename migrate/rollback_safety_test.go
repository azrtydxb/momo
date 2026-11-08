// SPDX-License-Identifier: Apache-2.0

//go:build integration

package migrate_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
)

const rbVersion = "20260101000000"

// rbFiles is one migration whose down is the given text.
func rbFiles(down string) fstest.MapFS {
	return fstest.MapFS{
		rbVersion + "_a.up.sql":   {Data: []byte("CREATE TABLE a (x int);")},
		rbVersion + "_a.down.sql": {Data: []byte(down)},
	}
}

// failedNoTxDown drops the table, then fails: a down that stops half way.
// withSum returns fsys with a momo.sum that matches it, as a MapFS to edit.
func withSum(t *testing.T, fsys fstest.MapFS) fstest.MapFS {
	t.Helper()
	return migratetest.WithSum(t, fsys).(fstest.MapFS)
}

const failedNoTxDown = "-- momo:no-transaction\nDROP TABLE a;\nSELECT 1/0;"

// A down that fails outside a transaction leaves the row failed and the
// database dirty; both Apply and Rollback refuse until it is resolved.
func TestRollbackNoTxFailureLeavesRowFailed(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	e := sumEngine(t, db, withSum(t, rbFiles(failedNoTxDown)))
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Rollback(ctx, migrate.RollbackOptions{}); err == nil || !strings.Contains(err.Error(), "down "+rbVersion) {
		t.Fatalf("want the down failure naming the version, got %v", err)
	}
	row := historyRow(t, db, rbVersion)
	if row.Status != migrate.StatusFailed || row.StatementsApplied != 1 || !strings.HasPrefix(row.Error, "down:") {
		t.Fatalf("row = %+v, want failed with one statement done and a down: error", row)
	}
	if tableExists(t, db, "a") {
		t.Fatal("the first statement of the down ran outside a transaction and stays")
	}
	if _, err := e.Rollback(ctx, migrate.RollbackOptions{}); !errors.Is(err, migrate.ErrDirty) {
		t.Fatalf("Rollback on a dirty database: want ErrDirty, got %v", err)
	}
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); !errors.Is(err, migrate.ErrDirty) {
		t.Fatalf("Apply on a dirty database: want ErrDirty, got %v", err)
	}
}

// After a failed down, resolve rolled_back means the down was finished by
// hand: the row goes away and the version is pending again.
func TestResolveRolledBackAfterFailedDown(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	e := sumEngine(t, db, withSum(t, rbFiles(failedNoTxDown)))
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Rollback(ctx, migrate.RollbackOptions{}); err == nil {
		t.Fatal("the down must fail")
	}
	if err := e.Resolve(ctx, rbVersion, migrate.ResolveRolledBack); err != nil {
		t.Fatal(err)
	}
	rep, err := e.Apply(ctx, migrate.ApplyOptions{})
	if err != nil || len(rep.Applied) != 1 {
		t.Fatalf("the version must be pending again and apply: %+v %v", rep, err)
	}
	if !tableExists(t, db, "a") || historyRow(t, db, rbVersion).Status != migrate.StatusApplied {
		t.Fatal("re-apply must recreate the table and record it applied")
	}
}

// After a failed down, resolve applied means the down did not take effect (or
// was undone by hand): the migration stays applied and the database is clean.
func TestResolveAppliedAfterFailedDown(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	e := sumEngine(t, db, withSum(t, rbFiles(failedNoTxDown)))
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Rollback(ctx, migrate.RollbackOptions{}); err == nil {
		t.Fatal("the down must fail")
	}
	if err := e.Resolve(ctx, rbVersion, migrate.ResolveApplied); err != nil {
		t.Fatal(err)
	}
	if row := historyRow(t, db, rbVersion); row.Status != migrate.StatusApplied || row.Error != "" {
		t.Fatalf("row = %+v, want applied with no error", row)
	}
	rep, err := e.Apply(ctx, migrate.ApplyOptions{})
	if err != nil || len(rep.Applied) != 0 {
		t.Fatalf("a clean database with nothing pending: %+v %v", rep, err)
	}
}

// A transactional down that fails rolls back with its history update: the
// table stays and the row stays applied, so the database is not dirty.
func TestRollbackTransactionalFailureLeavesRowApplied(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	e := sumEngine(t, db, withSum(t, rbFiles("DROP TABLE a;\nSELECT 1/0;")))
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Rollback(ctx, migrate.RollbackOptions{}); err == nil {
		t.Fatal("the down must fail")
	}
	if !tableExists(t, db, "a") {
		t.Fatal("a failed transactional down must leave the table")
	}
	if row := historyRow(t, db, rbVersion); row.Status != migrate.StatusApplied {
		t.Fatalf("row status = %s, want applied", row.Status)
	}
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatalf("the database must not be dirty: %v", err)
	}
}

// An up file edited after it was applied, or any file that disagrees with
// momo.sum, stops Rollback before a statement runs.
func TestRollbackChecksumMismatch(t *testing.T) {
	ctx := context.Background()

	t.Run("edited up file", func(t *testing.T) {
		db := testopen.Postgres(t)
		e := sumEngine(t, db, withSum(t, rbFiles("DROP TABLE a;")))
		if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
			t.Fatal(err)
		}
		edited := rbFiles("DROP TABLE a;")
		edited[rbVersion+"_a.up.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE a (x int, y int);")}
		e2 := sumEngine(t, db, withSum(t, edited)) // momo.sum matches the edit
		_, err := e2.Rollback(ctx, migrate.RollbackOptions{})
		if !errors.Is(err, migrate.ErrChecksumMismatch) || !strings.Contains(err.Error(), rbVersion+"_a.up.sql") {
			t.Fatalf("want ErrChecksumMismatch naming the up file, got %v", err)
		}
		if !tableExists(t, db, "a") {
			t.Fatal("nothing may run")
		}
	})

	t.Run("tampered down file", func(t *testing.T) {
		db := testopen.Postgres(t)
		good := withSum(t, rbFiles("DROP TABLE a;"))
		e := sumEngine(t, db, good)
		if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
			t.Fatal(err)
		}
		tampered := fstest.MapFS{}
		for k, v := range good {
			tampered[k] = v
		}
		tampered[rbVersion+"_a.down.sql"] = &fstest.MapFile{Data: []byte("DROP TABLE a; DROP SCHEMA public CASCADE;")}
		_, err := sumEngine(t, db, tampered).Rollback(ctx, migrate.RollbackOptions{})
		if !errors.Is(err, migrate.ErrChecksumMismatch) || !strings.Contains(err.Error(), rbVersion+"_a.down.sql") {
			t.Fatalf("want ErrChecksumMismatch naming the down file, got %v", err)
		}
		if !tableExists(t, db, "a") {
			t.Fatal("a tampered down must not run")
		}
	})

	t.Run("tampered down file whose up file is gone", func(t *testing.T) {
		db := testopen.Postgres(t)
		e := sumEngine(t, db, withSum(t, rbFiles("DROP TABLE a;")))
		if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
			t.Fatal(err)
		}
		// momo.sum was regenerated without the up file; the down is then
		// edited, so only momo.sum can catch it.
		only := withSum(t, fstest.MapFS{rbVersion + "_a.down.sql": {Data: []byte("DROP TABLE a;")}})
		tampered := fstest.MapFS{}
		for k, v := range only {
			tampered[k] = v
		}
		tampered[rbVersion+"_a.down.sql"] = &fstest.MapFile{Data: []byte("DROP TABLE a; DROP TABLE momo_migrations;")}
		_, err := sumEngine(t, db, tampered).Rollback(ctx, migrate.RollbackOptions{})
		if !errors.Is(err, migrate.ErrChecksumMismatch) || !strings.Contains(err.Error(), rbVersion+"_a.down.sql") {
			t.Fatalf("want ErrChecksumMismatch naming the down file, got %v", err)
		}
		if !tableExists(t, db, "a") {
			t.Fatal("a tampered down must not run")
		}
	})
}

// Rollback takes the migration lock: with it held elsewhere it waits LockWait
// and fails with ErrLocked, running nothing.
func TestRollbackTakesTheLock(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	e := sumEngine(t, db, withSum(t, rbFiles("DROP TABLE a;")))
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	release := holdLock(t, db)
	start := time.Now()
	_, err := e.Rollback(ctx, migrate.RollbackOptions{LockWait: 600 * time.Millisecond})
	release()
	if !errors.Is(err, migrate.ErrLocked) {
		t.Fatalf("want ErrLocked, got %v", err)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Fatalf("Rollback gave up after %v; it must wait for LockWait", time.Since(start))
	}
	if !tableExists(t, db, "a") {
		t.Fatal("nothing may run without the lock")
	}
	if _, err := e.Rollback(ctx, migrate.RollbackOptions{}); err != nil {
		t.Fatalf("the lock must be free after the refused call: %v", err)
	}
}

// A Go migration is forward-only: a rollback whose range includes one is
// refused with ErrGoMigrationNotReversible before any down runs, for a single
// rollback and for a group, and the history stays as it was.
func TestRollbackRefusesGoMigration(t *testing.T) {
	ctx := context.Background()
	for _, group := range []bool{false, true} {
		db := testopen.Postgres(t)
		fsys := fstest.MapFS{
			goVerCreate + "_create_log.up.sql":   {Data: []byte("CREATE TABLE gm_log (id serial PRIMARY KEY, v text);")},
			goVerCreate + "_create_log.down.sql": {Data: []byte("DROP TABLE gm_log;")},
		}
		g := migrate.GoMigration{Version: goVerGo, Name: "backfill", Checksum: "v1", Up: func(ctx context.Context, tx *momo.Tx) error {
			return insertLog(ctx, tx, "go")
		}}
		e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys, migrate.WithGoMigrations(g))
		if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
			t.Fatal(err)
		}
		before, err := readHistory(ctx, t, db)
		if err != nil {
			t.Fatal(err)
		}
		_, err = e.Rollback(ctx, migrate.RollbackOptions{Group: group})
		if !errors.Is(err, migrate.ErrGoMigrationNotReversible) || !strings.Contains(err.Error(), goVerGo) {
			t.Fatalf("group=%v: want ErrGoMigrationNotReversible naming %s, got %v", group, goVerGo, err)
		}
		if !tableExists(t, db, "gm_log") || logValues(t, db) != "go" {
			t.Fatalf("group=%v: nothing may be rolled back", group)
		}
		after, err := readHistory(ctx, t, db)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("group=%v: history changed:\nbefore %+v\nafter  %+v", group, before, after)
		}
	}
}
