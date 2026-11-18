// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"reflect"
	"strings"
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

// referencedSchema is stopsSchema with a table routes that has a unique code
// and is referenced by stops twice: by its primary key and by its code.
func referencedSchema(mutate func(routes *schema.Table)) *schema.Schema {
	s := stopsSchema(nil)
	routes := &s.Tables[0]
	routes.Columns = append(routes.Columns, schema.Column{Name: "code", Type: "text"})
	routes.Uniques = []schema.Unique{{Name: "routes_code_key", Columns: []string{"code"}}}
	stops := &s.Tables[1]
	stops.Columns = append(stops.Columns, schema.Column{Name: "route_code", Type: "text"})
	stops.ForeignKeys = append(stops.ForeignKeys, schema.ForeignKey{Name: "stops_route_code_fkey", Columns: []string{"route_code"}, RefTable: "routes", RefColumns: []string{"code"}})
	if mutate != nil {
		mutate(routes)
	}
	return s
}

// Catches: a foreign key to a deferrable primary key or unique constraint
// reaching a plan. PostgreSQL refuses it when the key is created or the foreign
// key is added, so the migration would fail half way through. It also covers
// the change of deferrability of a referenced key: the refusal comes before
// any plan, so no DROP CONSTRAINT of a key that a foreign key depends on, and
// no ADD of a deferrable one, is ever produced.
func TestDiffRefusesForeignKeyToDeferrableKey(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*schema.Table)
		want   string
	}{
		{"primary key", func(r *schema.Table) { r.PrimaryKeyDeferrable = true }, "the primary key and deferrable"},
		{"primary key initially deferred", func(r *schema.Table) { r.PrimaryKeyInitiallyDeferred = true }, "the primary key and deferrable"},
		{"unique", func(r *schema.Table) { r.Uniques[0].Deferrable = true }, "unique routes_code_key and deferrable"},
		{"unique initially deferred", func(r *schema.Table) { r.Uniques[0].InitiallyDeferred = true }, "unique routes_code_key and deferrable"},
	}
	for _, c := range cases {
		for _, from := range []*schema.Schema{{Dialect: "postgres"}, referencedSchema(nil)} {
			changes, _, err := Diff(from, referencedSchema(c.mutate), DiffOptions{AllowDestructive: true})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("%s: err = %v, want one containing %q", c.name, err, c.want)
			}
			if len(changes) != 0 {
				t.Errorf("%s: a refused schema produced a plan: %v", c.name, kinds(changes))
			}
		}
	}

	// A foreign key to a key that is not deferrable, and a deferrable key that
	// no foreign key references, are fine.
	for name, p := range map[string][2]*schema.Schema{
		"plain":            {referencedSchema(nil), referencedSchema(nil)},
		"unreferenced key": {referencedSchema(nil), stopsSchema(func(tb *schema.Table) { tb.PrimaryKeyDeferrable = true })},
	} {
		if _, _, err := Diff(p[0], p[1], DiffOptions{AllowDestructive: true}); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}

	// PostgreSQL uses a non-deferrable key over the same columns.
	covered := referencedSchema(func(r *schema.Table) {
		r.Uniques[0].Deferrable = true
		r.Uniques = append(r.Uniques, schema.Unique{Name: "routes_code_key2", Columns: []string{"code"}})
	})
	if _, _, err := Diff(&schema.Schema{Dialect: "postgres"}, covered, DiffOptions{}); err != nil {
		t.Errorf("a column set that a non-deferrable unique also covers was refused: %v", err)
	}
}

// Catches: the primary key flags of a table with no primary key being compared,
// so a flag with no key behind it produces a DROP and an ADD of a key that does
// not exist.
func TestDiffIgnoresPrimaryKeyFlagsWithoutKey(t *testing.T) {
	mk := func(deferrable bool) *schema.Schema {
		return &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{
			Name:                 "logs",
			Columns:              []schema.Column{{Name: "msg", Type: "text"}},
			PrimaryKeyDeferrable: deferrable,
		}}}
	}
	changes, _, err := Diff(mk(false), mk(true), DiffOptions{AllowDestructive: true})
	if err != nil || len(changes) != 0 {
		t.Errorf("changes = %v, %v, want none", kinds(changes), err)
	}
}
