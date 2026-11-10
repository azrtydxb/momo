// SPDX-License-Identifier: Apache-2.0

//go:build integration && cgo && !windows

package sqlite_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fkcase"
	"github.com/azrtydxb/momo/migrate"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
	"github.com/azrtydxb/momo/schema"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

type liteRunner struct {
	t *testing.T
	c momo.PinnedConn
}

func (r liteRunner) Exec(q string) error {
	_, err := r.c.Exec(context.Background(), q, nil)
	return err
}

func (r liteRunner) Scalar(q string) string {
	r.t.Helper()
	rows, err := r.c.Query(context.Background(), q, nil)
	if err != nil {
		r.t.Fatalf("%s: %v", q, err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		r.t.Fatalf("%s: no row", q)
	}
	var s string
	if err := rows.Scan(&s); err != nil {
		r.t.Fatalf("%s: %v", q, err)
	}
	return s
}

func (liteRunner) TextType() string { return "TEXT" }

func inspected(t *testing.T, c momo.PinnedConn) *schema.Schema {
	t.Helper()
	got, err := msqlite.New().Inspect(context.Background(), c, migrate.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// assertDiffEmpty fails when the models still differ from the inspection. It
// uses the diff, not schema.Equal: the model spells NO ACTION out and SQLite
// reports it as no clause, which the diff treats as the same.
func assertDiffEmpty(t *testing.T, got, want *schema.Schema) {
	t.Helper()
	changes, _, err := migrate.Diff(got, want, migrate.DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		var kinds []string
		for _, ch := range changes {
			kinds = append(kinds, string(ch.Kind)+" "+ch.Table+"."+ch.Name)
		}
		t.Fatalf("the inspection differs from the models: %v", kinds)
	}
}

// TestFKActions: the models' foreign key tags become constraints through the
// real schema builder and diff, every referential action survives render,
// apply and inspection, does what it says on live rows, and a relation with
// the no_constraint tag leaves no foreign key to inspect.
//
// Catches: an action dropped by the renderer or the inspector (a CASCADE that
// round-trips as NO ACTION leaves the child row behind), and a no_constraint
// that still creates a key.
func TestFKActions(t *testing.T) {
	db := testopen.SQLite(t)
	c := pinned(t, db)
	cases := fkcase.Cases(true)
	desired, err := fkcase.Schema(dialect.SQLite, cases)
	if err != nil {
		t.Fatal(err)
	}
	apply(t, c, render(t, &schema.Schema{Dialect: dialect.SQLite}, desired))

	got := inspected(t, c)
	assertDiffEmpty(t, got, desired)
	for _, tb := range got.Tables {
		if want := fkcase.WantForeignKeys(tb.Name); len(tb.ForeignKeys) != want {
			t.Errorf("table %s has %d foreign keys in the inspection, want %d", tb.Name, len(tb.ForeignKeys), want)
		}
	}
	if tb, ok := got.Table(fkcase.Loose); !ok || len(tb.ForeignKeys) != 0 {
		t.Fatalf("the no_constraint table must exist without a foreign key: %+v", tb)
	}
	r := liteRunner{t, c}
	if err := r.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	fkcase.Check(t, r, cases)
}

// Changing an action on an existing table needs a table rebuild on SQLite (an
// ALTER cannot change a key). The rows survive it and the new action is the
// one enforced afterwards.
func TestFKActionsChangeOnExistingTable(t *testing.T) {
	db := testopen.SQLite(t)
	c := pinned(t, db)
	cases := fkcase.Cases(true)
	v1, err := fkcase.Schema(dialect.SQLite, cases)
	if err != nil {
		t.Fatal(err)
	}
	apply(t, c, render(t, &schema.Schema{Dialect: dialect.SQLite}, v1))
	r := liteRunner{t, c}
	for _, q := range []string{
		`INSERT INTO fk_parents (id) VALUES (600)`,
		`INSERT INTO fk_del_cascade (id, parent_id) VALUES (7, 600)`,
	} {
		if err := r.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	changed := slices.Clone(cases)
	for i := range changed {
		if changed[i].Name == "del_cascade" {
			changed[i].OnDelete = "set_null"
		}
	}
	v2, err := fkcase.Schema(dialect.SQLite, changed)
	if err != nil {
		t.Fatal(err)
	}
	stmts := render(t, inspected(t, c), v2)
	// A rebuild copies the rows into a new table: an INSERT ... SELECT.
	if !slices.ContainsFunc(stmts, func(s migrate.Statement) bool { return strings.Contains(strings.ToUpper(s.SQL), "INSERT INTO") }) {
		t.Fatalf("changing the action must rebuild the table:\n%s", dump(stmts))
	}
	apply(t, c, stmts)
	assertDiffEmpty(t, inspected(t, c), v2)

	if err := r.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	if got := r.Scalar(`SELECT CAST(parent_id AS TEXT) FROM fk_del_cascade WHERE id = 7`); got != "600" {
		t.Fatalf("the rebuild must keep the row, parent_id = %s", got)
	}
	if err := r.Exec(`DELETE FROM fk_parents WHERE id = 600`); err != nil {
		t.Fatal(err)
	}
	if got := r.Scalar(`SELECT COALESCE(CAST(parent_id AS TEXT), 'NULL') FROM fk_del_cascade WHERE id = 7`); got != "NULL" {
		t.Fatalf("the changed key must SET NULL, the child has parent_id %s", got)
	}
}
