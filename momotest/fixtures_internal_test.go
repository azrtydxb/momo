// SPDX-License-Identifier: Apache-2.0

package momotest

import (
	"slices"
	"strings"
	"testing"
)

// TestParseFixtures: tables and rows keep the file order, the columns of a row
// are sorted, and a YAML null is a nil value. Catches a parser that ranges over
// a map (random table order) and one that drops the null.
func TestParseFixtures(t *testing.T) {
	rows, err := parseFixtures([]byte(`
zeta:
  - id: 2
    name: b
  - id: 1
    name: a
alpha:
  - title: x
    author_id: null
    id: 7
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	order := []string{rows[0].table, rows[1].table, rows[2].table}
	if !slices.Equal(order, []string{"zeta", "zeta", "alpha"}) {
		t.Fatalf("table order = %v, want file order zeta zeta alpha", order)
	}
	if rows[0].vals[0] != 2 || rows[1].vals[0] != 1 || rows[1].n != 2 {
		t.Fatalf("row order = %v then %v (n=%d), want list order", rows[0].vals, rows[1].vals, rows[1].n)
	}
	last := rows[2]
	if !slices.Equal(last.cols, []string{"author_id", "id", "title"}) {
		t.Fatalf("columns = %v, want them sorted", last.cols)
	}
	if last.vals[0] != nil || last.vals[1] != 7 || last.vals[2] != "x" {
		t.Fatalf("values = %#v, want nil, 7, x", last.vals)
	}
}

func TestParseFixturesRejects(t *testing.T) {
	cases := []struct {
		name, doc, want string
	}{
		{"list at the top", "- 1\n- 2\n", "top level"},
		{"scalar at the top", "just text\n", "top level"},
		{"empty file", "", "top level"},
		{"table is a mapping", "docs:\n  id: 1\n", "table docs must be a list"},
		{"row is a scalar", "docs:\n  - 5\n", "table docs row 1"},
		{"empty row", "docs:\n  - id: 1\n  - {}\n", "table docs row 2: a row needs at least one column"},
		{"bad yaml", "docs: [\n", "yaml"},
	}
	for _, c := range cases {
		_, err := parseFixtures([]byte(c.doc))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to mention %q", c.name, err, c.want)
		}
	}
}

// TestInsertStatement: the statement has one placeholder per identifier and per
// value, in the order they appear, so nothing is spliced into the SQL.
func TestInsertStatement(t *testing.T) {
	sql, args := insertStatement(fixtureRow{table: "docs", n: 1, cols: []string{"a", "b"}, vals: []any{1, "x"}})
	if sql != "INSERT INTO ? (?, ?) VALUES (?, ?)" {
		t.Fatalf("sql = %q", sql)
	}
	if len(args) != 5 || args[3] != 1 || args[4] != "x" {
		t.Fatalf("args = %#v, want table, two columns, then the values", args)
	}
	// A value that looks like SQL stays a bound argument.
	sql, args = insertStatement(fixtureRow{table: "docs", n: 1, cols: []string{"a"}, vals: []any{"1); DROP TABLE docs; --"}})
	if strings.Contains(sql, "DROP") || len(args) != 3 {
		t.Fatalf("sql = %q args = %#v, want the text bound", sql, args)
	}
}
