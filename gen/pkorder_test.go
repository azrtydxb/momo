// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"slices"
	"strings"
	"testing"
)

// Catches: the key being declared in field order, so a table whose key is
// (name, scope, at) over columns (at, scope, name, value) would differ from
// the database on every baseline.
func TestPrimaryKeyOrderFromConfig(t *testing.T) {
	g, diags, err := Load([]string{"./pkorder"}, Config{Dir: fixturesDir(t)})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	s, ds := BuildSchema(g.Models, pg())
	if len(ds) != 0 {
		t.Fatalf("diagnostics: %v", ds)
	}
	tb, ok := s.Table("health_samples")
	if !ok {
		t.Fatalf("tables: %+v", s.Tables)
	}
	if want := []string{"name", "scope", "at"}; !slices.Equal(tb.PrimaryKey, want) {
		t.Fatalf("primary key = %v, want %v", tb.PrimaryKey, want)
	}
	if tb.Columns[0].Name != "at" {
		t.Fatalf("column order must stay declaration order: %+v", tb.Columns)
	}
}

// Catches: a PrimaryKey list that names a column outside the pk fields being
// accepted, which would silently declare a different key than the tags.
func TestPrimaryKeyOrderMustNameThePKFields(t *testing.T) {
	m := mdl("T", "ts", fld("A", "string", "pk"), fld("B", "string", "pk"), fld("C", "string", ""))
	m.Config.PrimaryKey = []string{"a", "c"}
	_, diags := build(t, pg(), m)
	if !strings.Contains(sdiagsString(diags), "PrimaryKey lists (a, c)") {
		t.Fatalf("diags: %v", diags)
	}
}
