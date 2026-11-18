// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

func deferrableSQLiteSchema(mutate func(*schema.Table)) *schema.Schema {
	t := schema.Table{
		Name:        "stops",
		Columns:     []schema.Column{{Name: "id", Type: "INTEGER"}, {Name: "route_id", Type: "INTEGER"}},
		PrimaryKey:  []string{"id"},
		Uniques:     []schema.Unique{{Name: "stops_route_key", Columns: []string{"route_id"}}},
		ForeignKeys: []schema.ForeignKey{{Name: "stops_route_id_fkey", Columns: []string{"route_id"}, RefTable: "routes", RefColumns: []string{"id"}}},
	}
	if mutate != nil {
		mutate(&t)
	}
	routes := schema.Table{Name: "routes", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}}, PrimaryKey: []string{"id"}}
	return &schema.Schema{Dialect: "sqlite", Tables: []schema.Table{routes, t}}
}

// Catches: a deferrable constraint rendered as a plain one on SQLite, whose
// inspection cannot read a foreign key's deferrability back: the table would
// differ from the model on every drift run, and a primary key or unique
// constraint cannot be deferred at all. The renderer refuses with
// ErrUnsupported and names the constraint.
func TestSQLiteRenderRefusesDeferrableConstraints(t *testing.T) {
	cases := map[string]struct {
		mutate func(*schema.Table)
		names  string
	}{
		"primary key":               {func(t *schema.Table) { t.PrimaryKeyDeferrable = true }, "primary key of table"},
		"primary key initially":     {func(t *schema.Table) { t.PrimaryKeyInitiallyDeferred = true }, "primary key of table"},
		"unique":                    {func(t *schema.Table) { t.Uniques[0].Deferrable = true }, "stops_route_key"},
		"unique initially deferred": {func(t *schema.Table) { t.Uniques[0].InitiallyDeferred = true }, "stops_route_key"},
		"foreign key":               {func(t *schema.Table) { t.ForeignKeys[0].Deferrable = true }, "stops_route_id_fkey"},
		"foreign key initially":     {func(t *schema.Table) { t.ForeignKeys[0].InitiallyDeferred = true }, "stops_route_id_fkey"},
	}
	for name, c := range cases {
		to := deferrableSQLiteSchema(c.mutate)
		stmts, err := Dialect{}.Render([]migrate.Change{{Kind: migrate.CreateTable, Table: "stops", After: &to.Tables[1]}})
		if !errors.Is(err, momo.ErrUnsupported) || !strings.Contains(err.Error(), c.names) {
			t.Errorf("%s: create_table: want ErrUnsupported naming %q, got %+v, %v", name, c.names, stmts, err)
		}

		// A change of an existing table is a rebuild from the desired table.
		from := deferrableSQLiteSchema(nil)
		changes, _, derr := migrate.Diff(from, to, migrate.DiffOptions{AllowDestructive: true})
		if derr != nil {
			t.Fatalf("%s: diff: %v", name, derr)
		}
		ordered, derr := migrate.OrderWithViews(changes, from.Views, to.Views)
		if derr != nil {
			t.Fatalf("%s: order: %v", name, derr)
		}
		stmts, err = Dialect{}.RenderSchema(from, to, ordered)
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: rebuild: want ErrUnsupported, got %+v, %v", name, stmts, err)
		}
	}

	plain := deferrableSQLiteSchema(nil)
	if _, err := (Dialect{}).Render([]migrate.Change{{Kind: migrate.CreateTable, Table: "stops", After: &plain.Tables[1]}}); err != nil {
		t.Errorf("a plain table: %v", err)
	}
}
