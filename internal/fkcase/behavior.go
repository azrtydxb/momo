// SPDX-License-Identifier: Apache-2.0

package fkcase

import (
	"fmt"
	"testing"
)

// Runner runs SQL on the database under test.
type Runner interface {
	// Exec runs a statement and returns its error.
	Exec(sql string) error
	// Scalar runs a query that returns one text value.
	Scalar(sql string) string
	// TextType is the SQL type name that casts an integer to text.
	TextType() string
}

// Check inserts one parent and one child row per case and does to the parent
// what the case's action reacts to (a delete, or a change of its key), then
// asserts the effect the action promises: refused for NO ACTION, RESTRICT and
// the default; the child gone or moved for CASCADE; NULL for SET NULL; the
// column default (0) for SET DEFAULT. A failing assertion names the case.
func Check(t testing.TB, r Runner, cases []Case) {
	t.Helper()
	if err := r.Exec(fmt.Sprintf("INSERT INTO %s (id) VALUES (0)", Parent)); err != nil {
		t.Fatal(err)
	}
	for i, c := range cases {
		parent, tbl := 10+i, "fk_"+c.Name
		if err := r.Exec(fmt.Sprintf("INSERT INTO %s (id) VALUES (%d)", Parent, parent)); err != nil {
			t.Fatal(err)
		}
		if err := r.Exec(fmt.Sprintf("INSERT INTO %s (id, parent_id) VALUES (1, %d)", tbl, parent)); err != nil {
			t.Fatal(err)
		}
		action, op := c.OnDelete, fmt.Sprintf("DELETE FROM %s WHERE id = %d", Parent, parent)
		if c.OnUpdate != "" {
			action, op = c.OnUpdate, fmt.Sprintf("UPDATE %s SET id = %d WHERE id = %d", Parent, parent+100, parent)
		}
		opErr := r.Exec(op)
		child := r.Scalar(fmt.Sprintf("SELECT COALESCE((SELECT COALESCE(CAST(parent_id AS %s), 'NULL') FROM %s WHERE id = 1), 'GONE')", r.TextType(), tbl))

		wantErr, wantChild := false, ""
		switch action {
		case "", "no_action", "restrict":
			wantErr, wantChild = true, fmt.Sprint(parent)
		case "cascade":
			wantChild = fmt.Sprint(parent + 100)
			if c.OnUpdate == "" {
				wantChild = "GONE"
			}
		case "set_null":
			wantChild = "NULL"
		case "set_default":
			wantChild = "0"
		}
		if (opErr != nil) != wantErr || child != wantChild {
			t.Errorf("%s: %s gave error %v and child %q, want error=%v and child %q", c.Name, op, opErr, child, wantErr, wantChild)
		}
	}
}

// WantForeignKeys is the number of foreign keys an inspection must report for
// the table: none for the parent and for the no_constraint table, one for each
// case table.
func WantForeignKeys(table string) int {
	if table == Parent || table == Loose {
		return 0
	}
	return 1
}
