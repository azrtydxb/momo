// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/azrtydxb/momo"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	"github.com/azrtydxb/momo/schema"
)

// fkCase is one child table whose foreign key carries one referential action.
type fkCase struct{ name, onDelete, onUpdate string }

// fkCases covers every action the diff and the dialects know, on delete and on
// update. Parent i is 10+i, so the cases never touch each other's rows.
var fkCases = []fkCase{
	{"del_noaction", "NO ACTION", ""}, {"del_restrict", "RESTRICT", ""}, {"del_cascade", "CASCADE", ""},
	{"del_setnull", "SET NULL", ""}, {"del_setdefault", "SET DEFAULT", ""},
	{"upd_noaction", "", "NO ACTION"}, {"upd_restrict", "", "RESTRICT"}, {"upd_cascade", "", "CASCADE"},
	{"upd_setnull", "", "SET NULL"}, {"upd_setdefault", "", "SET DEFAULT"},
}

func fkActionSchema() *schema.Schema {
	tables := []schema.Table{{
		Name:       "fk_parents",
		Columns:    []schema.Column{{Name: "id", Type: "bigint"}},
		PrimaryKey: []string{"id"},
	}}
	for _, c := range fkCases {
		tables = append(tables, schema.Table{
			Name:       "fk_" + c.name,
			Columns:    []schema.Column{{Name: "id", Type: "bigint"}, {Name: "p", Type: "bigint", Nullable: true, Default: "0"}},
			PrimaryKey: []string{"id"},
			ForeignKeys: []schema.ForeignKey{{
				Name: "fk_" + c.name + "_p", Columns: []string{"p"}, RefTable: "fk_parents", RefColumns: []string{"id"},
				OnDelete: c.onDelete, OnUpdate: c.onUpdate,
			}},
		})
	}
	// A relation with the no_constraint tag: the column exists, the database
	// constraint does not.
	tables = append(tables, schema.Table{
		Name:       "fk_loose",
		Columns:    []schema.Column{{Name: "id", Type: "bigint"}, {Name: "parent_id", Type: "bigint", Nullable: true}},
		PrimaryKey: []string{"id"},
	})
	return &schema.Schema{Dialect: "postgres", Tables: tables}
}

func fkScalar(t *testing.T, db *momo.DB, q string) string {
	t.Helper()
	rows, err := db.Driver().Query(context.Background(), q, nil)
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

func fkExec(db *momo.DB, q string) error {
	_, err := db.Driver().Exec(context.Background(), q, nil)
	return err
}

// TestFKActions: every referential action survives render, apply and
// inspection, does what it says on a live row, and a relation without a
// database constraint leaves no foreign key to inspect.
//
// Catches: an action dropped by the renderer or the inspector (a CASCADE that
// round-trips as NO ACTION would pass a schema comparison that ignored it, but
// not the row that is still there after the delete).
func TestFKActions(t *testing.T) {
	db := testopen.Postgres(t)
	desired := fkActionSchema()
	applyStatements(t, db, planStatements(t, &schema.Schema{Dialect: "postgres"}, desired, migrate.DiffOptions{}))

	got := inspectNow(t, db)
	desired.Extensions = got.Extensions
	if !schema.Equal(got, desired) {
		t.Fatalf("the actions did not round-trip:\n%s", migratetest.Diff(got, desired))
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
	if again := planStatements(t, got, desired, migrate.DiffOptions{}); len(again) != 0 {
		t.Fatalf("a second diff must be empty: %+v", again)
	}

	if err := fkExec(db, `INSERT INTO fk_parents (id) VALUES (0)`); err != nil {
		t.Fatal(err)
	}
	for i, c := range fkCases {
		parent := 10 + i
		tbl := "fk_" + c.name
		if err := fkExec(db, fmt.Sprintf(`INSERT INTO fk_parents (id) VALUES (%d)`, parent)); err != nil {
			t.Fatal(err)
		}
		if err := fkExec(db, fmt.Sprintf(`INSERT INTO %s (id, p) VALUES (1, %d)`, tbl, parent)); err != nil {
			t.Fatal(err)
		}
		var op string
		if c.onDelete != "" {
			op = fmt.Sprintf(`DELETE FROM fk_parents WHERE id = %d`, parent)
		} else {
			op = fmt.Sprintf(`UPDATE fk_parents SET id = %d WHERE id = %d`, parent+100, parent)
		}
		err := fkExec(db, op)
		child := fkScalar(t, db, fmt.Sprintf(`SELECT coalesce((SELECT coalesce(p::text, 'NULL') FROM %s WHERE id = 1), 'GONE')`, tbl))
		var wantErr bool
		var wantChild string
		switch action := c.onDelete + c.onUpdate; action {
		case "NO ACTION", "RESTRICT":
			wantErr, wantChild = true, fmt.Sprint(parent)
		case "CASCADE":
			wantChild = fmt.Sprint(parent + 100)
			if c.onDelete != "" {
				wantChild = "GONE"
			}
		case "SET NULL":
			wantChild = "NULL"
		case "SET DEFAULT":
			wantChild = "0"
		}
		if (err != nil) != wantErr || child != wantChild {
			t.Errorf("%s: %s gave error %v and child %q, want error=%v and child %q", c.name, op, err, child, wantErr, wantChild)
		}
	}
}
