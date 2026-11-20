// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/schemautil"
	"github.com/azrtydxb/momo/schema"
)

func checkInTable(t *testing.T, d dialect.Name) *schema.Table {
	t.Helper()
	g, diags, err := Load([]string{"./checkincall"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	s, sdiags := BuildSchema(g.Models, Config{Dialects: []dialect.Name{d}})
	if len(sdiags) != 0 {
		t.Fatalf("schema diagnostics: %v", sdiags)
	}
	tbl, ok := s.Table("notes")
	if !ok {
		t.Fatal("notes table missing")
	}
	return tbl
}

// Catches: a CheckIn declaration that is not read, a value list kept in source
// order, a default name that is not <table>_<column>_check, an Expr that is not
// the canonical rendering of the sorted values, and the Name option ignored.
func TestGenerateCheckIn(t *testing.T) {
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		tbl := checkInTable(t, d)
		if len(tbl.Checks) != 2 {
			t.Fatalf("%s: checks = %+v", d, tbl.Checks)
		}
		kind, tag := tbl.Checks[0], tbl.Checks[1]
		if kind.Name != "notes_kind_check" || kind.In == nil || kind.In.Column != "kind" {
			t.Fatalf("%s: kind check = %+v", d, kind)
		}
		if want := []string{"archived", "draft", "it's", "published"}; !slices.Equal(kind.In.Values, want) {
			t.Fatalf("%s: values = %v, want the sorted %v", d, kind.In.Values, want)
		}
		if want := schemautil.CheckInExpr(d, "kind", kind.In.Values); kind.Expr != want {
			t.Fatalf("%s: Expr = %q, want %q", d, kind.Expr, want)
		}
		if tag.Name != "notes_tag_in" || tag.In == nil || tag.In.Column != "tag" || !slices.Equal(tag.In.Values, []string{"x", "y"}) {
			t.Fatalf("%s: tag check = %+v", d, tag)
		}
	}
}

// Catches: a repeated value, an empty list, a non-string field, a name clash
// with a tag or a c.Check, and a list built at run time, each accepted
// silently.
func TestGenerateCheckInCallRejected(t *testing.T) {
	_, diags, err := Load([]string{"./checkinbad"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	text := diagStrings(diags)
	for _, want := range []string{
		`CheckIn repeats the value "a"`,
		"CheckIn lists no values",
		"CheckIn field &n.Count must be a string field, it is int64",
		`check name "clashes_kind_check" is already produced by the check tag on field Kind`,
		`check name "clashes_label_in" is already declared at`,
		"unsupported companion expression: c.CheckIn(&v.Kind, values...)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("diagnostics lack %q:\n%s", want, text)
		}
	}
}

// Catches: a value list on an enum column, a column that is not text, or a
// value no database can write portably reaching the schema.
func TestGenerateCheckInSchemaRejected(t *testing.T) {
	g, _, err := Load([]string{"./checkinbadschema"}, Config{Dir: fixturesDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	_, diags := BuildSchema(g.Models, Config{Dialects: []dialect.Name{dialect.Postgres}})
	text := diagStrings(diags)
	for _, want := range []string{
		`column "status" has the enum type "typed_status"`,
		`column "price" is numeric(10,2), not a text column`,
		"holds a backslash or a control character",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("diagnostics lack %q:\n%s", want, text)
		}
	}
}
