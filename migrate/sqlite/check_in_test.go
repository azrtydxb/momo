// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

func notesWithCheckIn(values ...string) *schema.Schema {
	return liteSchema(schema.Table{
		Name:       "notes",
		Columns:    []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "kind", Type: "TEXT"}},
		PrimaryKey: []string{"id"},
		Checks:     []schema.Check{{Name: "notes_kind_check", Expr: "stale", In: &schema.CheckIn{Column: "kind", Values: values}}},
	})
}

// Catches: a value-list change applied as anything but a table rebuild (SQLite
// cannot alter a CHECK), the rebuilt table written with the stale Expr, and
// the change rendered twice.
func TestSQLiteRenderSchemaAlterCheckIn(t *testing.T) {
	from, to := notesWithCheckIn("a"), notesWithCheckIn("b", "a")
	ch := migrate.Change{Kind: migrate.AlterCheckIn, Table: "notes", Name: "notes_kind_check", Before: &from.Tables[0].Checks[0], After: &to.Tables[0].Checks[0], Added: []string{"b"}}
	stmts, err := Dialect{}.RenderSchema(from, to, []migrate.Change{ch})
	if err != nil {
		t.Fatal(err)
	}
	if len(stmts) < 4 || !strings.HasPrefix(stmts[0].SQL, `CREATE TABLE "new_notes"`) || !strings.Contains(stmts[0].SQL, `CONSTRAINT "notes_kind_check" CHECK ("kind" IN ('a','b'))`) {
		t.Fatalf("a changed value list must rebuild the table with the new CHECK, got %+v", stmts)
	}
	n := 0
	for _, s := range stmts {
		if strings.HasPrefix(s.SQL, `CREATE TABLE "new_notes"`) {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("the table is rebuilt %d times, want once: %+v", n, stmts)
	}
	if !(Dialect{}).NeedsRebuild(from, ch) {
		t.Fatal("NeedsRebuild must report an alter_check_in")
	}
}

// Catches: a named value-list constraint read back as an enum column, which
// drift reports as a changed table, and an unnamed one (the CHECK an enum
// column writes) read as a constraint.
func TestApplyChecksFoldsOnlyUnnamedValueLists(t *testing.T) {
	def := tableDef{checks: []checkText{
		{name: "", expr: `"mood" IN ('happy', 'sad')`},
		{name: "notes_kind_check", expr: `"kind" IN ('b','a')`},
		{name: "notes_other", expr: `length("kind") > 0`},
	}}
	tbl := &schema.Table{Name: "notes", Columns: []schema.Column{{Name: "mood", Type: "TEXT"}, {Name: "kind", Type: "TEXT"}}}
	enums := applyChecks(tbl, def)
	if len(enums) != 1 || enums[0].Name != "notes_mood" || tbl.Columns[0].Enum != "notes_mood" || tbl.Columns[1].Enum != "" {
		t.Fatalf("enums = %+v, columns = %+v: only the unnamed check is an enum", enums, tbl.Columns)
	}
	if len(tbl.Checks) != 2 {
		t.Fatalf("checks = %+v", tbl.Checks)
	}
	in := tbl.Checks[0].In
	if tbl.Checks[0].Name != "notes_kind_check" || in == nil || in.Column != "kind" || strings.Join(in.Values, ",") != "a,b" {
		t.Fatalf("the named value list must be a Check with In, got %+v", tbl.Checks[0])
	}
	if tbl.Checks[1].In != nil {
		t.Fatalf("a plain check must stay plain, got %+v", tbl.Checks[1])
	}
}
