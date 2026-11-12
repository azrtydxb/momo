// SPDX-License-Identifier: Apache-2.0

package mysql_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/migrate"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
	"github.com/azrtydxb/momo/schema"
)

// Catches: a VIRTUAL generated column rendered as STORED, which creates the
// wrong kind on a fresh install.
func TestRenderVirtualGeneratedColumn(t *testing.T) {
	tbl := schema.Table{
		Name:       "vg",
		PrimaryKey: []string{"id"},
		Columns: []schema.Column{
			{Name: "id", Type: "bigint"},
			{Name: "a", Type: "bigint", Nullable: true, Generated: "id * 2", GeneratedVirtual: true},
			{Name: "b", Type: "bigint", Nullable: true, Generated: "id * 3"},
		},
	}
	got := render(t, migrate.Change{Kind: migrate.CreateTable, Table: "vg", After: tbl})[0]
	if !strings.Contains(got, "`a` bigint GENERATED ALWAYS AS (id * 2) VIRTUAL") {
		t.Errorf("a is not VIRTUAL: %s", got)
	}
	if !strings.Contains(got, "`b` bigint GENERATED ALWAYS AS (id * 3) STORED") {
		t.Errorf("b is not STORED: %s", got)
	}
}

// Catches: a kind change rendered as MODIFY COLUMN, which MySQL refuses, or as
// a drop and add that moves the column and fails under an index. The change is
// refused with the custom-migration error PostgreSQL gives for a generated
// change.
func TestRenderGeneratedKindChangeIsRefused(t *testing.T) {
	virtual := schema.Column{Name: "a", Type: "bigint", Nullable: true, Generated: "id * 2", GeneratedVirtual: true}
	stored := virtual
	stored.GeneratedVirtual = false
	for _, c := range []struct{ from, to schema.Column }{{virtual, stored}, {stored, virtual}} {
		_, err := mmysql.New().Render([]migrate.Change{{Kind: migrate.AlterColumn, Table: "vg", Name: "a", Before: c.from, After: c.to}})
		if err == nil || !strings.Contains(err.Error(), "write a custom migration") {
			t.Errorf("%v to %v: err = %v, want the custom migration error", c.from.GeneratedVirtual, c.to.GeneratedVirtual, err)
		}
	}
	// A change of the expression alone is still a MODIFY COLUMN.
	other := virtual
	other.Generated = "id * 5"
	stmts, err := mmysql.New().Render([]migrate.Change{{Kind: migrate.AlterColumn, Table: "vg", Name: "a", Before: virtual, After: other}})
	if err != nil || len(stmts) != 1 || !strings.Contains(stmts[0].SQL, "MODIFY COLUMN") || !strings.Contains(stmts[0].SQL, "VIRTUAL") {
		t.Fatalf("expression change: %v, %+v", err, stmts)
	}
}
