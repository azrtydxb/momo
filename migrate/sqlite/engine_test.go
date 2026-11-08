// SPDX-License-Identifier: Apache-2.0

//go:build integration

package sqlite_test

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
	"github.com/azrtydxb/momo/schema"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

func at(ts string) migrate.Option {
	tm, err := time.Parse("20060102150405", ts)
	if err != nil {
		panic(err) // test-only constant input
	}
	return migrate.WithClock(func() time.Time { return tm })
}

func mergeFiles(parts ...map[string][]byte) map[string][]byte {
	out := map[string][]byte{}
	for _, p := range parts {
		for k, v := range p {
			out[k] = v
		}
	}
	return out
}

func upFile(t *testing.T, files map[string][]byte) string {
	t.Helper()
	for name, body := range files {
		if strings.HasSuffix(name, ".up.sql") {
			return string(body)
		}
	}
	t.Fatal("no up file")
	return ""
}

func newEngine(t *testing.T, db *momo.DB, dir fs.FS, ts string) *migrate.Engine {
	t.Helper()
	return migratetest.NewEngine(t, msqlite.New(), db.Driver(), dir, at(ts))
}

// TestSQLiteEngineLifecycle runs the whole path through the migration engine on
// SQLite: Diff writes the up file and snapshot, Apply creates the history table
// and the lock table and runs the file, a second Diff that needs a rebuild
// writes the foreign-keys-off directive and the twelve steps, Apply runs it with
// enforcement off and restores it, and Drift finds nothing.
func TestSQLiteEngineLifecycle(t *testing.T) {
	db := testopen.SQLite(t)
	ctx := context.Background()
	v1, v2 := roundTripSchema(false), roundTripSchema(true)

	m1, err := newEngine(t, db, migratetest.EmptyFS(), "20260101000000").Diff(ctx, v1, "init", migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if m1 == nil {
		t.Fatal("expected an initial migration")
	}
	if strings.Contains(upFile(t, m1.Files), "foreign-keys-off") {
		t.Fatal("a plan without a rebuild must not turn foreign keys off")
	}
	e2 := newEngine(t, db, migratetest.FS(m1.Files), "20260102000000")
	rep, err := e2.Apply(ctx, migrate.ApplyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Applied) != 1 {
		t.Fatalf("applied %+v", rep.Applied)
	}
	if again, err := e2.Apply(ctx, migrate.ApplyOptions{}); err != nil || len(again.Applied) != 0 {
		t.Fatalf("a second Apply must find nothing to do: %+v %v", again, err)
	}
	drift, err := e2.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !drift.Clean() {
		t.Fatalf("drift after the first apply: %+v", drift)
	}

	mustExec(t, db.Driver(), `INSERT INTO tenants (id, name) VALUES (1, 'acme')`)
	mustExec(t, db.Driver(), `INSERT INTO docs (title, status, tenant_id, score) VALUES ('One', 'a', 1, 1.5), ('Two', 'b', 1, 2.5)`)

	m2, err := e2.Diff(ctx, v2, "widen", migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if m2 == nil {
		t.Fatal("expected a second migration")
	}
	up := upFile(t, m2.Files)
	for _, want := range []string{"-- momo:sqlite-foreign-keys-off", `CREATE TABLE "new_docs"`, "PRAGMA foreign_key_check", `ALTER TABLE "new_docs" RENAME TO "docs"`} {
		if !strings.Contains(up, want) {
			t.Fatalf("the rebuild migration must contain %q:\n%s", want, up)
		}
	}

	e3 := newEngine(t, db, migratetest.FS(mergeFiles(m1.Files, m2.Files)), "20260103000000")
	rep, err = e3.Apply(ctx, migrate.ApplyOptions{})
	if err != nil {
		t.Fatalf("apply of the rebuild: %v\n%s", err, up)
	}
	if len(rep.Applied) != 1 {
		t.Fatalf("applied %+v", rep.Applied)
	}
	c := pinned(t, db)
	var n, fk int
	scanOne(t, c, `SELECT count(*) FROM docs`, &n)
	scanOne(t, c, `PRAGMA foreign_keys`, &fk)
	if n != 2 || fk != 1 {
		t.Fatalf("rows = %d (want 2), foreign_keys = %d (want 1: Apply must restore it)", n, fk)
	}
	for _, o := range [][2]string{{"index", "docs_title"}, {"trigger", "docs_touch"}, {"view", "open_docs"}, {"table", "momo_migrations"}} {
		var count int
		scanOne(t, c, `SELECT count(*) FROM sqlite_schema WHERE type='`+o[0]+`' AND name='`+o[1]+`'`, &count)
		if count != 1 {
			t.Errorf("%s %s missing", o[0], o[1])
		}
	}
	var lockRows int
	scanOne(t, c, `SELECT count(*) FROM momo_migration_lock`, &lockRows)
	if lockRows != 0 {
		t.Errorf("the migration lock must be released, %d row(s) left", lockRows)
	}
	release(t, c)

	drift, err = e3.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !drift.Clean() {
		t.Fatalf("drift after the rebuild: %+v", drift)
	}
	statuses, err := e3.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range statuses {
		if s.State != "applied" {
			t.Errorf("version %s is %s", s.Version, s.State)
		}
	}
	live, err := e3.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !schema.Equal(live, v2) {
		t.Fatalf("the migrated database is not v2\n%s", migrateDiff(live, v2))
	}
}

// TestSQLiteEngineRebuildOrphanFailsAndRestores applies a rebuild to a database
// that holds an orphan row. foreign_key_check must fail the migration, the
// transaction must roll back, and foreign keys must be on again afterwards.
func TestSQLiteEngineRebuildOrphanFailsAndRestores(t *testing.T) {
	db := testopen.SQLite(t)
	ctx := context.Background()
	v1, v2 := roundTripSchema(false), roundTripSchema(false)
	v2.Tables[1].Columns[1].Default = "'changed'"

	m1, err := newEngine(t, db, migratetest.EmptyFS(), "20260101000000").Diff(ctx, v1, "init", migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newEngine(t, db, migratetest.FS(m1.Files), "20260102000000").Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	c := pinned(t, db)
	mustExec(t, c, `PRAGMA foreign_keys = OFF`)
	mustExec(t, c, `INSERT INTO docs (title, tenant_id) VALUES ('orphan', 404)`)
	mustExec(t, c, `PRAGMA foreign_keys = ON`)
	release(t, c)

	e := newEngine(t, db, migratetest.FS(m1.Files), "20260103000000")
	m2, err := e.Diff(ctx, v2, "default", migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	e2 := newEngine(t, db, migratetest.FS(mergeFiles(m1.Files, m2.Files)), "20260104000000")
	_, err = e2.Apply(ctx, migrate.ApplyOptions{})
	if err == nil || !strings.Contains(err.Error(), "foreign_key_check") {
		t.Fatalf("want a foreign_key_check failure, got %v", err)
	}
	if !errors.Is(err, momo.ErrForeignKeyViolation) {
		t.Fatalf("the failure must be a momo.ErrForeignKeyViolation, got %v", err)
	}

	c = pinned(t, db)
	var fk int
	scanOne(t, c, `PRAGMA foreign_keys`, &fk)
	if fk != 1 {
		t.Fatalf("foreign keys must be restored after a failed rebuild, got %d", fk)
	}
	var def string
	scanOne(t, c, `SELECT dflt_value FROM pragma_table_info('docs') WHERE name = 'title'`, &def)
	if def != "''" {
		t.Fatalf("the failed rebuild must roll back, title default = %q", def)
	}
	var lockRows int
	scanOne(t, c, `SELECT count(*) FROM momo_migration_lock`, &lockRows)
	if lockRows != 0 {
		t.Fatalf("the lock must be released after a failure, %d row(s) left", lockRows)
	}
}
