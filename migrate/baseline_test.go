// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

const initVersion = "20260101000000"

// usersSchema is the schema after the init migration. The test databases always
// have the vector extension installed, so every snapshot declares it; without
// that, the live extension would be an unmanaged object.
func usersSchema(extra ...schema.Index) *schema.Schema {
	return &schema.Schema{
		Dialect:    "postgres",
		Extensions: []schema.Extension{{Name: "vector"}},
		Tables: []schema.Table{{
			Name:       "users",
			Columns:    []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "email", Type: "TEXT", Nullable: true}},
			PrimaryKey: []string{"id"},
			Indexes:    extra,
		}},
	}
}

// snapshotFS builds a directory with one up file and the snapshot of s for
// version, plus a matching momo.sum.
func snapshotFS(t testing.TB, version string, s *schema.Schema, up string) fs.FS {
	t.Helper()
	var snap bytes.Buffer
	if err := migrate.WriteSnapshot(&snap, migrate.SnapshotFile{Version: version, Schema: s}); err != nil {
		t.Fatal(err)
	}
	return migratetest.WithSum(t, fstest.MapFS{
		version + "_init.up.sql":        {Data: []byte(up)},
		version + "_init.snapshot.json": {Data: snap.Bytes()},
	})
}

// migratetestFS is the init migration of usersSchema with its snapshot.
func migratetestFS(t testing.TB, version string) fs.FS {
	return snapshotFS(t, version, usersSchema(), "CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT);\n")
}

// countDDL returns how many DDL statements ran through the counting driver,
// not counting the engine's own history-table bookkeeping.
func countDDL(t testing.TB, d *migratetest.CountingDriver) int {
	t.Helper()
	return d.DDL()
}

func refNames(refs []migrate.ObjectRef) []string {
	var out []string
	for _, r := range refs {
		out = append(out, r.Kind+":"+r.Name)
	}
	return out
}

func TestBaselineAndDrift(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := migratetestFS(t, initVersion)
	mustExec(t, pinned(t, db), `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)
	cd := migratetest.NewCountingDriver(db.Driver())
	e := migratetest.NewEngine(t, mpg.New(), cd, fsys)
	before := countDDL(t, cd)
	if err := e.Baseline(ctx, initVersion, false); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if countDDL(t, cd)-before != 0 {
		t.Fatal("baseline must execute no DDL")
	}
	mustExec(t, pinned(t, db), `ALTER TABLE users ADD COLUMN extra TEXT`)
	mustExec(t, pinned(t, db), `CREATE TABLE unmanaged_audit (id INTEGER PRIMARY KEY)`)
	rep, err := e.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Changed) == 0 {
		t.Fatal("altered column not reported as changed")
	}
	if len(rep.Unmanaged) != 1 || rep.Unmanaged[0].Name != "unmanaged_audit" {
		t.Fatalf("unmanaged table not listed: %+v", rep.Unmanaged)
	}
	plan, err := e.Plan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 0 {
		t.Fatal("baselined version must not be pending")
	}
}

func TestBaselineWritesHistoryRows(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	var snap bytes.Buffer
	if err := migrate.WriteSnapshot(&snap, migrate.SnapshotFile{Version: "20260101000001", ParentVersion: "20260101000000", Schema: usersSchema()}); err != nil {
		t.Fatal(err)
	}
	fsys := migratetest.WithSum(t, fstest.MapFS{
		"20260101000000_a.up.sql":        {Data: []byte("CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT);")},
		"20260101000001_b.up.sql":        {Data: []byte("SELECT 1;")},
		"20260101000001_b.snapshot.json": {Data: snap.Bytes()},
		"20260101000002_c.up.sql":        {Data: []byte("SELECT 2;")},
	})
	mustExec(t, pinned(t, db), `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
	if err := e.Baseline(ctx, "20260101000001", false); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"20260101000000", "20260101000001"} {
		r := historyRow(t, db, v)
		if r.Status != migrate.StatusApplied || r.StatementsApplied != 0 || r.ApplyID != "baseline" || r.Checksum == "" {
			t.Fatalf("baseline row for %s: %+v", v, r)
		}
	}
	sum, err := migrate.ComputeSum(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if got := historyRow(t, db, "20260101000000").Checksum; got != sum.Files["20260101000000_a.up.sql"] {
		t.Fatalf("checksum %q differs from momo.sum", got)
	}
	plan, err := e.Plan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 1 || plan[0].Version != "20260101000002" {
		t.Fatalf("only the later version must remain pending: %+v", plan)
	}
	// Idempotent: a second baseline changes nothing and does not fail.
	if err := e.Baseline(ctx, "20260101000001", false); err != nil {
		t.Fatalf("second baseline: %v", err)
	}
}

func TestBaselineMismatchAndForce(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := migratetestFS(t, initVersion)
	mustExec(t, pinned(t, db), `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT, extra TEXT)`)
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
	err := e.Baseline(ctx, initVersion, false)
	if !errors.Is(err, migrate.ErrBaselineMismatch) || !strings.Contains(err.Error(), "column") {
		t.Fatalf("want ErrBaselineMismatch naming the object kind, got %v", err)
	}
	if hist, _ := migrate.ReadHistory(ctx, pinned(t, db)); len(hist) != 0 {
		t.Fatalf("a refused baseline must write no rows: %+v", hist)
	}
	if err := e.Baseline(ctx, initVersion, true); err != nil {
		t.Fatalf("forced baseline: %v", err)
	}
	if r := historyRow(t, db, initVersion); r.Status != migrate.StatusApplied {
		t.Fatalf("forced baseline row: %+v", r)
	}
}

func TestBaselineNoSnapshot(t *testing.T) {
	db := testopen.Postgres(t)
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetestFS(t, initVersion))
	if err := e.Baseline(context.Background(), "20260101000009", false); !errors.Is(err, migrate.ErrNoSnapshot) {
		t.Fatalf("want ErrNoSnapshot, got %v", err)
	}
}

func TestBaselineGates(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := migratetestFS(t, initVersion)
	mustExec(t, pinned(t, db), `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)

	// Checksum gate: the directory differs from momo.sum.
	bad := fstest.MapFS{}
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := fs.ReadFile(fsys, p)
			bad[p] = &fstest.MapFile{Data: b}
		}
		return nil
	})
	bad[initVersion+"_init.up.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE users (id int);")}
	eBad, err := migrate.NewEngine(db.Driver(), bad, migrate.WithDialect(mpg.New()))
	if err != nil {
		t.Fatal(err)
	}
	if err := eBad.Baseline(ctx, initVersion, false); !errors.Is(err, migrate.ErrChecksumMismatch) {
		t.Fatalf("want ErrChecksumMismatch, got %v", err)
	}

	// Dirty gate.
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
	if err := migrate.EnsureHistory(ctx, pinned(t, db), mpg.New()); err != nil {
		t.Fatal(err)
	}
	mustExec(t, pinned(t, db), `INSERT INTO momo_migrations (version, name, checksum, status, applied_at) VALUES ('20250101000000', 'x', 'x', 'failed', '2025-01-01T00:00:00Z')`)
	if err := e.Baseline(ctx, initVersion, false); !errors.Is(err, migrate.ErrDirty) {
		t.Fatalf("want ErrDirty, got %v", err)
	}
}

func TestDriftIsReadOnlyAndTreatsEverythingAsUnmanagedWithoutHistory(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExec(t, pinned(t, db), `CREATE TABLE stray (id INTEGER PRIMARY KEY)`)
	cd := migratetest.NewCountingDriver(db.Driver())
	e := migratetest.NewEngine(t, mpg.New(), cd, migratetestFS(t, initVersion))
	rep, err := e.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// No migration applied: the empty schema is the baseline, so the live table
	// and the vector extension are unmanaged and the snapshot objects are not
	// consulted.
	got := strings.Join(refNames(rep.Unmanaged), ",")
	if !strings.Contains(got, "table:stray") || len(rep.Changed) != 0 || len(rep.Missing) != 0 {
		t.Fatalf("drift without history: %+v", rep)
	}
	if tableExists(t, db, "momo_migrations") {
		t.Fatal("Drift must not create the history table")
	}
	if n := cd.Writes(); n != 0 {
		t.Fatalf("Drift must be read-only; it ran %d write statements", n)
	}
}

func TestDriftExcludesHistoryTableAndHonoursExclude(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := migratetestFS(t, initVersion)
	mustExec(t, pinned(t, db), `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys, migrate.WithConfig(migrate.Config{Exclude: []string{"tmp_*", "legacy.*"}}))
	if err := e.Baseline(context.Background(), initVersion, false); err != nil {
		t.Fatal(err)
	}
	mustExec(t, pinned(t, db), `CREATE TABLE tmp_scratch (id INTEGER)`)
	mustExec(t, pinned(t, db), `CREATE TABLE audit_log (id INTEGER)`)
	rep, err := e.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Unmanaged) != 1 || rep.Unmanaged[0].Name != "audit_log" || len(rep.Changed) != 0 || len(rep.Missing) != 0 {
		t.Fatalf("history table must be left out and tmp_* excluded: %+v", rep)
	}
	if rep.Clean() {
		t.Fatal("report with an unmanaged table is not clean")
	}
}

func TestDriftReportsMissingAndChanged(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := migratetestFS(t, initVersion)
	mustExec(t, pinned(t, db), `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
	if err := e.Baseline(ctx, initVersion, false); err != nil {
		t.Fatal(err)
	}
	rep, err := e.Drift(ctx)
	if err != nil || !rep.Clean() {
		t.Fatalf("fresh baseline must be clean: %+v %v", rep, err)
	}
	mustExec(t, pinned(t, db), `ALTER TABLE users ALTER COLUMN email SET NOT NULL`)
	mustExec(t, pinned(t, db), `ALTER TABLE users ALTER COLUMN email TYPE VARCHAR(40)`)
	rep, err = e.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Changed) != 1 || rep.Changed[0].Object != (migrate.ObjectRef{Kind: "column", Name: "users.email"}) ||
		!strings.Contains(rep.Changed[0].Detail, "type") || !strings.Contains(rep.Changed[0].Detail, "nullable") {
		t.Fatalf("changed column must name the differing fields: %+v", rep.Changed)
	}
	mustExec(t, pinned(t, db), `DROP TABLE users`)
	rep, err = e.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Missing) != 1 || rep.Missing[0] != (migrate.ObjectRef{Kind: "table", Name: "users"}) || len(rep.Changed) != 0 {
		t.Fatalf("dropped table must be missing, once: %+v", rep)
	}
}

// invalidIndexSetup leaves an invalid unique index on users and returns a
// directory whose snapshot declares that index.
func invalidIndexSetup(t *testing.T, db *momo.DB) fs.FS {
	t.Helper()
	idx := schema.Index{Name: "users_email_uq", Unique: true, Parts: []schema.IndexPart{{Column: "email"}}}
	mustExec(t, pinned(t, db), `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)
	mustExec(t, pinned(t, db), `INSERT INTO users VALUES (1, 'a@x'), (2, 'a@x')`)
	// A unique CONCURRENTLY build over duplicates fails and leaves an invalid index.
	if _, err := pinned(t, db).Exec(context.Background(), `CREATE UNIQUE INDEX CONCURRENTLY users_email_uq ON users (email)`, nil); err == nil {
		t.Fatal("expected the concurrent build to fail")
	}
	return snapshotFS(t, initVersion, usersSchema(idx), "CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT);\n")
}

func TestBaselineRefusesInvalidIndex(t *testing.T) {
	db := testopen.Postgres(t)
	fsys := invalidIndexSetup(t, db)
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
	err := e.Baseline(context.Background(), initVersion, false)
	if !errors.Is(err, migrate.ErrBaselineMismatch) || !strings.Contains(err.Error(), "index") {
		t.Fatalf("an invalid index must refuse the baseline: %v", err)
	}
}

func TestDriftReportsInvalidIndex(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := invalidIndexSetup(t, db)
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
	if err := e.Baseline(ctx, initVersion, true); err != nil {
		t.Fatal(err)
	}
	rep, err := e.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := migrate.ObjectDiff{Object: migrate.ObjectRef{Kind: "index", Name: "users.users_email_uq"}, Detail: "invalid index (failed CONCURRENTLY build)"}
	if len(rep.Changed) != 1 || rep.Changed[0] != want || len(rep.Missing) != 0 || len(rep.Unmanaged) != 0 {
		t.Fatalf("invalid index must be reported as changed: %+v", rep)
	}
}
