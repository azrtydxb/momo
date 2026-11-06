// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// SQL escapes a quote inside a literal by doubling it, and an E” string also
// by backslash. The literal must stay whole and a cast after it is dropped.
func TestNormalizeExprQuoteEscapes(t *testing.T) {
	cases := []struct{ in, out string }{
		{`'it''s'::text`, `'it''s'`},
		{`'a''b'`, `'a''b'`},
		{`'it''s'::character varying`, `'it''s'`},
		{`'x'::text`, `'x'`},
		{`E'x\'y'::text`, `e'x\'y'`},
		{`E'it''s'::int`, `e'it''s'`},
		{`E'it''s'::text`, `e'it''s'`},
		{`'a'::text || 'b''c'::text`, `'a' || 'b''c'`},
		{`''''::text`, `''''`},
		{`'a''::text'`, `'a''::text'`},
		{`E'\\'::text`, `e'\\'`},
		{`E'a\'B'::text || 'C'::text`, `e'a\'B' || 'C'`},
		{`'a\'::text`, `'a\'`},
	}
	for _, c := range cases {
		if got := NormalizeExpr(dialect.Postgres, c.in); got != c.out {
			t.Errorf("NormalizeExpr(%q) = %q, want %q", c.in, got, c.out)
		}
	}
}
