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

func deferrableMySQLTable(mutate func(*schema.Table)) schema.Table {
	t := schema.Table{
		Name:        "stops",
		Columns:     []schema.Column{{Name: "id", Type: "bigint"}, {Name: "route_id", Type: "bigint"}},
		PrimaryKey:  []string{"id"},
		Uniques:     []schema.Unique{{Name: "stops_route_key", Columns: []string{"route_id"}}},
		ForeignKeys: []schema.ForeignKey{{Name: "stops_route_id_fkey", Columns: []string{"route_id"}, RefTable: "routes", RefColumns: []string{"id"}}},
	}
	if mutate != nil {
		mutate(&t)
	}
	return t
}

// Catches: a deferrable constraint rendered as a plain one on MySQL and
// MariaDB, which have no DEFERRABLE: the migration would build a database that
// differs from the model without a word, instead of being refused with
// ErrUnsupported that names the constraint.
func TestMySQLRenderRefusesDeferrableConstraints(t *testing.T) {
	cases := map[string]struct {
		mutate func(*schema.Table)
		names  string
	}{
		"primary key":               {func(t *schema.Table) { t.PrimaryKeyDeferrable = true }, "primary key of table stops"},
		"primary key initially":     {func(t *schema.Table) { t.PrimaryKeyInitiallyDeferred = true }, "primary key of table stops"},
		"unique":                    {func(t *schema.Table) { t.Uniques[0].Deferrable = true }, "stops_route_key"},
		"unique initially deferred": {func(t *schema.Table) { t.Uniques[0].InitiallyDeferred = true }, "stops_route_key"},
		"foreign key":               {func(t *schema.Table) { t.ForeignKeys[0].Deferrable = true }, "stops_route_id_fkey"},
		"foreign key initially":     {func(t *schema.Table) { t.ForeignKeys[0].InitiallyDeferred = true }, "stops_route_id_fkey"},
	}
	for name, c := range cases {
		tbl := deferrableMySQLTable(c.mutate)
		stmts, err := Dialect{}.Render([]migrate.Change{{Kind: migrate.CreateTable, Table: tbl.Name, After: &tbl}})
		if !errors.Is(err, momo.ErrUnsupported) || !strings.Contains(err.Error(), c.names) {
			t.Errorf("%s: create_table: want ErrUnsupported naming %q, got %+v, %v", name, c.names, stmts, err)
		}
	}

	tbl := deferrableMySQLTable(nil)
	uq := tbl.Uniques[0]
	uq.Deferrable = true
	fk := tbl.ForeignKeys[0]
	fk.InitiallyDeferred = true
	for _, c := range []migrate.Change{
		{Kind: migrate.AddUnique, Table: "stops", Name: uq.Name, After: &uq},
		{Kind: migrate.AddFK, Table: "stops", Name: fk.Name, After: &fk},
		{Kind: migrate.AddPrimaryKey, Table: "stops", Name: "stops_pkey", After: []string{"id"}, Deferrable: true},
	} {
		stmts, err := Dialect{}.Render([]migrate.Change{c})
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: want ErrUnsupported, got %+v, %v", c.Kind, stmts, err)
		}
	}

	// A plain model still renders.
	plain := deferrableMySQLTable(nil)
	if _, err := (Dialect{}).Render([]migrate.Change{{Kind: migrate.CreateTable, Table: plain.Name, After: &plain}}); err != nil {
		t.Errorf("a plain table: %v", err)
	}
}
