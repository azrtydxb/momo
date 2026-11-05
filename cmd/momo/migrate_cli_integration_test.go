// SPDX-License-Identifier: Apache-2.0

//go:build integration

package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	testdb "github.com/azrtydxb/momo/internal/testdb"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
)

// TestMigrateDiffRefusesOverwrite checks that diff never replaces a migration
// file. The clock is fixed and the exact target path is pre-created, so diff
// always meets the obstacle: the test cannot return early on a version change.
// The obstacle is a directory of the target's name; the migration walk ignores
// directories, so the engine still chooses that very version.
func TestMigrateDiffRefusesOverwrite(t *testing.T) {
	dsn := testdb.PostgresURL(t)
	root := copyModule(t, exampleSrc)
	writeFile(t, filepath.Join(root, "momo.yaml"), "version: 1\nmigrate:\n  dir:\n    postgres: m\n")
	chdir(t, root)
	fixed := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	old := migrateClock
	migrateClock = func() time.Time { return fixed }
	t.Cleanup(func() { migrateClock = old })

	target := filepath.Join(root, "m", "20260304050607_init.up.sql")
	mkdirAll(t, target)
	code, out, errOut := cliRun(t, dsn, "diff", "init")
	if code != 1 || !strings.Contains(errOut, "overwrite") || !strings.Contains(errOut, "20260304050607_init.up.sql") {
		t.Fatalf("exit %d, out %q, err %q; want 1 naming the target", code, out, errOut)
	}
	entries, err := os.ReadDir(filepath.Join(root, "m"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "20260304050607_init.up.sql" {
		t.Fatalf("a refused diff left files behind: %v", entries)
	}
}

// startedProject runs diff init and returns the database URL and project root.
func startedProject(t *testing.T) (dsn, root string) {
	t.Helper()
	dsn = testdb.PostgresURL(t)
	root = simpleModule(t)
	writeFile(t, filepath.Join(root, "momo.yaml"),
		"version: 1\nmigrate:\n  dir:\n    postgres: migrations/postgres\n  url_env:\n    postgres: MOMO_TEST_URL\n  exclude: [vector]\n")
	chdir(t, root)
	if code, _, errOut := cliRun(t, dsn, "diff", "init"); code != 0 {
		t.Fatalf("diff: exit %d: %s", code, errOut)
	}
	return dsn, root
}

// holdMigrationLock takes the PostgreSQL advisory lock apply needs, from a
// separate connection, until release is called.
func holdMigrationLock(t *testing.T, dsn string) (release func()) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxdriver.NewPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	pc, err := pgxdriver.New(pool).(momo.Pinner).Pin(ctx)
	if err != nil {
		pool.Close()
		t.Fatal(err)
	}
	unlock, _, err := mpg.New().Lock(ctx, pc, "momo:"+testdb.Schema(t)+".momo_migrations")
	if err != nil {
		_ = pc.Release()
		pool.Close()
		t.Fatal(err)
	}
	var once sync.Once
	release = func() {
		once.Do(func() {
			if err := unlock(); err != nil {
				t.Error(err)
			}
			_ = pc.Release()
			pool.Close()
		})
	}
	t.Cleanup(release)
	return release
}

// TestMigrateApplyLockWait covers --lock-wait and --force-unlock against a
// lock held by another connection.
func TestMigrateApplyLockWait(t *testing.T) {
	t.Run("times out with exit 3 naming the holder", func(t *testing.T) {
		dsn, _ := startedProject(t)
		holdMigrationLock(t, dsn)
		start := time.Now()
		code, _, errOut := cliRun(t, dsn, "apply", "--lock-wait", "600ms")
		if code != 3 || !strings.Contains(errOut, "held by") {
			t.Fatalf("exit %d, want 3 naming the holder: %s", code, errOut)
		}
		// 600ms asked for: far below the 30s default, so a flag that is
		// ignored makes this test take the default and fail the upper bound.
		if took := time.Since(start); took < 500*time.Millisecond || took > 15*time.Second {
			t.Fatalf("gave up after %v; --lock-wait 600ms must be honoured", took)
		}
	})
	t.Run("succeeds once the lock is released", func(t *testing.T) {
		dsn, _ := startedProject(t)
		release := holdMigrationLock(t, dsn)
		go func() {
			time.Sleep(700 * time.Millisecond)
			release()
		}()
		code, out, errOut := cliRun(t, dsn, "apply", "--lock-wait", "20s")
		if code != 0 || !strings.Contains(out, "applied") {
			t.Fatalf("exit %d, out %q, err %q", code, out, errOut)
		}
	})
	t.Run("force-unlock cannot take a live advisory lock", func(t *testing.T) {
		dsn, _ := startedProject(t)
		holdMigrationLock(t, dsn)
		code, _, errOut := cliRun(t, dsn, "apply", "--force-unlock", "--lock-wait", "600ms")
		if code != 3 || !strings.Contains(errOut, "held by") {
			t.Fatalf("exit %d, want 3: %s", code, errOut)
		}
	})
}

// TestMigrateStatusExitsOneOnDirty pins that status reports a dirty history row
// as a finding (exit 1) and prints it.
func TestMigrateStatusExitsOneOnDirty(t *testing.T) {
	dsn, root := startedProject(t)
	if code, _, errOut := cliRun(t, dsn, "apply"); code != 0 {
		t.Fatalf("apply: exit %d: %s", code, errOut)
	}
	if code, _, _ := cliRun(t, dsn, "status"); code != 0 {
		t.Fatalf("clean status: exit %d, want 0", code)
	}
	files, _ := filepath.Glob(filepath.Join(root, "migrations", "postgres", "*_init.up.sql"))
	if len(files) != 1 {
		t.Fatalf("glob: %v", files)
	}
	version := strings.SplitN(filepath.Base(files[0]), "_", 2)[0]
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if _, err := sqlDB.ExecContext(context.Background(), `UPDATE momo_migrations SET status = 'failed', error = 'boom' WHERE version = $1`, version); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := cliRun(t, dsn, "status")
	if code != 1 || !strings.Contains(out, "dirty") {
		t.Fatalf("dirty status: exit %d, out %q, err %q; want 1 listing the dirty row", code, out, errOut)
	}
}
