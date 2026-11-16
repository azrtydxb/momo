// SPDX-License-Identifier: Apache-2.0

package mysql_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// Catches: a value-list change split into a drop and an add, which leaves the
// table without the constraint between them, and the stale Expr of the check
// written in place of its value list.
func TestRenderAlterCheckIn(t *testing.T) {
	after := schema.Check{Name: "notes_kind_check", Expr: "stale", In: &schema.CheckIn{Column: "kind", Values: []string{"c", "a", "b"}}}
	got := render(t, migrate.Change{Kind: migrate.AlterCheckIn, Table: "notes", Name: after.Name, After: &after, Added: []string{"c"}})
	want := "ALTER TABLE `notes` DROP CONSTRAINT `notes_kind_check`, ADD CONSTRAINT `notes_kind_check` CHECK (`kind` IN ('a','b','c'))"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %q, want one statement %q", got, want)
	}
}

// Catches: a created table writing the stale Expr of a value-list check, or
// quoting the column with double quotes, which MySQL reads as a string.
func TestRenderCreateTableWithCheckIn(t *testing.T) {
	tbl := schema.Table{
		Name:       "notes",
		PrimaryKey: []string{"id"},
		Columns:    []schema.Column{{Name: "id", Type: "bigint"}, {Name: "kind", Type: "varchar(20)"}},
		Checks:     []schema.Check{{Name: "notes_kind_check", Expr: "stale", In: &schema.CheckIn{Column: "kind", Values: []string{"b", "a"}}}},
	}
	got := render(t, migrate.Change{Kind: migrate.CreateTable, Table: "notes", Name: "notes", After: &tbl})
	if len(got) != 1 || !strings.Contains(got[0], "CONSTRAINT `notes_kind_check` CHECK (`kind` IN ('a','b'))") {
		t.Fatalf("got %q", got)
	}
}
