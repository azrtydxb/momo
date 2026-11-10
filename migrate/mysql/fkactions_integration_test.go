// SPDX-License-Identifier: Apache-2.0

//go:build integration

package mysql_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
	"github.com/azrtydxb/momo/schema"
)

// fkCase is one child table whose foreign key carries one referential action.
// MySQL accepts SET DEFAULT in the grammar but InnoDB rejects it, so it is not
// a case here. Parent i is 10+i, so the cases never touch each other's rows.
type fkCase struct{ name, onDelete, onUpdate string }

var fkCases = []fkCase{
	{"del_noaction", "NO ACTION", ""}, {"del_restrict", "RESTRICT", ""}, {"del_cascade", "CASCADE", ""}, {"del_setnull", "SET NULL", ""},
	{"upd_noaction", "", "NO ACTION"}, {"upd_restrict", "", "RESTRICT"}, {"upd_cascade", "", "CASCADE"}, {"upd_setnull", "", "SET NULL"},
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
	tables := []schema.Table{parentsTable()}
	for _, c := range fkCases {
		tables = append(tables, schema.Table{
			Name:       "fk_" + c.name,
			Columns:    []schema.Column{{Name: "id", Type: "bigint"}, {Name: "p", Type: "int", Nullable: true}},
			PrimaryKey: []string{"id"},
			ForeignKeys: []schema.ForeignKey{{
				Name: "fk_" + c.name + "_p", Columns: []string{"p"}, RefTable: "parents", RefColumns: []string{"a"},
				OnDelete: declared(c.onDelete), OnUpdate: declared(c.onUpdate),
			}},
		})
	}
	// A relation with the no_constraint tag: the column exists, the database
	// constraint does not.
	tables = append(tables, schema.Table{
		Name:       "fk_loose",
		Columns:    []schema.Column{{Name: "id", Type: "bigint"}, {Name: "parent_id", Type: "int", Nullable: true}},
		PrimaryKey: []string{"id"},
	})
	return &schema.Schema{Dialect: "mysql", Tables: tables}
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
// round-trips as NO ACTION leaves the child row behind after the delete).
func TestFKActions(t *testing.T) {
	for _, tg := range targets {
		t.Run(tg.name, func(t *testing.T) {
			db := tg.open(t)
			dir := t.TempDir()
			eng := migratetest.NewEngine(t, mmysql.New(), db.Driver(), os.DirFS(dir))
			desired := fkActionSchema()
			applyExample(t, dir, eng, desired, "fk_actions")
			assertLive(t, eng, desired)

			live, err := eng.Inspect(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			for _, tb := range live.Tables {
				want := 1
				if tb.Name == "parents" || tb.Name == "fk_loose" {
					want = 0
				}
				if len(tb.ForeignKeys) != want {
					t.Errorf("table %s has %d foreign keys in the inspection, want %d", tb.Name, len(tb.ForeignKeys), want)
				}
			}

			for i, c := range fkCases {
				parent := 10 + i
				tbl := "fk_" + c.name
				if err := fkExec(db, fmt.Sprintf(`INSERT INTO parents (id, a, b) VALUES (%d, %d, 0)`, parent, parent)); err != nil {
					t.Fatal(err)
				}
				if err := fkExec(db, fmt.Sprintf(`INSERT INTO %s (id, p) VALUES (1, %d)`, tbl, parent)); err != nil {
					t.Fatal(err)
				}
				var op string
				if c.onDelete != "" {
					op = fmt.Sprintf(`DELETE FROM parents WHERE a = %d`, parent)
				} else {
					op = fmt.Sprintf(`UPDATE parents SET a = %d WHERE a = %d`, parent+100, parent)
				}
				err := fkExec(db, op)
				child := fkScalar(t, db, fmt.Sprintf(`SELECT COALESCE((SELECT COALESCE(CAST(p AS CHAR), 'NULL') FROM %s WHERE id = 1), 'GONE')`, tbl))
				var wantErr bool
				var wantChild string
				switch c.onDelete + c.onUpdate {
				case "NO ACTION", "RESTRICT":
					wantErr, wantChild = true, fmt.Sprint(parent)
				case "CASCADE":
					wantChild = fmt.Sprint(parent + 100)
					if c.onDelete != "" {
						wantChild = "GONE"
					}
				case "SET NULL":
					wantChild = "NULL"
				}
				if (err != nil) != wantErr || child != wantChild {
					t.Errorf("%s: %s gave error %v and child %q, want error=%v and child %q", c.name, op, err, child, wantErr, wantChild)
				}
			}
		})
	}
}
