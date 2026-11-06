// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo"

	testdb "github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
)

func TestNonTransactionalFailureRecovery(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := migratetest.WithSum(t, fstest.MapFS{
		"20260101000000_bad.up.sql": {Data: []byte("-- momo:no-transaction\nCREATE TABLE ok1 (x int);\nCREATE TABLE ok2 (x int);\nINSERT INTO missing VALUES (1);")},
	})
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil {
		t.Fatal("apply must fail")
	}
	row := historyRow(t, db, "20260101000000")
	if row.Status != migrate.StatusFailed || row.StatementsApplied != 2 || row.Error == "" {
		t.Fatalf("want failed with statements_applied=2, got %+v", row)
	}
	if row.Checksum == "" || row.Name != "bad" || row.Kind != "sql" || row.ApplyID == "" {
		t.Fatalf("history row lacks checksum, name, kind or apply id: %+v", row)
	}
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); !errors.Is(err, migrate.ErrDirty) {
		t.Fatalf("apply must refuse while dirty, got %v", err)
	}
	if err := e.Resolve(ctx, "20260101000000", migrate.ResolveRolledBack); err != nil {
		t.Fatal(err)
	}
	plan, err := e.Plan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 1 {
		t.Fatalf("after resolve the version is pending again, plan = %+v", plan)
	}
}

func TestResolveApplied(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := migratetest.WithSum(t, fstest.MapFS{
		"20260101000000_bad.up.sql": {Data: []byte("-- momo:no-transaction\nCREATE TABLE ok1 (x int);\nINSERT INTO missing VALUES (1);")},
	})
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil {
		t.Fatal("apply must fail")
	}
	if err := e.Resolve(ctx, "20260101000000", migrate.ResolveApplied); err != nil {
		t.Fatal(err)
	}
	row := historyRow(t, db, "20260101000000")
	if row.Status != migrate.StatusApplied || row.StatementsApplied != 1 {
		t.Fatalf("want applied keeping statements_applied=1, got %+v", row)
	}
	if err := e.Resolve(ctx, "20260101000000", migrate.ResolveApplied); err == nil {
		t.Fatal("resolving a version without a dirty row must fail")
	}
	if plan, err := e.Plan(ctx); err != nil || len(plan) != 0 {
		t.Fatalf("nothing pending after resolve applied: %+v %v", plan, err)
	}
}

func TestTransactionalFailureRolledBack(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	bad := migratetest.WithSum(t, fstest.MapFS{
		"20260101000000_t.up.sql": {Data: []byte("CREATE TABLE a (x int);\nINSERT INTO missing VALUES (1);")},
	})
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), bad)
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil {
		t.Fatal("apply must fail")
	}
	if tableExists(t, db, "a") {
		t.Fatal("transactional failure must roll back the DDL")
	}
	row := historyRow(t, db, "20260101000000")
	if row.Status != migrate.StatusRolledBack || row.Error == "" {
		t.Fatalf("want rolled_back with the error text, got %+v", row)
	}
	fixed := migratetest.WithSum(t, fstest.MapFS{
		"20260101000000_t.up.sql": {Data: []byte("CREATE TABLE a (x int);")},
	})
	e2 := migratetest.NewEngine(t, mpg.New(), db.Driver(), fixed)
	if _, err := e2.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatalf("a rolled-back transactional migration must apply again without resolve: %v", err)
	}
	if !tableExists(t, db, "a") {
		t.Fatal("the re-applied migration did not create a")
	}
	row = historyRow(t, db, "20260101000000")
	if row.Status != migrate.StatusApplied || row.StatementsApplied != 1 || row.Checksum == "" || row.Error != "" {
		t.Fatalf("re-applied row wrong: %+v", row)
	}
}

func TestNewEngineRequiresDialect(t *testing.T) {
	_, err := migrate.NewEngine(nil, fstest.MapFS{})
	if err == nil || !strings.Contains(err.Error(), "migrate.WithDialect") {
		t.Fatalf("want an error naming migrate.WithDialect, got %v", err)
	}
}

func runEnv(t *testing.T, c momo.PinnedConn) migrate.RunEnv {
	t.Helper()
	return migrate.RunEnv{Conn: c, Dialect: mpg.New(), Clock: time.Now}
}

// Without the migration lock, a second engine's failure path must not turn the
// first engine's applied row back into a pending version.
func TestFailurePathNeverOverwritesApplied(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := migratetest.WithSum(t, fstest.MapFS{
		"20260101000000_t.up.sql": {Data: []byte("CREATE TABLE a (x int);")},
	})
	if _, err := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys).Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	c := pinned(t, db)
	bad := migrate.Pending{Version: "20260101000000", Name: "t", Statements: []migrate.Statement{{SQL: "CREATE TABLE b (x int)"}, {SQL: "INSERT INTO missing VALUES (1)"}}}
	if _, err := migrate.RunMigration(ctx, runEnv(t, c), bad, "engine-b"); err == nil {
		t.Fatal("engine B must fail")
	}
	row := historyRow(t, db, "20260101000000")
	if row.Status != migrate.StatusApplied || row.Error != "" || row.ApplyID == "engine-b" {
		t.Fatalf("engine A's applied row was overwritten: %+v", row)
	}
	if tableExists(t, db, "b") {
		t.Fatal("engine B's DDL must be rolled back")
	}
}

func TestCancelledContextStillRecordsFailure(t *testing.T) {
	for _, tc := range []struct{ name, prefix, status string }{
		{"transactional", "", migrate.StatusRolledBack},
		{"non-transactional", "-- momo:no-transaction\n", migrate.StatusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testopen.Postgres(t)
			marker := "momo-cancel-" + testdb.Schema(t)
			body := tc.prefix + "SELECT pg_sleep(10) /* " + marker + " */;"
			fsys := migratetest.WithSum(t, fstest.MapFS{"20260101000000_slow.up.sql": {Data: []byte(body)}})
			e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
			ctx, cancel := context.WithCancel(context.Background())
			cancelWhenRunning(t, db, marker, cancel) // cancel once the slow statement runs, not after a fixed delay
			if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil {
				t.Fatal("apply must fail when its context is cancelled")
			}
			row := historyRow(t, db, "20260101000000")
			if row.Status != tc.status || row.Error == "" {
				t.Fatalf("want %s with the error recorded after cancellation, got %+v", tc.status, row)
			}
		})
	}
}

func TestClass22ErrorIsRedacted(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := migratetest.WithSum(t, fstest.MapFS{
		"20260101000000_t.up.sql": {Data: []byte("CREATE TABLE t (intcol int);\nINSERT INTO t(intcol) VALUES ('secret-abc');")},
	})
	_, err := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys).Apply(ctx, migrate.ApplyOptions{})
	if err == nil {
		t.Fatal("apply must fail")
	}
	row := historyRow(t, db, "20260101000000")
	if strings.Contains(row.Error, "secret-abc") || strings.Contains(err.Error(), "secret-abc") {
		t.Fatalf("data leaked: recorded %q, returned %q", row.Error, err)
	}
	if !strings.Contains(row.Error, "22P02") {
		t.Fatalf("the SQLSTATE should remain: %q", row.Error)
	}
}

func TestLeftoverRunningRowIsDirty(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	c := pinned(t, db)
	if err := migrate.EnsureHistory(ctx, c, mpg.New()); err != nil {
		t.Fatal(err)
	}
	mustExec(t, c, `INSERT INTO momo_migrations (version, name, checksum, status, applied_at) VALUES ('20250101000000', 'old', 'x', 'running', 'now')`)
	fsys := migratetest.WithSum(t, fstest.MapFS{"20260101000000_t.up.sql": {Data: []byte("CREATE TABLE a (x int);")}})
	_, err := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys).Apply(ctx, migrate.ApplyOptions{})
	if !errors.Is(err, migrate.ErrDirty) {
		t.Fatalf("want ErrDirty, got %v", err)
	}
	if tableExists(t, db, "a") {
		t.Fatal("nothing may run while dirty")
	}
}

func TestMixedNoTxStatementsApplied(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	c := pinned(t, db)
	if err := migrate.EnsureHistory(ctx, c, mpg.New()); err != nil {
		t.Fatal(err)
	}
	p := migrate.Pending{Version: "20260101000000", Name: "mixed", Statements: []migrate.Statement{
		{SQL: "CREATE TABLE m1 (x int)"},
		{SQL: "CREATE TABLE m2 (x int)", NoTx: true},
		{SQL: "INSERT INTO missing VALUES (1)"},
		{SQL: "CREATE TABLE m3 (x int)"},
	}}
	if _, err := migrate.RunMigration(ctx, runEnv(t, c), p, "a1"); err == nil {
		t.Fatal("must fail")
	}
	row := historyRow(t, db, "20260101000000")
	if row.Status != migrate.StatusFailed || row.StatementsApplied != 2 {
		t.Fatalf("want failed with 2 statements applied, got %+v", row)
	}
	if !tableExists(t, db, "m1") || !tableExists(t, db, "m2") || tableExists(t, db, "m3") {
		t.Fatal("statements before the failure stay applied; later ones do not run")
	}
}

func TestRolledBackRowIsRetried(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	bad := migratetest.WithSum(t, fstest.MapFS{"20260101000000_t.up.sql": {Data: []byte("INSERT INTO missing VALUES (1);")}})
	if _, err := migratetest.NewEngine(t, mpg.New(), db.Driver(), bad).Apply(ctx, migrate.ApplyOptions{}); err == nil {
		t.Fatal("must fail")
	}
	if r := historyRow(t, db, "20260101000000"); r.Status != migrate.StatusRolledBack {
		t.Fatalf("want rolled_back, got %+v", r)
	}
	fixed := migratetest.WithSum(t, fstest.MapFS{"20260101000000_t.up.sql": {Data: []byte("-- momo:no-transaction\nCREATE TABLE a (x int);")}})
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fixed)
	if plan, err := e.Plan(ctx); err != nil || len(plan) != 1 {
		t.Fatalf("a rolled_back version is pending: %+v %v", plan, err)
	}
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if r := historyRow(t, db, "20260101000000"); r.Status != migrate.StatusApplied || r.Error != "" {
		t.Fatalf("retry must end applied with no error, got %+v", r)
	}
}
