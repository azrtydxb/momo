// SPDX-License-Identifier: Apache-2.0

package schemautil_test

import (
	"slices"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/schemautil"
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
			col, values, ok := schemautil.ParseCheckIn(c.expr)
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
		if got := schemautil.CheckInExpr(d, "kind", values); got != want {
			t.Errorf("%s: got %s, want %s", d, got, want)
		}
		if got := schemautil.CheckInExpr(d, "kind", []string{"it's", "c", "a"}); got != want {
			t.Errorf("%s: a reordered list renders %s, want %s", d, got, want)
		}
	}
	// What the renderer writes is what the parser reads back.
	col, got, ok := schemautil.ParseCheckIn(schemautil.CheckInExpr(dialect.MySQL, "kind", values))
	if !ok || col != "kind" || !slices.Equal(got, []string{"a", "c", "it's"}) {
		t.Fatalf("round trip = %q %v %v", col, got, ok)
	}
}

// Catches: SortedValues keeping repeats, leaving the order alone, or sorting
// the caller's slice in place.
func TestSortedValues(t *testing.T) {
	in := []string{"c", "a", "c", "b"}
	if got := schemautil.SortedValues(in); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("SortedValues = %v, want [a b c]", got)
	}
	if !slices.Equal(in, []string{"c", "a", "c", "b"}) {
		t.Errorf("SortedValues changed its argument: %v", in)
	}
}
