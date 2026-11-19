// SPDX-License-Identifier: Apache-2.0

package mysql

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// Catches: an exclusion constraint rendered as nothing on MySQL and MariaDB,
// which have none: the migration would build a table that accepts the rows the
// model forbids, instead of being refused with ErrUnsupported that names the
// constraint.
func TestMySQLRenderRefusesExclusionConstraints(t *testing.T) {
	tbl := schema.Table{
		Name:       "bookings",
		Columns:    []schema.Column{{Name: "id", Type: "bigint"}, {Name: "room_id", Type: "bigint"}},
		PrimaryKey: []string{"id"},
		Excludes:   []schema.Exclude{{Name: "bookings_no_overlap", Elements: []schema.ExcludeElement{{Column: "room_id", Operator: "="}}}},
	}
	stmts, err := Dialect{}.Render([]migrate.Change{{Kind: migrate.CreateTable, Table: tbl.Name, After: &tbl}})
	if !errors.Is(err, momo.ErrUnsupported) || !strings.Contains(err.Error(), "bookings_no_overlap") {
		t.Errorf("create_table: want ErrUnsupported naming the constraint, got %+v, %v", stmts, err)
	}
	for _, c := range []migrate.Change{
		{Kind: migrate.AddExclude, Table: "bookings", Name: "bookings_no_overlap", After: &tbl.Excludes[0]},
		{Kind: migrate.DropExclude, Table: "bookings", Name: "bookings_no_overlap", Before: &tbl.Excludes[0]},
	} {
		stmts, err := Dialect{}.Render([]migrate.Change{c})
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: want ErrUnsupported, got %+v, %v", c.Kind, stmts, err)
		}
	}
}
