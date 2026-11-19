// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

func extensionNames(s *schema.Schema) []string {
	var out []string
	for _, e := range s.Extensions {
		out = append(out, e.Name)
	}
	return out
}

// Catches: Config.Exclude not reaching the schema (the migration builds a
// table that accepts overlapping bookings), an element's operator or column
// lost, the deferral flags dropped, a range field mapped to the wrong range
// type (tsrange against tstzrange), and a GiST constraint over a scalar column
// that does not add btree_gist, so CREATE TABLE fails on a fresh database.
func TestCompanionExclude(t *testing.T) {
	g, diags, err := Load([]string{"./companionexclude"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	s, diags := BuildSchema(g.Models, pg())
	if len(diags) != 0 {
		t.Fatalf("schema diagnostics: %v", diags)
	}
	tb, ok := s.Table("bookings")
	if !ok {
		t.Fatal("no bookings table")
	}
	want := []schema.Exclude{{
		Name:              "bookings_no_overlap",
		Elements:          []schema.ExcludeElement{{Column: "room_id", Operator: "="}, {Column: "during", Operator: "&&"}},
		Deferrable:        true,
		InitiallyDeferred: true,
	}}
	if len(tb.Excludes) != 1 || tb.Excludes[0].Name != want[0].Name || !slices.Equal(tb.Excludes[0].Elements, want[0].Elements) ||
		tb.Excludes[0].Deferrable != want[0].Deferrable || tb.Excludes[0].InitiallyDeferred != want[0].InitiallyDeferred {
		t.Errorf("bookings excludes = %+v, want %+v", tb.Excludes, want)
	}
	if len(tb.Indexes) != 0 {
		t.Errorf("an exclusion constraint became an index: %+v", tb.Indexes)
	}
	wantTypes := map[string]string{"during": "tstzrange", "local": "tsrange", "days": "daterange", "qty": "int4range", "big": "int8range", "price": "numrange"}
	for col, typ := range wantTypes {
		if c := mustCol(t, s, "bookings", col); c.Type != typ {
			t.Errorf("bookings.%s type = %q, want %q", col, c.Type, typ)
		}
	}
	if !slices.Contains(extensionNames(s), "btree_gist") {
		t.Errorf("extensions = %v, want btree_gist for the GiST constraint over room_id", extensionNames(s))
	}
}

// Catches: btree_gist added for a constraint that only compares range columns,
// which would make a migration create an extension nothing needs (and fail on a
// server without it).
func TestCompanionExcludeOverRangesNeedsNoExtension(t *testing.T) {
	g, diags, err := Load([]string{"./companionexcluderange"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	s, diags := BuildSchema(g.Models, pg())
	if len(diags) != 0 {
		t.Fatalf("schema diagnostics: %v", diags)
	}
	if len(s.Extensions) != 0 {
		t.Errorf("extensions = %v, want none", extensionNames(s))
	}
	tb, _ := s.Table("shifts")
	if len(tb.Excludes) != 1 || tb.Excludes[0].Method != "gist" || !tb.Excludes[0].Deferrable || tb.Excludes[0].InitiallyDeferred {
		t.Errorf("shifts excludes = %+v, want gist and deferrable only", tb.Excludes)
	}
}

// Catches: an exclusion constraint or a range field reaching a dialect that has
// neither: the failure would come late, from the renderer, instead of the
// generator naming the model.
func TestCompanionExcludeRefusedOutsidePostgres(t *testing.T) {
	g, _, err := Load([]string{"./companionexclude"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		_, diags := BuildSchema(g.Models, Config{Dialects: []dialect.Name{d}})
		joined := diagStrings(diags)
		for _, want := range []string{
			"exclusion constraint bookings_no_overlap is not supported on " + string(d),
			"has no SQL mapping on " + string(d),
		} {
			if !strings.Contains(joined, want) {
				t.Errorf("%s: missing diagnostic %q in:\n%s", d, want, joined)
			}
		}
	}
}

// Catches: a malformed Config.Exclude accepted, which would write an operator
// into the DDL unquoted or build a constraint with half an element.
func TestCompanionExcludeRefusals(t *testing.T) {
	_, diags, err := Load([]string{"./companionexcludebad"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	joined := diagStrings(diags)
	for _, want := range []string{
		`exclusion constraint "a_bad_op": "&&; DROP TABLE a" is not an operator`,
		`Exclude takes a name followed by pairs of a field reference and an operator`,
		`exclusion constraint "a_bad_method": access method "brin" is not one of gist, spgist, btree, hash`,
		`duplicate Using on exclusion constraint "a_dup"`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing diagnostic %q in:\n%s", want, joined)
		}
	}
}

func TestGistNeedsBtreeGist(t *testing.T) {
	for typ, want := range map[string]bool{
		"integer": true, "bigint": true, "text": true, "uuid": true, "numeric(10,2)": true, "status": true,
		"tstzrange": false, "int4range": false, "datemultirange": false, "inet": false, "point": false, "integer[]": false,
		"": false,
	} {
		if got := gistNeedsBtreeGist(typ); got != want {
			t.Errorf("gistNeedsBtreeGist(%q) = %v, want %v", typ, got, want)
		}
	}
}
