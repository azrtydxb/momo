// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// Catches: a value-list change that is not a drop and an add NOT VALID in the
// transactional head with the validation outside it, the values written
// unsorted or unescaped, or the stale Expr of the check used in place of its
// value list.
func TestRenderAlterCheckIn(t *testing.T) {
	after := schema.Check{Name: "notes_kind_check", Expr: "stale", In: &schema.CheckIn{Column: "kind", Values: []string{"c", "it's", "a"}}}
	got := renderSQL(t, migrate.Change{Kind: migrate.AlterCheckIn, Table: "notes", Name: after.Name, After: &after, Added: []string{"c"}})
	want := []migrate.Statement{
		{SQL: `ALTER TABLE "notes" DROP CONSTRAINT "notes_kind_check"`},
		{SQL: `ALTER TABLE "notes" ADD CONSTRAINT "notes_kind_check" CHECK ("kind" IN ('a','c','it''s')) NOT VALID`},
		{SQL: `ALTER TABLE "notes" VALIDATE CONSTRAINT "notes_kind_check"`, NoTx: true},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].SQL != want[i].SQL || got[i].NoTx != want[i].NoTx {
			t.Errorf("statement %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// Catches: a table created with a value-list check writing the stale Expr.
func TestRenderCreateTableWithCheckIn(t *testing.T) {
	tbl := schema.Table{
		Name:    "notes",
		Columns: []schema.Column{{Name: "kind", Type: "text"}},
		Checks:  []schema.Check{{Name: "notes_kind_check", Expr: "stale", In: &schema.CheckIn{Column: "kind", Values: []string{"b", "a"}}}},
	}
	got := renderSQL(t, migrate.Change{Kind: migrate.CreateTable, Table: "notes", Name: "notes", After: &tbl})
	if len(got) != 1 || !strings.Contains(got[0].SQL, `CONSTRAINT "notes_kind_check" CHECK ("kind" IN ('a','b'))`) {
		t.Fatalf("got %+v", got)
	}
}
