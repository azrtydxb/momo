// SPDX-License-Identifier: Apache-2.0

//go:build integration && cgo && !windows

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

// TestSQLiteAutoMigrateSkipsRebuilds checks that AutoMigrate leaves a table
// alone when an additive change to it would need a rebuild. A rebuild writes
// the whole desired table, so it would also drop the column the model no
// longer has and apply the other changes AutoMigrate skips.
func TestSQLiteAutoMigrateSkipsRebuilds(t *testing.T) {
	db := testopen.SQLite(t)
	ctx := context.Background()
	mustExec(t, db.Driver(), `CREATE TABLE items (id INTEGER NOT NULL PRIMARY KEY, name TEXT, legacy TEXT)`)
	mustExec(t, db.Driver(), `INSERT INTO items (name, legacy) VALUES ('a', 'keep'), ('b', 'me')`)

	// The model drops legacy and adds the enum column status.
	desired := &schema.Schema{
		Dialect: "sqlite",
		Enums:   []schema.Enum{{Name: "items_status", Values: []string{"new", "done"}}},
		Tables: []schema.Table{{
			Name: "items",
			Columns: []schema.Column{
				{Name: "id", Type: "INTEGER"},
				{Name: "name", Type: "TEXT", Nullable: true},
				{Name: "status", Type: "TEXT", Nullable: true, Enum: "items_status"},
			},
			PrimaryKey: []string{"id"},
		}},
	}
	rep, err := newEngine(t, db, migratetest.EmptyFS(), "20260101000000").AutoMigrate(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	var skipped bool
	for _, sk := range rep.Skipped {
		if sk.Change.Kind == migrate.AddColumn && sk.Change.Name == "status" {
			skipped = true
			if sk.Reason != "requires a table rebuild; use a migration" {
				t.Errorf("reason = %q", sk.Reason)
			}
		}
	}
	if !skipped {
		t.Fatalf("the enum column must be reported as skipped: %+v", rep)
	}
	for _, a := range rep.Applied {
		if a.Table == "items" {
			t.Errorf("no change to items may be applied: %+v", a)
		}
	}

	c := pinned(t, db)
	var legacy string
	scanOne(t, c, `SELECT legacy FROM items WHERE name = 'a'`, &legacy)
	if legacy != "keep" {
		t.Fatalf("legacy data lost: %q", legacy)
	}
	var n int
	scanOne(t, c, `SELECT count(*) FROM pragma_table_info('items') WHERE name = 'status'`, &n)
	if n != 0 {
		t.Fatal("status must not have been added by a rebuild")
	}
}

// TestSQLiteRollbackRebuildKeepsChildRows runs a hand-written down migration
// that rebuilds a parent table with child rows. Rollback must honour the
// foreign-keys-off directive as Apply does: with enforcement on, DROP TABLE of
// the parent fails on the child rows.
func TestSQLiteRollbackRebuildKeepsChildRows(t *testing.T) {
	db := testopen.SQLite(t)
	ctx := context.Background()
	v1 := &schema.Schema{Dialect: "sqlite", Tables: []schema.Table{
		{Name: "parents", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "note", Type: "TEXT", Nullable: true}}, PrimaryKey: []string{"id"}},
		{
			Name:       "children",
			Columns:    []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "parent_id", Type: "INTEGER"}},
			PrimaryKey: []string{"id"},
			ForeignKeys: []schema.ForeignKey{
				{Name: "children_parent_id_fkey", Columns: []string{"parent_id"}, RefTable: "parents", RefColumns: []string{"id"}},
			},
		},
	}}
	m1, err := newEngine(t, db, migratetest.EmptyFS(), "20260101000000").Diff(ctx, v1, "init", migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	files := mergeFiles(m1.Files)
	for name := range m1.Files {
		if up, ok := strings.CutSuffix(name, ".up.sql"); ok {
			files[up+".down.sql"] = []byte(`-- momo:sqlite-foreign-keys-off
CREATE TABLE "new_parents" ("id" INTEGER NOT NULL, PRIMARY KEY ("id"));
INSERT INTO "new_parents" ("id") SELECT "id" FROM "parents";
DROP TABLE "parents";
ALTER TABLE "new_parents" RENAME TO "parents";
PRAGMA foreign_key_check;
`)
		}
	}
	e := newEngine(t, db, migratetest.FS(files), "20260102000000")
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	mustExec(t, db.Driver(), `INSERT INTO parents (id, note) VALUES (1, 'x')`)
	mustExec(t, db.Driver(), `INSERT INTO children (id, parent_id) VALUES (1, 1), (2, 1)`)

	rep, err := e.Rollback(ctx, migrate.RollbackOptions{})
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if len(rep.Applied) != 1 {
		t.Fatalf("applied %+v", rep.Applied)
	}
	c := pinned(t, db)
	var kids, parents, fk, noteCols int
	scanOne(t, c, `SELECT count(*) FROM children`, &kids)
	scanOne(t, c, `SELECT count(*) FROM parents`, &parents)
	scanOne(t, c, `PRAGMA foreign_keys`, &fk)
	scanOne(t, c, `SELECT count(*) FROM pragma_table_info('parents') WHERE name = 'note'`, &noteCols)
	if kids != 2 || parents != 1 || fk != 1 || noteCols != 0 {
		t.Fatalf("children = %d (want 2), parents = %d (want 1), foreign_keys = %d (want 1), note columns = %d (want 0)", kids, parents, fk, noteCols)
	}
}

// TestSQLiteAutoMigrateWithStrictTable checks that a database holding a STRICT
// and AUTOINCREMENT table, which a model cannot declare, does not stop
// AutoMigrate: the unrelated change is applied and the table keeps its options.
func TestSQLiteAutoMigrateWithStrictTable(t *testing.T) {
	db := testopen.SQLite(t)
	ctx := context.Background()
	mustExec(t, db.Driver(), `CREATE TABLE s (id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT, v TEXT) STRICT`)
	mustExec(t, db.Driver(), `CREATE TABLE other (id INTEGER NOT NULL PRIMARY KEY)`)
	desired := &schema.Schema{Dialect: "sqlite", Tables: []schema.Table{
		{Name: "s", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "v", Type: "TEXT", Nullable: true}}, PrimaryKey: []string{"id"}},
		{Name: "other", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "note", Type: "TEXT", Nullable: true}}, PrimaryKey: []string{"id"}},
	}}
	rep, err := newEngine(t, db, migratetest.EmptyFS(), "20260101000000").AutoMigrate(ctx, desired)
	if err != nil {
		t.Fatalf("a STRICT table must not stop AutoMigrate: %v", err)
	}
	if len(rep.Applied) != 1 || rep.Applied[0].Name != "note" {
		t.Fatalf("the unrelated column must be added: %+v", rep)
	}
	c := pinned(t, db)
	var n int
	scanOne(t, c, `SELECT count(*) FROM pragma_table_info('other') WHERE name = 'note'`, &n)
	if n != 1 {
		t.Fatal("note was not added")
	}
	var text string
	scanOne(t, c, `SELECT sql FROM sqlite_schema WHERE name = 's'`, &text)
	if !strings.Contains(text, "AUTOINCREMENT") || !strings.HasSuffix(strings.TrimSpace(text), "STRICT") {
		t.Fatalf("s lost its options: %s", text)
	}
}
