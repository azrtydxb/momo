// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// TestCompanionDeferrable is issue 237: Config.Unique, Config.ForeignKey and the
// primary key calls declare DEFERRABLE and INITIALLY DEFERRED, and the schema
// carries them.
//
// Catches: a flag lost between the companion reader and the schema (the
// generated migration is plain and a fresh install fails the reorder), the
// implied DEFERRABLE of INITIALLY DEFERRED not being set, and a deferrable
// unique turned into an index.
func TestCompanionDeferrable(t *testing.T) {
	g, diags, err := Load([]string{"./companiondeferrable"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	s, diags := BuildSchema(g.Models, pg())
	if len(diags) != 0 {
		t.Fatalf("schema diagnostics: %v", diags)
	}
	stops, _ := s.Table("stops")
	if !stops.PrimaryKeyDeferrable || stops.PrimaryKeyInitiallyDeferred {
		t.Errorf("stops primary key = deferrable %v, initially %v, want deferrable only", stops.PrimaryKeyDeferrable, stops.PrimaryKeyInitiallyDeferred)
	}
	if len(stops.Indexes) != 0 {
		t.Errorf("a deferrable unique became an index: %+v", stops.Indexes)
	}
	want := []schema.Unique{
		{Name: "stops_position_key", Columns: []string{"position"}},
		{Name: "stops_route_key", Columns: []string{"route_id"}, Deferrable: true},
		{Name: "stops_route_position_key", Columns: []string{"route_id", "position"}, Deferrable: true, InitiallyDeferred: true},
	}
	if !slices.EqualFunc(stops.Uniques, want, func(a, b schema.Unique) bool {
		return a.Name == b.Name && slices.Equal(a.Columns, b.Columns) && a.Deferrable == b.Deferrable && a.InitiallyDeferred == b.InitiallyDeferred
	}) {
		t.Errorf("stops uniques = %+v, want %+v", stops.Uniques, want)
	}
	if len(stops.ForeignKeys) != 1 {
		t.Fatalf("stops foreign keys = %+v", stops.ForeignKeys)
	}
	if fk := stops.ForeignKeys[0]; !fk.Deferrable || fk.InitiallyDeferred || fk.OnDelete != "CASCADE" {
		t.Errorf("stops foreign key = %+v, want deferrable with ON DELETE CASCADE", fk)
	}
	depots, _ := s.Table("depots")
	if !depots.PrimaryKeyDeferrable || !depots.PrimaryKeyInitiallyDeferred {
		t.Errorf("depots primary key = deferrable %v, initially %v, want both", depots.PrimaryKeyDeferrable, depots.PrimaryKeyInitiallyDeferred)
	}
}

// Catches: a deferrable constraint reaching a dialect that cannot defer it: the
// migration would be refused late, by the renderer, instead of the generator
// naming the model.
func TestCompanionDeferrableRefusedOutsidePostgres(t *testing.T) {
	g, _, err := Load([]string{"./companiondeferrable"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		_, diags := BuildSchema(g.Models, Config{Dialects: []dialect.Name{d}})
		joined := diagStrings(diags)
		if !strings.Contains(joined, "is deferrable, which is not supported on "+string(d)) {
			t.Errorf("%s: no diagnostic for the deferrable constraints:\n%s", d, joined)
		}
		for _, name := range []string{"the primary key", "unique stops_route_position_key", "foreign key stops_route_id_fkey"} {
			if !strings.Contains(joined, name) {
				t.Errorf("%s: the diagnostic does not name %q:\n%s", d, name, joined)
			}
		}
	}
	_, diags := BuildSchema(g.Models, Config{Dialects: []dialect.Name{dialect.Postgres, dialect.SQLite}})
	if joined := diagStrings(diags); !strings.Contains(joined, "not supported on sqlite") || strings.Contains(joined, "not supported on postgres") {
		t.Errorf("postgres and sqlite: want a diagnostic for sqlite only:\n%s", joined)
	}
}

// Catches: a deferrable index or a unique constraint with options accepted
// (PostgreSQL cannot defer an index, so the migration would fail), a repeated
// option, and a deferrable primary key on a model that has none.
func TestCompanionDeferrableRefusals(t *testing.T) {
	_, diags, err := Load([]string{"./companiondeferrablebad"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	joined := diagStrings(diags)
	for _, want := range []string{
		`index "a_name_idx": only a unique constraint, declared with Config.Unique, can be deferrable`,
		`unique "a_name_key": a deferrable unique constraint cannot take other options`,
		`duplicate Deferrable on index "a_dup_key"`,
		`duplicate PrimaryKeyDeferrable call`,
		// A second call among the two, in either order, is a repeat.
		`duplicate PrimaryKeyInitiallyDeferred call`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing diagnostic %q in:\n%s", want, joined)
		}
	}

	// Model A repeats PrimaryKeyDeferrable once, and model C calls
	// PrimaryKeyDeferrable after PrimaryKeyInitiallyDeferred: both are repeats.
	if n := strings.Count(joined, "duplicate PrimaryKeyDeferrable call"); n != 2 {
		t.Errorf("duplicate PrimaryKeyDeferrable call reported %d times, want 2 (either call order):\n%s", n, joined)
	}

	// The schema builder reports the key, after the companion has been read.
	_, diags, err = Load([]string{"./companiondeferrablenopk"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	if joined := diagStrings(diags); !strings.Contains(joined, `PrimaryKeyDeferrable: model B has no primary key`) {
		t.Errorf("a deferrable primary key on a model without one is not refused:\n%s", joined)
	}
}

// Catches: a foreign key that references a deferrable primary key (belongs_to)
// or a deferrable unique constraint (Config.ForeignKey) passing the generator.
// PostgreSQL refuses both as a foreign key target, so the migration would fail
// at apply time instead of the generator naming the key.
func TestBuildSchemaRefusesForeignKeyToDeferrableKey(t *testing.T) {
	// Load builds the schema, so its diagnostics carry the refusal.
	_, diags, err := Load([]string{"./companiondeferrablefk"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	joined := diagStrings(diags)
	for _, want := range []string{
		"foreign key children_parent_id_fkey references (id) of parents, which is the primary key and deferrable",
		"foreign key children_code_fkey references (code) of slots, which is unique slots_code_key and deferrable",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing diagnostic %q in:\n%s", want, joined)
		}
	}
}

// Catches: a refusal that fires when a non-deferrable key covers the same
// columns (PostgreSQL uses that key), and one that misses a deferrable key
// whose columns are listed in another order.
func TestDeferrableKeyOf(t *testing.T) {
	cases := []struct {
		name string
		tbl  schema.Table
		cols []string
		want string
	}{
		{"plain primary key", schema.Table{PrimaryKey: []string{"id"}}, []string{"id"}, ""},
		{"deferrable primary key", schema.Table{PrimaryKey: []string{"id"}, PrimaryKeyDeferrable: true}, []string{"id"}, "the primary key"},
		{"initially deferred primary key", schema.Table{PrimaryKey: []string{"id"}, PrimaryKeyInitiallyDeferred: true}, []string{"id"}, "the primary key"},
		{"deferrable unique, other order", schema.Table{Uniques: []schema.Unique{{Name: "u", Columns: []string{"a", "b"}, Deferrable: true}}}, []string{"b", "a"}, "unique u"},
		{"other columns", schema.Table{Uniques: []schema.Unique{{Name: "u", Columns: []string{"a"}, Deferrable: true}}}, []string{"b"}, ""},
		{"covered by a plain unique too", schema.Table{Uniques: []schema.Unique{{Name: "u", Columns: []string{"a"}, Deferrable: true}, {Name: "v", Columns: []string{"a"}}}}, []string{"a"}, ""},
		{"covered by a unique index too", schema.Table{
			Uniques: []schema.Unique{{Name: "u", Columns: []string{"a"}, Deferrable: true}},
			Indexes: []schema.Index{{Name: "i", Unique: true, Parts: []schema.IndexPart{{Column: "a"}}}},
		}, []string{"a"}, ""},
		{"partial unique index does not cover", schema.Table{
			Uniques: []schema.Unique{{Name: "u", Columns: []string{"a"}, Deferrable: true}},
			Indexes: []schema.Index{{Name: "i", Unique: true, Where: "a > 0", Parts: []schema.IndexPart{{Column: "a"}}}},
		}, []string{"a"}, "unique u"},
	}
	for _, c := range cases {
		if got := deferrableKeyOf(&c.tbl, c.cols); got != c.want {
			t.Errorf("%s: deferrableKeyOf = %q, want %q", c.name, got, c.want)
		}
	}
}
