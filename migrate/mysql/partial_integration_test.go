// SPDX-License-Identifier: Apache-2.0

//go:build integration

package mysql_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
)

// appliedStatements reads statements_applied for a version from the history
// table through the driver.
func appliedStatements(t *testing.T, db *momo.DB, version string) int64 {
	t.Helper()
	rows, err := db.Driver().Query(context.Background(), "SELECT statements_applied FROM momo_migrations WHERE version = ?", []any{version})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Error(err)
		}
	}()
	if !rows.Next() {
		t.Fatalf("no history row for %s: %v", version, rows.Err())
	}
	var n int64
	if err := rows.Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func mustExec(t *testing.T, q momo.Querier, sql string) {
	t.Helper()
	if _, err := momo.Exec(context.Background(), q, sql); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestMySQLPartialFailure: DDL commits implicitly, so a failing migration
// keeps the statements that ran before it. The engine must run it statement by
// statement and record how far it got, never report it as rolled back.
func TestMySQLPartialFailure(t *testing.T) {
	ctx := context.Background()
	db := testopen.MySQL84(t)
	dir := t.TempDir()
	const version = "20260101000000"
	writeFile(t, dir, version+"_partial.up.sql",
		"CREATE TABLE a (id bigint PRIMARY KEY);\nCREATE TABLE b (id bigint PRIMARY KEY);\nINSERT INTO missing_table VALUES (1);\n")
	eng := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(dir))
	if _, err := eng.Apply(ctx, migrate.ApplyOptions{}); err == nil {
		t.Fatal("apply of a failing migration succeeded")
	}
	if _, err := momo.Exec(ctx, db, "SELECT 1 FROM a LIMIT 1"); err != nil {
		t.Fatalf("table a is missing; a non-transactional migration keeps earlier DDL: %v", err)
	}
	states, err := eng.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var state string
	for _, st := range states {
		if st.Version == version {
			state = string(st.State)
		}
	}
	if state != "dirty" {
		t.Fatalf("status of %s = %q, want dirty (a transactional run would say rolled_back)", version, state)
	}
	if got := appliedStatements(t, db, version); got != 2 {
		t.Fatalf("statements_applied = %d, want 2", got)
	}
	if _, err := eng.Apply(ctx, migrate.ApplyOptions{}); !errors.Is(err, migrate.ErrDirty) {
		t.Fatalf("second apply: err = %v, want migrate.ErrDirty", err)
	}
	if err := eng.Resolve(ctx, version, migrate.ResolveRolledBack); err != nil {
		t.Fatal(err)
	}
}

// TestMySQLFailingDown: a down migration commits statement by statement too. A
// failure leaves the earlier statements applied and the row failed (dirty),
// with the count of statements that ran; it is never reported as rolled back.
func TestMySQLFailingDown(t *testing.T) {
	ctx := context.Background()
	db := testopen.MySQL84(t)
	dir := t.TempDir()
	const version = "20260101000000"
	writeFile(t, dir, version+"_tables.up.sql", "CREATE TABLE r1 (id int);\nCREATE TABLE r2 (id int);\n")
	writeFile(t, dir, version+"_tables.down.sql", "DROP TABLE r1;\nDROP TABLE no_such_table;\nDROP TABLE r2;\n")
	eng := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(dir))
	if _, err := eng.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Rollback(ctx, migrate.RollbackOptions{}); err == nil {
		t.Fatal("a failing down migration succeeded")
	}
	if _, err := momo.Exec(ctx, db, "SELECT 1 FROM r1 LIMIT 1"); err == nil {
		t.Fatal("table r1 still exists; the first down statement was not committed")
	}
	if _, err := momo.Exec(ctx, db, "SELECT 1 FROM r2 LIMIT 1"); err != nil {
		t.Fatalf("table r2 is gone, but the down stopped before dropping it: %v", err)
	}
	states, err := eng.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var state string
	for _, st := range states {
		if st.Version == version {
			state = string(st.State)
		}
	}
	if state != "dirty" {
		t.Fatalf("status of %s = %q, want dirty", version, state)
	}
	if got := appliedStatements(t, db, version); got != 1 {
		t.Fatalf("statements_applied = %d, want 1", got)
	}
}

// TestMySQLRollbackMarksRunning: a down migration commits statement by
// statement, so the history row must leave applied before the first down
// statement runs. The first down statement records the status it sees; the
// second fails. A rollback killed half-way would otherwise leave an applied row
// over a half-reverted schema.
func TestMySQLRollbackMarksRunning(t *testing.T) {
	ctx := context.Background()
	db := testopen.MySQL84(t)
	dir := t.TempDir()
	const version = "20260101000000"
	writeFile(t, dir, version+"_seen.up.sql", "CREATE TABLE seen (status varchar(32));\n")
	writeFile(t, dir, version+"_seen.down.sql",
		"INSERT INTO seen SELECT status FROM momo_migrations WHERE version = '"+version+"';\nDROP TABLE no_such_table;\n")
	eng := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(dir))
	if _, err := eng.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Rollback(ctx, migrate.RollbackOptions{}); err == nil {
		t.Fatal("a failing down migration succeeded")
	}
	rows, err := db.Driver().Query(ctx, "SELECT status FROM seen", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatalf("the first down statement did not run: %v", rows.Err())
	}
	var seen string
	if err := rows.Scan(&seen); err != nil {
		t.Fatal(err)
	}
	if seen != migrate.StatusRunning {
		t.Fatalf("the row was %q while the first down statement ran, want %q", seen, migrate.StatusRunning)
	}
	// Failed rollback is dirty: Apply refuses, Status reports dirty.
	if _, err := eng.Apply(ctx, migrate.ApplyOptions{}); !errors.Is(err, migrate.ErrDirty) {
		t.Fatalf("apply after a failed rollback: err = %v, want migrate.ErrDirty", err)
	}
}
