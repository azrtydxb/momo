// SPDX-License-Identifier: Apache-2.0

//go:build integration && cgo && !windows

package sqlite_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	msqlite "github.com/azrtydxb/momo/migrate/sqlite"
	"github.com/azrtydxb/momo/schema"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

// fkCase is one child table whose foreign key carries one referential action.
// Parent i is 10+i, so the cases never touch each other's rows.
type fkCase struct{ name, onDelete, onUpdate string }

var fkCases = []fkCase{
	{"del_noaction", "NO ACTION", ""}, {"del_restrict", "RESTRICT", ""}, {"del_cascade", "CASCADE", ""},
	{"del_setnull", "SET NULL", ""}, {"del_setdefault", "SET DEFAULT", ""},
	{"upd_noaction", "", "NO ACTION"}, {"upd_restrict", "", "RESTRICT"}, {"upd_cascade", "", "CASCADE"},
	{"upd_setnull", "", "SET NULL"}, {"upd_setdefault", "", "SET DEFAULT"},
}

// declared spells an action as a model does: NO ACTION is the default, which
// the database reports as no clause, so it is declared by leaving it out.
func declared(action string) string {
	if action == "NO ACTION" {
		return ""
	}
	return action
}

func fkActionSchema() *schema.Schema {
	tables := []schema.Table{{
		Name:       "fk_parents",
		Columns:    []schema.Column{{Name: "id", Type: "INTEGER"}},
		PrimaryKey: []string{"id"},
	}}
	for _, c := range fkCases {
		tables = append(tables, schema.Table{
			Name:       "fk_" + c.name,
			Columns:    []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "p", Type: "INTEGER", Nullable: true, Default: "0"}},
			PrimaryKey: []string{"id"},
			ForeignKeys: []schema.ForeignKey{{
				Name: "fk_" + c.name + "_p", Columns: []string{"p"}, RefTable: "fk_parents", RefColumns: []string{"id"},
				OnDelete: declared(c.onDelete), OnUpdate: declared(c.onUpdate),
			}},
		})
	}
	// A relation with the no_constraint tag: the column exists, the database
	// constraint does not.
	tables = append(tables, schema.Table{
		Name:       "fk_loose",
		Columns:    []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "parent_id", Type: "INTEGER", Nullable: true}},
		PrimaryKey: []string{"id"},
	})
	return &schema.Schema{Dialect: dialect.SQLite, Tables: tables}
}

func fkScalar(t *testing.T, c momo.PinnedConn, q string) string {
	t.Helper()
	rows, err := c.Query(context.Background(), q, nil)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatalf("%s: no row", q)
	}
	var s string
	if err := rows.Scan(&s); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return s
}

func fkExec(c momo.PinnedConn, q string) error {
	_, err := c.Exec(context.Background(), q, nil)
	return err
}

// TestFKActions: every referential action survives render, apply and
// inspection, does what it says on a live row, and a relation without a
// database constraint leaves no foreign key to inspect.
//
// Catches: an action dropped by the renderer or the inspector (a CASCADE that
// round-trips as NO ACTION leaves the child row behind after the delete).
func TestFKActions(t *testing.T) {
	db := testopen.SQLite(t)
	c := pinned(t, db)
	desired := fkActionSchema()
	apply(t, c, render(t, &schema.Schema{Dialect: dialect.SQLite}, desired))
	assertNoDiff(t, c, desired)

	got, err := msqlite.New().Inspect(context.Background(), c, migrate.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tb := range got.Tables {
		want := 1
		if tb.Name == "fk_parents" || tb.Name == "fk_loose" {
			want = 0
		}
		if len(tb.ForeignKeys) != want {
			t.Errorf("table %s has %d foreign keys in the inspection, want %d", tb.Name, len(tb.ForeignKeys), want)
		}
	}

	if err := fkExec(c, `PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	if err := fkExec(c, `INSERT INTO fk_parents (id) VALUES (0)`); err != nil {
		t.Fatal(err)
	}
	for i, fc := range fkCases {
		parent := 10 + i
		tbl := "fk_" + fc.name
		if err := fkExec(c, fmt.Sprintf(`INSERT INTO fk_parents (id) VALUES (%d)`, parent)); err != nil {
			t.Fatal(err)
		}
		if err := fkExec(c, fmt.Sprintf(`INSERT INTO %s (id, p) VALUES (1, %d)`, tbl, parent)); err != nil {
			t.Fatal(err)
		}
		var op string
		if fc.onDelete != "" {
			op = fmt.Sprintf(`DELETE FROM fk_parents WHERE id = %d`, parent)
		} else {
			op = fmt.Sprintf(`UPDATE fk_parents SET id = %d WHERE id = %d`, parent+100, parent)
		}
		opErr := fkExec(c, op)
		child := fkScalar(t, c, fmt.Sprintf(`SELECT COALESCE((SELECT COALESCE(CAST(p AS TEXT), 'NULL') FROM %s WHERE id = 1), 'GONE')`, tbl))
		var wantErr bool
		var wantChild string
		switch fc.onDelete + fc.onUpdate {
		case "NO ACTION", "RESTRICT":
			wantErr, wantChild = true, fmt.Sprint(parent)
		case "CASCADE":
			wantChild = fmt.Sprint(parent + 100)
			if fc.onDelete != "" {
				wantChild = "GONE"
			}
		case "SET NULL":
			wantChild = "NULL"
		case "SET DEFAULT":
			wantChild = "0"
		}
		if (opErr != nil) != wantErr || child != wantChild {
			t.Errorf("%s: %s gave error %v and child %q, want error=%v and child %q", fc.name, op, opErr, child, wantErr, wantChild)
		}
	}
}
