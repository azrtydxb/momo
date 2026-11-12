// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

func virtualDiffSchema(d dialect.Name, virtual bool, gen string) *schema.Schema {
	return &schema.Schema{Dialect: d, Tables: []schema.Table{{
		Name: "vg",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint"},
			{Name: "a", Type: "bigint", Nullable: true, Generated: gen, GeneratedVirtual: virtual},
		},
		PrimaryKey: []string{"id"},
	}}}
}

// Catches: columnAttrsDiffer ignoring the generated kind, so a change between
// VIRTUAL and STORED with the same expression is invisible to diff and drift.
func TestDiffGeneratedKindChange(t *testing.T) {
	for _, d := range []dialect.Name{dialect.MySQL, dialect.SQLite} {
		for _, flip := range []bool{true, false} {
			changes, _, err := migrate.Diff(virtualDiffSchema(d, flip, "id * 2"), virtualDiffSchema(d, !flip, "id * 2"), migrate.DiffOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if len(changes) != 1 || changes[0].Kind != migrate.AlterColumn || changes[0].Name != "a" {
				t.Errorf("%s, from virtual=%v: changes = %+v, want one alter_column of a", d, flip, changes)
			}
		}
		changes, _, err := migrate.Diff(virtualDiffSchema(d, true, "id * 2"), virtualDiffSchema(d, true, "id * 2"), migrate.DiffOptions{})
		if err != nil || len(changes) != 0 {
			t.Errorf("%s: equal VIRTUAL columns: %v, %+v", d, err, changes)
		}
	}
	// The flag means nothing without an expression.
	changes, _, err := migrate.Diff(virtualDiffSchema(dialect.MySQL, true, ""), virtualDiffSchema(dialect.MySQL, false, ""), migrate.DiffOptions{})
	if err != nil || len(changes) != 0 {
		t.Errorf("flag without an expression: %v, %+v", err, changes)
	}
}
