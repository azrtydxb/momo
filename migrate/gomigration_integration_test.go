// SPDX-License-Identifier: Apache-2.0

//go:build integration

package migrate_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
)

const (
	goVerCreate = "20260101000000"
	goVerGo     = "20260102000000"
	goVerLate   = "20260103000000"
)

// goLogFS is a directory of two SQL migrations that a Go migration sits between.
func goLogFS() fstest.MapFS {
	return fstest.MapFS{
		goVerCreate + "_create_log.up.sql": {Data: []byte("CREATE TABLE gm_log (id serial PRIMARY KEY, v text);\nINSERT INTO gm_log (v) VALUES ('sql1');")},
		goVerLate + "_late.up.sql":         {Data: []byte("INSERT INTO gm_log (v) VALUES ('sql3');")},
	}
}

func logValues(t *testing.T, db *momo.DB) string {
	t.Helper()
	return firstValue(t, db, `SELECT coalesce(string_agg(v, ',' ORDER BY id), '') FROM gm_log`)
}

func insertLog(ctx context.Context, q momo.Querier, v string) error {
	_, err := momo.Exec(ctx, q, `INSERT INTO gm_log (v) VALUES (?)`, v)
	return err
}

func TestGoMigrationOrderedWithSQLRunsOnce(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	runs := 0
	g := migrate.GoMigration{Version: goVerGo, Name: "backfill", Checksum: "v1", Up: func(ctx context.Context, tx *momo.Tx) error {
		runs++
		return insertLog(ctx, tx, "go")
	}}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), goLogFS(), migrate.WithGoMigrations(g))

	plan, err := e.Plan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 3 || plan[0].Version != goVerCreate || plan[1].Version != goVerGo || plan[1].Kind != "go" || plan[2].Version != goVerLate {
		t.Fatalf("plan must order the Go migration between the SQL files, got %+v", plan)
	}
	rep, err := e.Apply(ctx, migrate.ApplyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Applied) != 3 {
		t.Fatalf("want 3 applied, got %+v", rep.Applied)
	}
	if got := logValues(t, db); got != "sql1,go,sql3" {
		t.Fatalf("migrations ran in the wrong order: %s", got)
	}
	row := historyRow(t, db, goVerGo)
	if row.Kind != "go" || row.Status != migrate.StatusApplied || row.Name != "backfill" || row.Checksum == "" || row.ApplyID == "" {
		t.Fatalf("history row of the Go migration: %+v", row)
	}
	if sql := historyRow(t, db, goVerCreate); sql.Kind != "sql" {
		t.Fatalf("SQL row kind = %q", sql.Kind)
	}

	// A second apply, and a second engine, do not run it again.
	if rep, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil || len(rep.Applied) != 0 {
		t.Fatalf("second apply = %+v, %v", rep, err)
	}
	e2 := migratetest.NewEngine(t, mpg.New(), db.Driver(), goLogFS(), migrate.WithGoMigrations(g))
	if rep, err := e2.Apply(ctx, migrate.ApplyOptions{}); err != nil || len(rep.Applied) != 0 {
		t.Fatalf("apply on a second engine = %+v, %v", rep, err)
	}
	if runs != 1 {
		t.Fatalf("the Go migration ran %d times, want 1", runs)
	}
	st, err := e.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(st) != 3 || st[1].Version != goVerGo || st[1].State != "applied" {
		t.Fatalf("status = %+v", st)
	}
}

func TestGoMigrationFailureRollsBackAndRetries(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mode := "error"
	g := migrate.GoMigration{Version: goVerGo, Name: "backfill", Up: func(ctx context.Context, tx *momo.Tx) error {
		if err := insertLog(ctx, tx, "go"); err != nil {
			return err
		}
		switch mode {
		case "error":
			return errors.New("boom")
		case "panic":
			panic("kaboom")
		}
		return nil
	}}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), goLogFS(), migrate.WithGoMigrations(g))

	for _, m := range []string{"error", "panic"} {
		mode = m
		if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil || !strings.Contains(err.Error(), map[string]string{"error": "boom", "panic": "kaboom"}[m]) {
			t.Fatalf("%s: apply must fail with the cause, got %v", m, err)
		}
		if got := logValues(t, db); got != "sql1" {
			t.Fatalf("%s: the insert must roll back with the transaction, log = %q", m, got)
		}
		row := historyRow(t, db, goVerGo)
		if row.Status != migrate.StatusRolledBack || row.Kind != "go" || row.Error == "" {
			t.Fatalf("%s: want a rolled_back go row with the error, got %+v", m, row)
		}
	}
	// rolled_back is not dirty: the fixed migration runs on the next apply.
	mode = "ok"
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := logValues(t, db); got != "sql1,go,sql3" {
		t.Fatalf("log = %q", got)
	}
}

func TestGoMigrationNoTransactionFailureIsDirty(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fail := true
	g := migrate.GoMigration{Version: goVerGo, Name: "index_and_fill", UpNoTx: func(ctx context.Context, h *momo.DB) error {
		// CREATE INDEX CONCURRENTLY is refused inside a transaction, so this
		// only works when the migration really runs outside one.
		if _, err := momo.Exec(ctx, h, `CREATE INDEX CONCURRENTLY gm_log_v ON gm_log (v)`); err != nil {
			return err
		}
		if err := insertLog(ctx, h, "go"); err != nil {
			return err
		}
		if fail {
			return errors.New("half done")
		}
		return nil
	}}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), goLogFS(), migrate.WithGoMigrations(g))

	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil || !strings.Contains(err.Error(), "half done") {
		t.Fatalf("apply must fail with the cause, got %v", err)
	}
	row := historyRow(t, db, goVerGo)
	if row.Status != migrate.StatusFailed || row.Kind != "go" || !strings.Contains(row.Error, "half done") {
		t.Fatalf("want a failed go row, got %+v", row)
	}
	if got := logValues(t, db); got != "sql1,go" {
		t.Fatalf("a no-transaction migration keeps its partial work, log = %q", got)
	}
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); !errors.Is(err, migrate.ErrDirty) {
		t.Fatalf("apply must refuse while dirty, got %v", err)
	}
	st, err := e.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st[1].Version != goVerGo || st[1].State != "dirty" {
		t.Fatalf("status = %+v", st)
	}

	// Resolve as applied: the version is done and the later SQL file runs.
	if err := e.Resolve(ctx, goVerGo, migrate.ResolveApplied); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if row := historyRow(t, db, goVerGo); row.Status != migrate.StatusApplied {
		t.Fatalf("resolved row = %+v", row)
	}
	if got := logValues(t, db); got != "sql1,go,sql3" {
		t.Fatalf("log = %q", got)
	}
}

func TestGoMigrationNoTransactionResolveRolledBackRuns(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fail := true
	runs := 0
	g := migrate.GoMigration{Version: goVerGo, Name: "fill", UpNoTx: func(ctx context.Context, h *momo.DB) error {
		runs++
		if fail {
			return errors.New("not yet")
		}
		return insertLog(ctx, h, "go")
	}}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), goLogFS(), migrate.WithGoMigrations(g))
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil {
		t.Fatal("apply must fail")
	}
	if err := e.Resolve(ctx, goVerGo, migrate.ResolveRolledBack); err != nil {
		t.Fatal(err)
	}
	fail = false
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if runs != 2 {
		t.Fatalf("a migration resolved as rolled_back must run again, ran %d times", runs)
	}
	if got := logValues(t, db); got != "sql1,go,sql3" {
		t.Fatalf("log = %q", got)
	}
}

func TestGoMigrationVersionCollisionAndIdentity(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	noop := func(context.Context, *momo.Tx) error { return nil }

	clash := migrate.GoMigration{Version: goVerCreate, Name: "clash", Up: noop}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), goLogFS(), migrate.WithGoMigrations(clash))
	if _, err := e.Plan(ctx); err == nil || !strings.Contains(err.Error(), goVerCreate) {
		t.Fatalf("a version shared with a SQL file must be an error, got %v", err)
	}
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil {
		t.Fatal("apply must refuse the version collision")
	}

	v1 := migrate.GoMigration{Version: goVerGo, Name: "backfill", Checksum: "v1", Up: noop}
	e = migratetest.NewEngine(t, mpg.New(), db.Driver(), goLogFS(), migrate.WithGoMigrations(v1))
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	v2 := v1
	v2.Checksum = "v2"
	e = migratetest.NewEngine(t, mpg.New(), db.Driver(), goLogFS(), migrate.WithGoMigrations(v2))
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); !errors.Is(err, migrate.ErrChecksumMismatch) {
		t.Fatalf("a changed identity must be ErrChecksumMismatch, got %v", err)
	}
	st, err := e.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st[1].Version != goVerGo || st[1].State != "mismatch" {
		t.Fatalf("status = %+v", st)
	}
}
