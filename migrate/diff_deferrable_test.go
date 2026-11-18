// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"reflect"
	"testing"

	"github.com/azrtydxb/momo/schema"
)

func stopsSchema(mutate func(*schema.Table)) *schema.Schema {
	t := schema.Table{
		Name: "stops",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint"},
			{Name: "route_id", Type: "bigint"},
			{Name: "position", Type: "integer"},
		},
		PrimaryKey:  []string{"id"},
		Uniques:     []schema.Unique{{Name: "stops_route_position_key", Columns: []string{"route_id", "position"}}},
		ForeignKeys: []schema.ForeignKey{{Name: "stops_route_id_fkey", Columns: []string{"route_id"}, RefTable: "routes", RefColumns: []string{"id"}}},
	}
	if mutate != nil {
		mutate(&t)
	}
	routes := schema.Table{Name: "routes", Columns: []schema.Column{{Name: "id", Type: "bigint"}}, PrimaryKey: []string{"id"}}
	return &schema.Schema{Dialect: "postgres", Tables: []schema.Table{routes, t}}
}

// Catches: a change of deferrability that Diff does not see, or sees as the
// wrong object, or that drops a constraint and forgets the add.
func TestDiffChangesDeferrability(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*schema.Table)
		want   []ChangeKind
	}{
		{"unique", func(tb *schema.Table) { tb.Uniques[0].Deferrable = true }, []ChangeKind{DropUnique, AddUnique}},
		{"unique initially deferred", func(tb *schema.Table) { tb.Uniques[0].InitiallyDeferred = true }, []ChangeKind{DropUnique, AddUnique}},
		{"foreign key", func(tb *schema.Table) { tb.ForeignKeys[0].Deferrable = true }, []ChangeKind{DropFK, AddFK}},
		{"foreign key initially deferred", func(tb *schema.Table) { tb.ForeignKeys[0].InitiallyDeferred = true }, []ChangeKind{DropFK, AddFK}},
		{"primary key", func(tb *schema.Table) { tb.PrimaryKeyDeferrable = true }, []ChangeKind{DropPrimaryKey, AddPrimaryKey}},
		{"primary key initially deferred", func(tb *schema.Table) { tb.PrimaryKeyInitiallyDeferred = true }, []ChangeKind{DropPrimaryKey, AddPrimaryKey}},
	}
	for _, c := range cases {
		changes, _, err := Diff(stopsSchema(nil), stopsSchema(c.mutate), DiffOptions{AllowDestructive: true})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := kinds(changes); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: changes = %v, want %v", c.name, got, c.want)
		}
		back, _, err := Diff(stopsSchema(c.mutate), stopsSchema(nil), DiffOptions{AllowDestructive: true})
		if err != nil || !reflect.DeepEqual(kinds(back), c.want) {
			t.Errorf("%s: the change back = %v, %v, want %v", c.name, kinds(back), err, c.want)
		}
	}
}

// Catches: the added constraint carrying no deferrability, so the rendered
// ADD CONSTRAINT is plain after a change to deferrable.
func TestDiffCarriesDeferrabilityOnTheAdd(t *testing.T) {
	to := stopsSchema(func(tb *schema.Table) {
		tb.PrimaryKeyDeferrable, tb.PrimaryKeyInitiallyDeferred = true, true
		tb.Uniques[0].Deferrable, tb.Uniques[0].InitiallyDeferred = true, true
		tb.ForeignKeys[0].Deferrable = true
	})
	changes, _, err := Diff(stopsSchema(nil), to, DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range changes {
		switch c.Kind {
		case AddPrimaryKey:
			if !c.Deferrable || !c.InitiallyDeferred {
				t.Errorf("add_primary_key carries deferrable %v, initially deferred %v", c.Deferrable, c.InitiallyDeferred)
			}
		case AddUnique:
			if u := c.After.(*schema.Unique); !u.Deferrable || !u.InitiallyDeferred {
				t.Errorf("add_unique carries %+v", u)
			}
		case AddFK:
			if f := c.After.(*schema.ForeignKey); !f.Deferrable || f.InitiallyDeferred {
				t.Errorf("add_fk carries %+v", f)
			}
		}
	}
}

// Catches: two spellings of one constraint (INITIALLY DEFERRED with and without
// DEFERRABLE) showing up as a change, and an unchanged deferrable schema
// producing changes.
func TestDiffIgnoresTheSpellingOfDeferrability(t *testing.T) {
	spelled := stopsSchema(func(tb *schema.Table) {
		tb.Uniques[0].Deferrable, tb.Uniques[0].InitiallyDeferred = true, true
		tb.PrimaryKeyDeferrable, tb.PrimaryKeyInitiallyDeferred = true, true
	})
	implied := stopsSchema(func(tb *schema.Table) {
		tb.Uniques[0].InitiallyDeferred = true
		tb.PrimaryKeyInitiallyDeferred = true
	})
	for _, p := range [][2]*schema.Schema{{spelled, implied}, {implied, spelled}, {spelled, spelled}} {
		changes, _, err := Diff(p[0], p[1], DiffOptions{AllowDestructive: true})
		if err != nil || len(changes) != 0 {
			t.Errorf("changes = %v, %v, want none", kinds(changes), err)
		}
	}
}
