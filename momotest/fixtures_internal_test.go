// SPDX-License-Identifier: Apache-2.0

package momotest

import (
	"slices"
	"strings"
	"testing"
	"time"
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
		{"nested map", "docs:\n  - id: 1\n    meta:\n      a: 1\n", "table docs row 1: column meta: a value must be a scalar"},
		{"nested list", "docs:\n  - id: 1\n    tags: [a, b]\n", "table docs row 1: column tags: a value must be a scalar"},
		{"bad binary", "docs:\n  - blob: !!binary \"!!!\"\n", "table docs row 1"},
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

// TestParseFixturesScalarTypes pins what each kind of YAML scalar becomes: the
// driver sees a float64, a bool, a time.Time and, for !!binary, the bytes.
func TestParseFixturesScalarTypes(t *testing.T) {
	rows, err := parseFixtures([]byte(`
docs:
  - b: true
    blob: !!binary aGk=
    f: 1.5
    i: 42
    n: null
    s: "42"
    t: 2026-01-02T03:04:05Z
`))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]any{}
	for i, c := range rows[0].cols {
		got[c] = rows[0].vals[i]
	}
	if got["b"] != true || got["f"] != 1.5 || got["i"] != 42 || got["n"] != nil || got["s"] != "42" {
		t.Fatalf("values = %#v", got)
	}
	if b, ok := got["blob"].([]byte); !ok || string(b) != "hi" {
		t.Fatalf("!!binary = %#v, want the bytes of hi", got["blob"])
	}
	if ts, ok := got["t"].(time.Time); !ok || !ts.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("timestamp = %#v, want a time.Time", got["t"])
	}
}

// TestParseFixturesDeterministic: the same file parses to the same rows in the
// same order every time. A parser that ranges over a map would not.
func TestParseFixturesDeterministic(t *testing.T) {
	const doc = "t1:\n  - b: 1\n    a: 2\n    c: 3\nt2:\n  - z: 1\n    y: 2\nt3:\n  - m: 1\n"
	first, err := parseFixtures([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		got, err := parseFixtures([]byte(doc))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(first) {
			t.Fatalf("run %d: %d rows, want %d", i, len(got), len(first))
		}
		for j := range got {
			if got[j].table != first[j].table || got[j].n != first[j].n || !slices.Equal(got[j].cols, first[j].cols) {
				t.Fatalf("run %d row %d: %+v differs from %+v", i, j, got[j], first[j])
			}
		}
	}
}
