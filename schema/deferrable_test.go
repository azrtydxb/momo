// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"bytes"
	"strings"
	"testing"
)

func deferrableTable() Table {
	cols := []Column{
		{Name: "id", Type: "bigint"},
		{Name: "route_id", Type: "bigint"},
		{Name: "position", Type: "integer"},
	}
	t := Table{Name: "stops", Columns: cols, PrimaryKey: []string{"id"}}
	t.PrimaryKeyDeferrable, t.PrimaryKeyInitiallyDeferred = true, true
	t.Uniques = []Unique{{Name: "stops_route_position_key", Columns: []string{"route_id", "position"}, Deferrable: true, InitiallyDeferred: true}}
	t.ForeignKeys = []ForeignKey{{Name: "stops_route_id_fkey", Columns: []string{"route_id"}, RefTable: "routes", RefColumns: []string{"id"}, Deferrable: true}}
	return t
}

// Catches: a snapshot that loses the deferrability of a constraint when it is
// written and read back, so a regenerated migration drops DEFERRABLE.
func TestSnapshotRoundTripKeepsDeferrability(t *testing.T) {
	in := &Schema{Dialect: "postgres", Tables: []Table{deferrableTable()}}
	var buf bytes.Buffer
	if err := in.Write(&buf); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"deferrable": true`, `"initially_deferred": true`, `"primary_key_deferrable": true`, `"primary_key_initially_deferred": true`} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("the snapshot lacks %s:\n%s", want, buf.String())
		}
	}
	if in.FormatVersion != 1 {
		t.Errorf("format version = %d, want 1: the flags are optional fields", in.FormatVersion)
	}
	out, err := Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	tb := out.Tables[0]
	if !tb.PrimaryKeyDeferrable || !tb.PrimaryKeyInitiallyDeferred {
		t.Errorf("primary key flags = %v, %v, want both set", tb.PrimaryKeyDeferrable, tb.PrimaryKeyInitiallyDeferred)
	}
	if u := tb.Uniques[0]; !u.Deferrable || !u.InitiallyDeferred {
		t.Errorf("unique flags = %v, %v, want both set", u.Deferrable, u.InitiallyDeferred)
	}
	if f := tb.ForeignKeys[0]; !f.Deferrable || f.InitiallyDeferred {
		t.Errorf("foreign key flags = %v, %v, want deferrable and not initially deferred", f.Deferrable, f.InitiallyDeferred)
	}
	if !Equal(in, out) {
		t.Error("the loaded snapshot differs from the written one")
	}
}

// Catches: a snapshot written before the flags existed failing to load, or
// loading as deferrable, which would make every older migration directory
// report drift against a database that was never deferrable.
func TestOldSnapshotLoadsAsNotDeferrable(t *testing.T) {
	const old = `{
  "format_version": 1,
  "dialect": "postgres",
  "tables": [{
    "name": "stops",
    "columns": [{"name": "id", "type": "bigint"}, {"name": "route_id", "type": "bigint"}],
    "primary_key": ["id"],
    "uniques": [{"name": "stops_route_key", "columns": ["route_id"]}],
    "foreign_keys": [{"name": "stops_route_id_fkey", "columns": ["route_id"], "ref_table": "routes", "ref_columns": ["id"]}]
  }]
}`
	s, err := Load(strings.NewReader(old))
	if err != nil {
		t.Fatal(err)
	}
	tb := s.Tables[0]
	if tb.PrimaryKeyDeferrable || tb.PrimaryKeyInitiallyDeferred || tb.Uniques[0].Deferrable || tb.Uniques[0].InitiallyDeferred || tb.ForeignKeys[0].Deferrable || tb.ForeignKeys[0].InitiallyDeferred {
		t.Fatalf("an old snapshot loaded as deferrable: %+v", tb)
	}
	plain := &Schema{Dialect: "postgres", Tables: []Table{{
		Name:        "stops",
		Columns:     []Column{{Name: "id", Type: "bigint"}, {Name: "route_id", Type: "bigint"}},
		PrimaryKey:  []string{"id"},
		Uniques:     []Unique{{Name: "stops_route_key", Columns: []string{"route_id"}}},
		ForeignKeys: []ForeignKey{{Name: "stops_route_id_fkey", Columns: []string{"route_id"}, RefTable: "routes", RefColumns: []string{"id"}}},
	}}}
	if !Equal(s, plain) {
		t.Error("an old snapshot differs from the same schema without deferrability")
	}
	var buf bytes.Buffer
	if err := s.Write(&buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "defer") {
		t.Errorf("a plain constraint writes a deferrability field:\n%s", buf.String())
	}
}

// Catches: deferrability ignored by Equal, so a database whose constraint is
// not deferrable compares equal to a model that says it is; and the two
// spellings of an initially deferred constraint comparing different.
func TestEqualComparesDeferrability(t *testing.T) {
	base := func() *Schema { return &Schema{Dialect: "postgres", Tables: []Table{deferrableTable()}} }
	if !Equal(base(), base()) {
		t.Fatal("a schema differs from itself")
	}
	for name, mutate := range map[string]func(*Table){
		"primary key":         func(t *Table) { t.PrimaryKeyDeferrable, t.PrimaryKeyInitiallyDeferred = false, false },
		"primary key initial": func(t *Table) { t.PrimaryKeyInitiallyDeferred = false },
		"unique":              func(t *Table) { t.Uniques[0].Deferrable, t.Uniques[0].InitiallyDeferred = false, false },
		"unique initial":      func(t *Table) { t.Uniques[0].InitiallyDeferred = false },
		"foreign key":         func(t *Table) { t.ForeignKeys[0].Deferrable = false },
		"foreign key initial": func(t *Table) { t.ForeignKeys[0].InitiallyDeferred = true },
	} {
		other := base()
		mutate(&other.Tables[0])
		if Equal(base(), other) {
			t.Errorf("%s: a change of deferrability is not a difference", name)
		}
	}

	// INITIALLY DEFERRED implies DEFERRABLE: either spelling is one constraint.
	implied, spelled := base(), base()
	implied.Tables[0].Uniques[0].Deferrable = false
	implied.Tables[0].PrimaryKeyDeferrable = false
	if !Equal(implied, spelled) {
		t.Error("an initially deferred constraint without the deferrable flag differs from the one with it")
	}
	n := Normalized(implied).Tables[0]
	if !n.Uniques[0].Deferrable || !n.PrimaryKeyDeferrable {
		t.Errorf("Normalized did not set deferrable on initially deferred constraints: %+v", n)
	}
}
