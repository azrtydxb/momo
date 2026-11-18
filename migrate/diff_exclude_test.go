// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/schema"
)

func bookingsSchema(mutate func(*schema.Table)) *schema.Schema {
	t := schema.Table{
		Name: "bookings",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint"},
			{Name: "room_id", Type: "integer"},
			{Name: "during", Type: "tstzrange"},
		},
		PrimaryKey: []string{"id"},
		Excludes: []schema.Exclude{{
			Name:     "bookings_no_overlap",
			Elements: []schema.ExcludeElement{{Column: "room_id", Operator: "="}, {Column: "during", Operator: "&&"}},
		}},
	}
	if mutate != nil {
		mutate(&t)
	}
	return &schema.Schema{Dialect: "postgres", Extensions: []schema.Extension{{Name: "btree_gist"}}, Tables: []schema.Table{t}}
}

// Catches: an exclusion constraint added, dropped or changed that Diff does not
// see, or sees as a different object, or that drops the old one and forgets the
// add, so the table ends up without the rule.
func TestDiffExcludeConstraints(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*schema.Table)
		want   []ChangeKind
	}{
		{"operator", func(tb *schema.Table) { tb.Excludes[0].Elements[1].Operator = "=" }, []ChangeKind{DropExclude, AddExclude}},
		{"column", func(tb *schema.Table) { tb.Excludes[0].Elements[0].Column = "id" }, []ChangeKind{DropExclude, AddExclude}},
		{"fewer elements", func(tb *schema.Table) { tb.Excludes[0].Elements = tb.Excludes[0].Elements[1:] }, []ChangeKind{DropExclude, AddExclude}},
		{"method", func(tb *schema.Table) { tb.Excludes[0].Method = "spgist" }, []ChangeKind{DropExclude, AddExclude}},
		{"deferrable", func(tb *schema.Table) { tb.Excludes[0].Deferrable = true }, []ChangeKind{DropExclude, AddExclude}},
		{"initially deferred", func(tb *schema.Table) { tb.Excludes[0].InitiallyDeferred = true }, []ChangeKind{DropExclude, AddExclude}},
		{"renamed", func(tb *schema.Table) { tb.Excludes[0].Name = "bookings_other" }, []ChangeKind{DropExclude, AddExclude}},
	}
	for _, c := range cases {
		changes, _, err := Diff(bookingsSchema(nil), bookingsSchema(c.mutate), DiffOptions{AllowDestructive: true})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := kinds(changes); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: changes = %v, want %v", c.name, got, c.want)
		}
	}
	none := func(tb *schema.Table) { tb.Excludes = nil }
	added, _, err := Diff(bookingsSchema(none), bookingsSchema(nil), DiffOptions{})
	if err != nil || !reflect.DeepEqual(kinds(added), []ChangeKind{AddExclude}) {
		t.Errorf("add = %v, %v", kinds(added), err)
	}
	if len(added) == 1 && (!added[0].Risky || added[0].Name != "bookings_no_overlap" || added[0].Table != "bookings") {
		t.Errorf("the add of a constraint to an existing table is not risky or not named: %+v", added[0])
	}
	dropped, _, err := Diff(bookingsSchema(nil), bookingsSchema(none), DiffOptions{})
	if err != nil || !reflect.DeepEqual(kinds(dropped), []ChangeKind{DropExclude}) {
		t.Errorf("drop = %v, %v", kinds(dropped), err)
	}
}

// Catches: the same constraint spelled two ways (an empty access method and
// gist, INITIALLY DEFERRED with and without DEFERRABLE) showing up as a change,
// which would make every Baseline and drift of a table built from a model
// report the constraint.
func TestDiffIgnoresTheSpellingOfAnExclusion(t *testing.T) {
	a := bookingsSchema(func(tb *schema.Table) {
		tb.Excludes[0].Method = "gist"
		tb.Excludes[0].Deferrable, tb.Excludes[0].InitiallyDeferred = true, true
	})
	b := bookingsSchema(func(tb *schema.Table) { tb.Excludes[0].InitiallyDeferred = true })
	c := bookingsSchema(func(tb *schema.Table) {
		tb.Excludes[0].Method = "GiST"
		tb.Excludes[0].InitiallyDeferred = true
	})
	for _, p := range [][2]*schema.Schema{{a, b}, {b, a}, {a, c}, {b, c}} {
		changes, _, err := Diff(p[0], p[1], DiffOptions{AllowDestructive: true})
		if err != nil || len(changes) != 0 {
			t.Errorf("changes = %v, %v, want none", kinds(changes), err)
		}
		if !schema.Equal(p[0], p[1]) {
			t.Error("schema.Equal reports two spellings of one constraint as different")
		}
	}
	if schema.Equal(bookingsSchema(nil), bookingsSchema(func(tb *schema.Table) { tb.Excludes[0].Elements[1].Operator = "=" })) {
		t.Error("schema.Equal ignores a change of operator")
	}
}

// Catches: a renamed column leaving the constraint that names it behind as a
// drop and an add, which would rebuild a GiST index for a rename.
func TestDiffExcludeFollowsAColumnRename(t *testing.T) {
	to := bookingsSchema(func(tb *schema.Table) {
		tb.Columns[1].Name = "venue_id"
		tb.Excludes[0].Elements[0].Column = "venue_id"
	})
	changes, _, err := Diff(bookingsSchema(nil), to, DiffOptions{AllowDestructive: true, RenameColumns: map[string]string{"bookings.room_id": "venue_id"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range changes {
		if c.Kind == DropExclude || c.Kind == AddExclude {
			t.Errorf("a column rename re-created the exclusion constraint: %v", kinds(changes))
		}
	}
}

// Catches: a snapshot that carries an exclusion constraint losing it on the way
// through JSON, and an old snapshot (no excludes field) not loading unchanged.
func TestSnapshotExcludeCompatibility(t *testing.T) {
	raw, err := json.Marshal(bookingsSchema(nil))
	if err != nil {
		t.Fatal(err)
	}
	var back schema.Schema
	if err := json.Unmarshal(raw, &back); err != nil || !schema.Equal(&back, bookingsSchema(nil)) {
		t.Fatalf("round trip: %v\n%s", err, raw)
	}
	for _, want := range []string{`"excludes"`, `"operator":"&&"`, `"column":"during"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the snapshot lacks %s:\n%s", want, raw)
		}
	}
	plain, err := json.Marshal(bookingsSchema(func(tb *schema.Table) { tb.Excludes = nil }))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), "exclude") {
		t.Errorf("a table with no exclusion constraint writes a field for them (an old snapshot would change):\n%s", plain)
	}
}

// Catches: drift not listing an exclusion constraint as an object of its table,
// so a constraint dropped by hand, or added by hand, goes unreported.
func TestFlattenListsExclusionConstraints(t *testing.T) {
	objs := flatten(bookingsSchema(nil), true)
	o, ok := objs[key("exclude", "", "bookings.bookings_no_overlap")]
	if !ok {
		t.Fatalf("no exclude object among %d objects", len(objs))
	}
	if !o.child || o.parent != key("table", "", "bookings") {
		t.Errorf("the exclude object is %+v, want a child of the table", o)
	}
	if tbl := objs[key("table", "", "bookings")]; strings.Contains(tbl.body, "excludes") {
		t.Errorf("the table object also carries the constraint, so a change would be reported twice: %s", tbl.body)
	}
	if !covers([]Change{{Kind: DropExclude, Table: "bookings", Name: "bookings_no_overlap"}}, o.ref) {
		t.Error("a drop_exclude change does not cover the exclude object it repairs")
	}
}

func TestExcludeChangesAreOrdered(t *testing.T) {
	changes := []Change{
		{Kind: AddExclude, Table: "bookings", Name: "x"},
		{Kind: DropExclude, Table: "bookings", Name: "x"},
	}
	ordered, err := OrderErr(changes)
	if err != nil {
		t.Fatal(err)
	}
	if got := kinds(ordered); !reflect.DeepEqual(got, []ChangeKind{DropExclude, AddExclude}) {
		t.Errorf("order = %v, want the drop before the add", got)
	}
}
