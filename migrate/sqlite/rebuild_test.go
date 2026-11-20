// SPDX-License-Identifier: Apache-2.0

//go:build integration && cgo && !windows

package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// rebuildPinned pins one connection of db and releases it when the test ends.
func rebuildPinned(t *testing.T, db *momo.DB) momo.PinnedConn {
	t.Helper()
	pn, ok := db.Driver().(momo.Pinner)
	if !ok {
		t.Fatal("driver does not implement momo.Pinner")
	}
	c, err := pn.Pin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Release() }) // Cleanup has no error channel; Release failures do not affect the assertions
	return c
}

func mustExec(t *testing.T, c momo.PinnedConn, sql string) {
	t.Helper()
	if _, err := c.Exec(context.Background(), sql, nil); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// scanOne runs a query on c, requires one row and scans it into dest.
func scanOne(t *testing.T, c momo.PinnedConn, sql string, dest ...any) {
	t.Helper()
	rows, err := c.Query(context.Background(), sql, nil)
	if err != nil {
		t.Fatalf("query %q: %v", sql, err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		t.Fatalf("no rows for %q", sql)
	}
	if err := rows.Scan(dest...); err != nil {
		t.Fatal(err)
	}
}

// newRebuildFixture returns a pinned connection with foreign_keys ON, a parent
// table items with an index, a view, a trigger on items, a child table links
// with an FK to items and a trigger on links that updates items, and three
// parent rows and two child rows.
func newRebuildFixture(t *testing.T) momo.PinnedConn {
	t.Helper()
	db := testopen.SQLite(t)
	c := rebuildPinned(t, db)
	mustExec(t, c, `PRAGMA foreign_keys = ON`)
	mustExec(t, c, `CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT, qty INTEGER)`)
	mustExec(t, c, `CREATE INDEX items_name ON items(name)`)
	mustExec(t, c, `CREATE TRIGGER items_audit AFTER UPDATE ON items BEGIN SELECT 1; END`)
	mustExec(t, c, `CREATE VIEW items_v AS SELECT id, name FROM items`)
	mustExec(t, c, `CREATE TABLE links (id INTEGER PRIMARY KEY, item_id INTEGER NOT NULL REFERENCES items(id))`)
	mustExec(t, c, `CREATE TRIGGER links_touch AFTER INSERT ON links BEGIN UPDATE items SET qty = qty WHERE id = NEW.item_id; END`)
	mustExec(t, c, `INSERT INTO items (name, qty) VALUES ('a', 1), ('b', 2), ('c', 3)`)
	mustExec(t, c, `INSERT INTO links (item_id) VALUES (1), (2)`)
	return c
}

var itemsDesired = schema.Table{Name: "items", Columns: []schema.Column{
	{Name: "id", Type: "INTEGER"}, {Name: "name", Type: "TEXT", Nullable: true},
	{Name: "qty", Type: "INTEGER", Default: "0"},
}, PrimaryKey: []string{"id"}}

var itemsIndex = []schema.Index{{Name: "items_name", Parts: []schema.IndexPart{{Column: "name"}}}}

func TestSQLiteRebuild(t *testing.T) {
	ctx := context.Background()
	c := newRebuildFixture(t)
	reset, err := New().Setup(ctx, c, migrate.SessionOptions{ForeignKeysOff: true})
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := rebuildLive(ctx, c, "items", itemsDesired, itemsIndex)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.RunStatements(ctx, c, stmts); err != nil {
		t.Fatal(err)
	}
	if err := reset(); err != nil {
		t.Fatal(err)
	}
	var n int
	scanOne(t, c, `SELECT count(*) FROM items`, &n)
	if n != 3 {
		t.Fatalf("rows lost: %d", n)
	}
	var sum int
	scanOne(t, c, `SELECT sum(qty) FROM items`, &sum)
	if sum != 6 {
		t.Fatalf("row content changed: sum(qty) = %d, want 6", sum)
	}
	for _, want := range [][2]string{{"index", "items_name"}, {"trigger", "items_audit"}, {"trigger", "links_touch"}, {"view", "items_v"}} {
		var count int
		scanOne(t, c, `SELECT count(*) FROM sqlite_schema WHERE type='`+want[0]+`' AND name='`+want[1]+`'`, &count)
		if count != 1 {
			t.Fatalf("%s %s missing after rebuild", want[0], want[1])
		}
	}
	var viewRows int
	scanOne(t, c, `SELECT count(*) FROM items_v`, &viewRows)
	if viewRows != 3 {
		t.Fatalf("the recreated view reads %d rows, want 3", viewRows)
	}
	var def string
	scanOne(t, c, `SELECT dflt_value FROM pragma_table_info('items') WHERE name = 'qty'`, &def)
	if def != "0" {
		t.Fatalf("the rebuilt table must carry the new definition, qty default = %q", def)
	}
	var violations int
	scanOne(t, c, `SELECT count(*) FROM pragma_foreign_key_check`, &violations)
	if violations != 0 {
		t.Fatalf("foreign_key_check returned %d rows after the rebuild", violations)
	}
	var fk int
	scanOne(t, c, `PRAGMA foreign_keys`, &fk)
	if fk != 1 {
		t.Fatalf("foreign_keys must be restored to ON, got %d", fk)
	}
}

func TestSQLiteRebuildFKStillEnforced(t *testing.T) {
	ctx := context.Background()
	c := newRebuildFixture(t)
	reset, err := New().Setup(ctx, c, migrate.SessionOptions{ForeignKeysOff: true})
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := rebuildLive(ctx, c, "items", itemsDesired, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.RunStatements(ctx, c, stmts); err != nil {
		t.Fatal(err)
	}
	if err := reset(); err != nil {
		t.Fatal(err)
	}
	_, err = c.Exec(ctx, `INSERT INTO links (id, item_id) VALUES (99, 404)`, nil)
	if err == nil {
		t.Fatal("orphan insert must fail: the foreign key is not enforced after the rebuild")
	}
	if !errors.Is(err, momo.ErrForeignKeyViolation) {
		t.Fatalf("want a foreign key violation, got %v", err)
	}
}

func TestSQLiteRebuildOrphanRollsBack(t *testing.T) {
	ctx := context.Background()
	c := newRebuildFixture(t)
	reset, err := New().Setup(ctx, c, migrate.SessionOptions{ForeignKeysOff: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := reset(); err != nil {
			t.Fatal(err)
		}
	}()
	// An orphan that the rebuild would carry forward (FKs are off while the orphan is inserted).
	mustExec(t, c, `INSERT INTO links (id, item_id) VALUES (99, 404)`)
	stmts, err := rebuildLive(ctx, c, "items", itemsDesired, itemsIndex)
	if err != nil {
		t.Fatal(err)
	}
	err = migrate.RunStatements(ctx, c, stmts)
	if err == nil || !strings.Contains(err.Error(), "foreign_key_check") || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("want a foreign_key_check violation with rollback, got %v", err)
	}
	if !errors.Is(err, momo.ErrForeignKeyViolation) {
		t.Fatalf("the violation must be typed momo.ErrForeignKeyViolation, got %v", err)
	}
	var sqlText string
	scanOne(t, c, `SELECT sql FROM sqlite_schema WHERE type='table' AND name='items'`, &sqlText)
	if sqlText != `CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT, qty INTEGER)` {
		t.Fatalf("rollback must restore the original table definition, got %q", sqlText)
	}
	var n int
	scanOne(t, c, `SELECT count(*) FROM items`, &n)
	if n != 3 {
		t.Fatalf("rollback must keep all rows, got %d", n)
	}
	for _, name := range []string{"items_audit", "links_touch", "items_v", "items_name"} {
		var count int
		scanOne(t, c, `SELECT count(*) FROM sqlite_schema WHERE name = '`+name+`'`, &count)
		if count != 1 {
			t.Fatalf("rollback must restore %s", name)
		}
	}
}

func TestSQLiteRebuildBlocked(t *testing.T) {
	ctx := context.Background()
	c := newRebuildFixture(t)
	mustExec(t, c, `CREATE TABLE a (id INTEGER PRIMARY KEY, x TEXT)`)
	mustExec(t, c, `CREATE VIEW a_v AS SELECT x FROM a`)
	desired := schema.Table{Name: "a", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}}, PrimaryKey: []string{"id"}}
	stmts, err := rebuildLive(ctx, c, "a", desired, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = migrate.RunStatements(ctx, c, stmts)
	if !errors.Is(err, migrate.ErrRebuildBlocked) || !strings.Contains(err.Error(), "a_v") {
		t.Fatalf("want ErrRebuildBlocked naming a_v, got %v", err)
	}
	var sqlText string
	scanOne(t, c, `SELECT sql FROM sqlite_schema WHERE type='table' AND name='a'`, &sqlText)
	if sqlText != `CREATE TABLE a (id INTEGER PRIMARY KEY, x TEXT)` {
		t.Fatalf("a blocked rebuild must roll back, got %q", sqlText)
	}
}

// TestSQLiteCaptureFollowsViews checks that a view built on a captured view is
// captured too, and that a string literal naming the table is not a mention.
func TestSQLiteCaptureFollowsViews(t *testing.T) {
	ctx := context.Background()
	c := newRebuildFixture(t)
	mustExec(t, c, `CREATE VIEW items_v2 AS SELECT name FROM items_v`)
	mustExec(t, c, `CREATE VIEW other AS SELECT 'items' AS label`)
	deps, err := captureDependents(ctx, c, "items")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, d := range deps {
		names = append(names, d.Type+":"+d.Name)
	}
	got := strings.Join(names, ",")
	want := "trigger:items_audit,view:items_v,trigger:links_touch,view:items_v2"
	if got != want {
		t.Fatalf("captured %s, want %s", got, want)
	}
}
