// SPDX-License-Identifier: Apache-2.0

//go:build integration

package migrate_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

// excludedInvalidSetup leaves a failed CONCURRENTLY unique index on tmp_x, a
// table Config.Exclude hides, next to the clean users table of the snapshot.
func excludedInvalidSetup(t *testing.T, db *momo.DB) fs.FS {
	t.Helper()
	mustExecDB(t, db, `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)
	mustExecDB(t, db, `CREATE TABLE tmp_x (id INTEGER, v INTEGER)`)
	mustExecDB(t, db, `INSERT INTO tmp_x VALUES (1, 7), (2, 7)`)
	if err := execDB(t, db, `CREATE UNIQUE INDEX CONCURRENTLY tmp_x_v_uq ON tmp_x (v)`); err == nil {
		t.Fatal("expected the concurrent build to fail")
	}
	return migratetestFS(t, initVersion)
}

func TestBaselineIgnoresInvalidIndexOfExcludedTable(t *testing.T) {
	db := testopen.Postgres(t)
	fsys := excludedInvalidSetup(t, db)
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys, migrate.WithConfig(migrate.Config{Exclude: []string{"tmp_*"}}))
	if err := e.Baseline(context.Background(), initVersion, false); err != nil {
		t.Fatalf("an invalid index on an excluded table must not refuse the baseline: %v", err)
	}
}

// Without the exclusion the same database is refused, so the index is the cause.
func TestBaselineInvalidIndexControlWithoutExclude(t *testing.T) {
	db := testopen.Postgres(t)
	fsys := excludedInvalidSetup(t, db)
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
	if err := e.Baseline(context.Background(), initVersion, false); !errors.Is(err, migrate.ErrBaselineMismatch) {
		t.Fatalf("control without Exclude: %v", err)
	}
}

func TestDriftIgnoresInvalidIndexOfExcludedTable(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := excludedInvalidSetup(t, db)
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys, migrate.WithConfig(migrate.Config{Exclude: []string{"tmp_*"}}))
	if err := e.Baseline(ctx, initVersion, true); err != nil {
		t.Fatal(err)
	}
	rep, err := e.Drift(ctx)
	if err != nil || !rep.Clean() {
		t.Fatalf("an invalid index on an excluded table must not be reported: %+v %v", rep, err)
	}
	// An invalid index on a table that is not excluded is still reported.
	mustExecDB(t, db, `INSERT INTO users VALUES (1, 'a@x'), (2, 'a@x')`)
	if err := execDB(t, db, `CREATE UNIQUE INDEX CONCURRENTLY users_email_uq ON users (email)`); err == nil {
		t.Fatal("expected the concurrent build to fail")
	}
	rep, err = e.Drift(ctx)
	if err != nil || len(rep.Changed) != 1 || rep.Changed[0].Object.Name != "users.users_email_uq" {
		t.Fatalf("invalid index of a managed table: %+v %v", rep, err)
	}
}

// An object present in the snapshot only and hidden by Config.Exclude is not
// Missing: the exclusion applies to the snapshot side of drift as well.
func TestDriftAppliesExcludeToSnapshotSide(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	s := usersSchema()
	s.Tables = append(s.Tables, schema.Table{Name: "tmp_scratch", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}}})
	fsys := snapshotFS(t, initVersion, s, "CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT);\n")
	mustExecDB(t, db, `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys, migrate.WithConfig(migrate.Config{Exclude: []string{"tmp_*"}}))
	if err := e.Baseline(ctx, initVersion, true); err != nil {
		t.Fatal(err)
	}
	rep, err := e.Drift(ctx)
	if err != nil || !rep.Clean() {
		t.Fatalf("a snapshot-only excluded table must not be Missing: %+v %v", rep, err)
	}
}

// Baseline is additive: a later version that is already applied is left alone,
// and a missing earlier row is recorded.
func TestBaselineWithLaterVersionAlreadyApplied(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	var s1, s2 bytes.Buffer
	if err := migrate.WriteSnapshot(&s1, migrate.SnapshotFile{Version: "20260101000001", ParentVersion: "20260101000000", Schema: usersSchema()}); err != nil {
		t.Fatal(err)
	}
	if err := migrate.WriteSnapshot(&s2, migrate.SnapshotFile{Version: "20260101000002", ParentVersion: "20260101000001", Schema: usersSchema()}); err != nil {
		t.Fatal(err)
	}
	fsys := migratetest.WithSum(t, fstest.MapFS{
		"20260101000000_a.up.sql":        {Data: []byte("SELECT 0;")},
		"20260101000001_b.up.sql":        {Data: []byte("SELECT 1;")},
		"20260101000001_b.snapshot.json": {Data: s1.Bytes()},
		"20260101000002_c.up.sql":        {Data: []byte("SELECT 2;")},
		"20260101000002_c.snapshot.json": {Data: s2.Bytes()},
	})
	mustExecDB(t, db, `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
	if err := e.Baseline(ctx, "20260101000002", false); err != nil {
		t.Fatal(err)
	}
	mustExecDB(t, db, `DELETE FROM momo_migrations WHERE version = '20260101000000'`)
	if err := e.Baseline(ctx, "20260101000001", false); err != nil {
		t.Fatalf("baseline below an applied version must succeed: %v", err)
	}
	rows, err := readHistory(ctx, t, db)
	if err != nil || len(rows) != 3 {
		t.Fatalf("want rows 0,1,2: %+v %v", rows, err)
	}
	if r := historyRow(t, db, "20260101000000"); r.ApplyID != "baseline" || r.Status != migrate.StatusApplied {
		t.Fatalf("missing earlier row must be re-recorded: %+v", r)
	}
}

// The privilege error reaches the caller of Baseline, with and without force:
// force does not bypass canonicalization, and nothing is recorded.
func TestBaselineSurfacesCanonicalizePrivilegeError(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExecDB(t, db, `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)
	e := migratetest.NewEngine(t, privDialect{mpg.New()}, db.Driver(), migratetestFS(t, initVersion))
	for _, force := range []bool{false, true} {
		err := e.Baseline(ctx, initVersion, force)
		if !errors.Is(err, migrate.ErrCanonicalizePrivilege) {
			t.Fatalf("force=%v: want ErrCanonicalizePrivilege, got %v", force, err)
		}
	}
	rows, err := readHistory(ctx, t, db)
	if err != nil || len(rows) != 0 {
		t.Fatalf("a refused baseline must record nothing: %+v %v", rows, err)
	}
}
