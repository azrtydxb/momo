// SPDX-License-Identifier: Apache-2.0

package schema_test

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// Catches: an expression spelling a database reports that is not read back as
// a value list, and an expression that only looks like one (an extra
// condition, a function, a negation, a number list) read as one.
func TestParseCheckIn(t *testing.T) {
	cases := []struct {
		name, expr, col string
		values          []string
		ok              bool
	}{
		{"plain", `kind IN ('a','b')`, "kind", []string{"a", "b"}, true},
		{"quoted spaced", `"kind" in ('a', 'b')`, "kind", []string{"a", "b"}, true},
		{"parenthesised", `(kind IN ('a'))`, "kind", []string{"a"}, true},
		{"mysql", "(`kind` in (_utf8mb4'a',_utf8mb4'b'))", "kind", []string{"a", "b"}, true},
		{"mysql escaped quotes", "(`kind` in (_utf8mb4\\'a\\',_utf8mb4\\'b\\'))", "kind", []string{"a", "b"}, true},
		{"postgres text", `(kind = ANY (ARRAY['a'::text, 'b'::text]))`, "kind", []string{"a", "b"}, true},
		{"postgres varchar", `((kind)::text = ANY ((ARRAY['a'::character varying, 'b'::character varying])::text[]))`, "kind", []string{"a", "b"}, true},
		{"escaped quote", `kind IN ('it''s')`, "kind", []string{"it's"}, true},
		{"extra condition", `kind IN ('a') AND qty > 0`, "", nil, false},
		{"function", `lower(kind) IN ('a')`, "", nil, false},
		{"negation", `kind NOT IN ('a')`, "", nil, false},
		{"numbers", `qty IN (1, 2)`, "", nil, false},
		{"one value equality", `kind = 'a'`, "kind", []string{"a"}, true},
		{"one value postgres", `(kind = 'a'::text)`, "kind", []string{"a"}, true},
		{"one value postgres varchar", `((kind)::text = 'a'::text)`, "kind", []string{"a"}, true},
		{"one value mysql", "(`kind` = _utf8mb4\\'a\\')", "kind", []string{"a"}, true},
		{"equality with a list", `kind = 'a', 'b'`, "", nil, false},
		{"equality with a number", `qty = 1`, "", nil, false},
		{"backslash", `kind IN ('a\b')`, "", nil, false},
		{"trailing comma", `kind IN ('a',)`, "", nil, false},
		{"empty", `kind IN ()`, "", nil, false},
		{"unterminated", `kind IN ('a`, "", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			col, values, ok := schema.ParseCheckIn(c.expr)
			if ok != c.ok || col != c.col || !slices.Equal(values, c.values) {
				t.Fatalf("ParseCheckIn(%q) = %q, %v, %v; want %q, %v, %v", c.expr, col, values, ok, c.col, c.values, c.ok)
			}
		})
	}
}

// Catches: a rendering that depends on the order of the values, repeats one,
// quotes the column the wrong way for the dialect (MySQL reads a double-quoted
// name as a string) or leaves a quote in a value unescaped.
func TestCheckInExpr(t *testing.T) {
	values := []string{"c", "it's", "a", "c"}
	cases := map[dialect.Name]string{
		dialect.Postgres: `"kind" IN ('a','c','it''s')`,
		dialect.SQLite:   `"kind" IN ('a','c','it''s')`,
		dialect.MySQL:    "`kind` IN ('a','c','it''s')",
	}
	for d, want := range cases {
		if got := schema.CheckInExpr(d, "kind", values); got != want {
			t.Errorf("%s: got %s, want %s", d, got, want)
		}
		if got := schema.CheckInExpr(d, "kind", []string{"it's", "c", "a"}); got != want {
			t.Errorf("%s: a reordered list renders %s, want %s", d, got, want)
		}
	}
	// What the renderer writes is what the parser reads back.
	col, got, ok := schema.ParseCheckIn(schema.CheckInExpr(dialect.MySQL, "kind", values))
	if !ok || col != "kind" || !slices.Equal(got, []string{"a", "c", "it's"}) {
		t.Fatalf("round trip = %q %v %v", col, got, ok)
	}
}

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
		return schema.Check{Name: "notes_status_check", Expr: schema.CheckInExpr(d, "status", values), In: &schema.CheckIn{Column: "status", Values: values}}
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
