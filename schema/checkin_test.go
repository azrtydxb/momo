// SPDX-License-Identifier: Apache-2.0

package schema_test

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/schemautil"
	"github.com/azrtydxb/momo/schema"
)

// Catches: a check that ignores its value list when rendering, and one that
// keeps the stale expression.
func TestCheckExprFor(t *testing.T) {
	plain := schema.Check{Name: "c", Expr: "qty > 0"}
	if got := plain.ExprFor(dialect.Postgres); got != "qty > 0" {
		t.Errorf("a plain check renders %q", got)
	}
	in := schema.Check{Name: "c", Expr: "stale", In: &schema.CheckIn{Column: "kind", Values: []string{"b", "a"}}}
	if got := in.ExprFor(dialect.Postgres); got != `"kind" IN ('a','b')` {
		t.Errorf("a value list renders %q", got)
	}
}

// Catches: a value list accepted that no database can write portably.
func TestCheckInValidate(t *testing.T) {
	good := schema.CheckIn{Column: "kind", Values: []string{"a", "it's", "ünï"}}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, in := range map[string]schema.CheckIn{
		"no column":  {Values: []string{"a"}},
		"no values":  {Column: "kind"},
		"backslash":  {Column: "kind", Values: []string{`a\b`}},
		"newline":    {Column: "kind", Values: []string{"a\nb"}},
		"nul":        {Column: "kind", Values: []string{"a\x00b"}},
		"tab inside": {Column: "kind", Values: []string{"a\tb"}},
	} {
		if err := in.Validate(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

// Catches: Sort leaving the values in source order, so that two snapshots of
// one set differ, and a snapshot written with an "in" key when the check has no
// value list, which would change every existing snapshot.
func TestCheckInSortAndJSON(t *testing.T) {
	s := &schema.Schema{Dialect: dialect.Postgres, Tables: []schema.Table{{
		Name:    "notes",
		Columns: []schema.Column{{Name: "kind", Type: "text"}},
		Checks: []schema.Check{
			{Name: "a_plain", Expr: "kind <> ''"},
			{Name: "b_list", Expr: "x", In: &schema.CheckIn{Column: "kind", Values: []string{"c", "a", "c", "b"}}},
		},
	}}}
	s.Sort()
	if got := s.Tables[0].Checks[1].In.Values; !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("values = %v, want sorted and de-duplicated", got)
	}
	var buf bytes.Buffer
	if err := s.Write(&buf); err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Tables []struct {
			Checks []map[string]json.RawMessage `json:"checks"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	checks := raw.Tables[0].Checks
	if _, has := checks[0]["in"]; has {
		t.Errorf("a plain check carries an in key: %s", buf.String())
	}
	if !strings.Contains(string(checks[1]["in"]), `"column": "kind"`) && !strings.Contains(string(checks[1]["in"]), `"column":"kind"`) {
		t.Errorf("the value list is not in the snapshot: %s", buf.String())
	}
}

// Catches: ValueList taking the list from the wrong place: it must be In when
// set, else read from the expression, and nothing for an expression that is
// not a value list (a NULL element, another operator).
func TestCheckValueList(t *testing.T) {
	got, ok := schema.Check{Expr: "x IN ('z')", In: &schema.CheckIn{Column: "kind", Values: []string{"a", "b"}}}.ValueList()
	if !ok || got.Column != "kind" || !slices.Equal(got.Values, []string{"a", "b"}) {
		t.Errorf("In set: %+v, %v; want it taken from In", got, ok)
	}
	got, ok = schema.Check{Expr: "(status = ANY (ARRAY['b'::text, 'it''s'::text]))"}.ValueList()
	if !ok || got.Column != "status" || !slices.Equal(got.Values, []string{"b", "it's"}) {
		t.Errorf("read from Expr: %+v, %v", got, ok)
	}
	for _, expr := range []string{"(status = ANY (ARRAY['a'::text, NULL::text]))", "qty > 0", "status IN ('a', 1)", ""} {
		if got, ok := (schema.Check{Expr: expr}).ValueList(); ok {
			t.Errorf("%q is no value list, got %+v", expr, got)
		}
	}
	if _, ok := (schema.Check{Expr: "x > 0", In: &schema.CheckIn{Column: "x"}}).ValueList(); ok {
		t.Error("an In without values must not count as a list")
	}
}

// Catches: Equal comparing the text of a value-list check, so a declared
// CheckIn and the same constraint as each database reports it differ.
func TestEqualComparesCheckInAsASet(t *testing.T) {
	table := func(d dialect.Name, c schema.Check) *schema.Schema {
		return &schema.Schema{Dialect: d, Tables: []schema.Table{{
			Name:    "notes",
			Columns: []schema.Column{{Name: "status", Type: "text"}},
			Checks:  []schema.Check{c},
		}}}
	}
	declared := func(d dialect.Name, values ...string) schema.Check {
		return schema.Check{Name: "notes_status_check", Expr: schemautil.CheckInExpr(d, "status", values), In: &schema.CheckIn{Column: "status", Values: values}}
	}
	read := func(expr string) schema.Check { return schema.Check{Name: "notes_status_check", Expr: expr} }
	cases := []struct {
		name     string
		d        dialect.Name
		declared schema.Check
		inspect  schema.Check
		want     bool
	}{
		{"postgres any array", dialect.Postgres, declared(dialect.Postgres, "a", "b"), read("(status = ANY (ARRAY['a'::text, 'b'::text]))"), true},
		{"postgres other order", dialect.Postgres, declared(dialect.Postgres, "b", "a"), read("(status = ANY (ARRAY['a'::text, 'b'::text]))"), true},
		{"postgres one value", dialect.Postgres, declared(dialect.Postgres, "a"), read("(status = 'a'::text)"), true},
		{"postgres extra value", dialect.Postgres, declared(dialect.Postgres, "a", "b", "c"), read("(status = ANY (ARRAY['a'::text, 'b'::text]))"), false},
		{"postgres other column", dialect.Postgres, declared(dialect.Postgres, "a", "b"), read("(kind = ANY (ARRAY['a'::text, 'b'::text]))"), false},
		{"mysql introducers", dialect.MySQL, declared(dialect.MySQL, "a", "b"), read("(`status` in (_utf8mb4\\'a\\',_utf8mb4\\'b\\'))"), true},
		{"mariadb in list", dialect.MySQL, declared(dialect.MySQL, "a", "b"), read("`status` in ('a','b')"), true},
		{"mysql extra value", dialect.MySQL, declared(dialect.MySQL, "a", "b", "c"), read("(`status` in (_utf8mb4\\'a\\',_utf8mb4\\'b\\'))"), false},
		{"sqlite list", dialect.SQLite, declared(dialect.SQLite, "a", "b"), schema.Check{Name: "notes_status_check", Expr: `"status" IN ('a','b')`, In: &schema.CheckIn{Column: "status", Values: []string{"b", "a"}}}, true},
		{"sqlite other value", dialect.SQLite, declared(dialect.SQLite, "a", "b"), schema.Check{Name: "notes_status_check", Expr: `"status" IN ('a','c')`, In: &schema.CheckIn{Column: "status", Values: []string{"a", "c"}}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, b := table(c.d, c.declared), table(c.d, c.inspect)
			if got := schema.Equal(a, b); got != c.want {
				t.Errorf("Equal(declared, inspected) = %v, want %v", got, c.want)
			}
			if got := schema.Equal(b, a); got != c.want {
				t.Errorf("Equal(inspected, declared) = %v, want %v", got, c.want)
			}
		})
	}
}
