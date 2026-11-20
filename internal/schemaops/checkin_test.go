// SPDX-License-Identifier: Apache-2.0

package schemaops

import (
	"testing"

	"github.com/azrtydxb/momo/schema"
)

// Catches: SameCheck deciding on the wrong thing: text for value lists, sets
// for anything else, or a different answer for the declared side and the
// inspected side depending on which is first.
func TestSameCheck(t *testing.T) {
	in := func(expr, col string, values ...string) schema.Check {
		return schema.Check{Name: "c", Expr: expr, In: &schema.CheckIn{Column: col, Values: values}}
	}
	read := func(expr string) schema.Check { return schema.Check{Name: "c", Expr: expr} }
	cases := []struct {
		name string
		a, b schema.Check
		want bool
	}{
		{"plain same text", read("qty > 0"), read("qty > 0"), true},
		{"plain other text", read("qty > 0"), read("qty >= 0"), false},
		{"plain lists are text without In", read("s IN ('a','b')"), read("s IN ('b','a')"), false},
		{"declared and read, other order", in("x", "status", "a", "b"), read("(status = ANY (ARRAY['b'::text, 'a'::text]))"), true},
		{"declared and read, repeated value", in("x", "status", "a", "b", "a"), read("status in ('a','b')"), true},
		{"quote in a value", in("x", "status", "it's", "a"), read("(status = ANY (ARRAY['a'::text, 'it''s'::text]))"), true},
		{"other quoted value", in("x", "status", "it's"), read("(status = 'its'::text)"), false},
		{"mixed-case values differ", in("x", "status", "A", "b"), read("status in ('a','b')"), false},
		{"mixed-case values equal", in("x", "status", "A", "b"), read("status in ('b','A')"), true},
		{"other column", in("x", "status", "a"), read("kind in ('a')"), false},
		{"NULL element falls back to text, other", in("x", "status", "a", "b"), read("(status = ANY (ARRAY['a'::text, NULL::text]))"), false},
		{"NULL element falls back to text, same", in("s = ANY (ARRAY['a'::text, NULL::text])", "s", "a"), read("s = ANY (ARRAY['a'::text, NULL::text])"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SameCheck(c.a, c.b); got != c.want {
				t.Errorf("SameCheck(a, b) = %v, want %v", got, c.want)
			}
			if got := SameCheck(c.b, c.a); got != c.want {
				t.Errorf("SameCheck(b, a) = %v, want %v", got, c.want)
			}
		})
	}
}
