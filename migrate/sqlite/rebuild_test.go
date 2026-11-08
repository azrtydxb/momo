// SPDX-License-Identifier: Apache-2.0

//go:build integration && cgo && !windows

package sqlite_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
	"github.com/azrtydxb/momo/schema"
)

var itemsDesired = schema.Table{Name: "items", Columns: []schema.Column{
	{Name: "id", Type: "INTEGER"}, {Name: "name", Type: "TEXT", Nullable: true},
	{Name: "qty", Type: "INTEGER", Default: "0"},
}, PrimaryKey: []string{"id"}}

var itemsIndex = []schema.Index{{Name: "items_name", Parts: []schema.IndexPart{{Column: "name"}}}}

func TestSQLiteRebuild(t *testing.T) {
	ctx := context.Background()
	c := newRebuildFixture(t)
	reset, err := msqlite.New().Setup(ctx, c, migrate.SessionOptions{ForeignKeysOff: true})
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := msqlite.Rebuild(ctx, c, "items", itemsDesired, itemsIndex)
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
	reset, err := msqlite.New().Setup(ctx, c, migrate.SessionOptions{ForeignKeysOff: true})
	if err != nil {
		t.Fatal(err)
	}
	stmts, err := msqlite.Rebuild(ctx, c, "items", itemsDesired, nil)
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
	reset, err := msqlite.New().Setup(ctx, c, migrate.SessionOptions{ForeignKeysOff: true})
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
	stmts, err := msqlite.Rebuild(ctx, c, "items", itemsDesired, itemsIndex)
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
	stmts, err := msqlite.Rebuild(ctx, c, "a", desired, nil)
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
	deps, err := msqlite.Capture(ctx, c, "items")
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
