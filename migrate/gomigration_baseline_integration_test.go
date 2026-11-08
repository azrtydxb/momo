// SPDX-License-Identifier: Apache-2.0

//go:build integration

package migrate_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

// goBaselineFS is the init migration plus the snapshot-only Go migration after it.
func goBaselineFS(t *testing.T) fstest.MapFS {
	t.Helper()
	var s1, s2 bytes.Buffer
	if err := migrate.WriteSnapshot(&s1, migrate.SnapshotFile{Version: initVersion, Schema: usersSchema()}); err != nil {
		t.Fatal(err)
	}
	if err := migrate.WriteSnapshot(&s2, migrate.SnapshotFile{Version: goVerGo, ParentVersion: initVersion, Schema: usersSchema()}); err != nil {
		t.Fatal(err)
	}
	return fstest.MapFS{
		initVersion + "_init.up.sql":        {Data: []byte("CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT);")},
		initVersion + "_init.snapshot.json": {Data: s1.Bytes()},
		goVerGo + "_backfill.snapshot.json": {Data: s2.Bytes()},
	}
}

func TestBaselineRecordsRegisteredGoMigrations(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExecDB(t, db, `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)
	runs := 0
	g := migrate.GoMigration{Version: goVerGo, Name: "backfill", Up: func(context.Context, *momo.Tx) error {
		runs++
		return nil
	}}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.WithSum(t, goBaselineFS(t)), migrate.WithGoMigrations(g))
	if err := e.Baseline(ctx, goVerGo, false); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	row := historyRow(t, db, goVerGo)
	// The checksum of a go row is the migration identity: sha256 of "go", the
	// version, the name and the Checksum string, NUL separated.
	idSum := sha256.Sum256([]byte("go\x00" + g.Version + "\x00" + g.Name + "\x00" + g.Checksum))
	if row.Kind != "go" || row.Status != migrate.StatusApplied || row.Checksum != hex.EncodeToString(idSum[:]) || row.ApplyID != migrate.BaselineApplyID {
		t.Fatalf("want an applied baseline go row, got %+v", row)
	}
	if plan, err := e.Plan(ctx); err != nil || len(plan) != 0 {
		t.Fatalf("nothing is pending after the baseline: %+v %v", plan, err)
	}
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil || runs != 0 {
		t.Fatalf("a baselined Go migration must not run: %v, ran %d times", err, runs)
	}
}

func TestBaselineWithoutGoRegistrationRefuses(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExecDB(t, db, `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.WithSum(t, goBaselineFS(t)))
	err := e.Baseline(ctx, goVerGo, false)
	if !errors.Is(err, migrate.ErrGoMigrationUnregistered) || !strings.Contains(err.Error(), goVerGo+"_backfill.snapshot.json") {
		t.Fatalf("want ErrGoMigrationUnregistered naming the snapshot, got %v", err)
	}
	// Nothing was recorded, not even the SQL migration before it.
	rows, err := readHistory(ctx, t, db)
	if err != nil || len(rows) != 0 {
		t.Fatalf("a refused baseline records nothing: %+v %v", rows, err)
	}
	// Baselining before the Go version is fine.
	if err := e.Baseline(ctx, initVersion, false); err != nil {
		t.Fatalf("baseline before the Go version: %v", err)
	}
}

func TestGoMigrationDBTemplateFailurePaths(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	seen := 0
	tmpl := momo.New(db.Driver(), momo.WithHook(countHook{&seen}))

	// Up writes and fails through the template: rolled back, hook fired.
	g := migrate.GoMigration{Version: goVerGo, Name: "backfill", Up: func(ctx context.Context, tx *momo.Tx) error {
		if err := insertLog(ctx, tx, "go"); err != nil {
			return err
		}
		return errors.New("boom")
	}}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), goLogFS(), migrate.WithGoMigrations(g), migrate.WithGoMigrationDB(tmpl))
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil {
		t.Fatal("apply must fail")
	}
	if got := logValues(t, db); got != "sql1" {
		t.Fatalf("the write must roll back, log = %q", got)
	}
	if seen == 0 {
		t.Fatal("the hook must fire for the statement that was rolled back")
	}

	// UpNoTx through the template: the OnConn copy keeps the hook.
	seen = 0
	g2 := migrate.GoMigration{Version: goVerGo, Name: "backfill", UpNoTx: func(ctx context.Context, h *momo.DB) error { return insertLog(ctx, h, "go") }}
	e2 := migratetest.NewEngine(t, mpg.New(), db.Driver(), goLogFS(), migrate.WithGoMigrations(g2), migrate.WithGoMigrationDB(tmpl))
	if _, err := e2.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if seen == 0 || logValues(t, db) != "sql1,go,sql3" {
		t.Fatalf("UpNoTx through the template: hook calls %d, log %q", seen, logValues(t, db))
	}
}
