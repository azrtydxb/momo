// SPDX-License-Identifier: Apache-2.0

package tags

import (
	"slices"
	"testing"
)

// Catches: a comma inside the parentheses of a type tag splitting it into two
// options, so type:decimal(10,2) or type:enum('a','b') could not be written,
// and an unbalanced parenthesis swallowing the options after it.
func TestSplitOptions(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{"pk,identity", []string{"pk", "identity"}},
		{"type:decimal(10,2),notnull", []string{"type:decimal(10,2)", "notnull"}},
		{"type:enum('a','b')", []string{"type:enum('a','b')"}},
		{"comment:smile (,notnull", []string{"comment:smile (", "notnull"}},
		{"comment:a),notnull", []string{"comment:a)", "notnull"}},
		{"type:enum('(','b'),notnull", []string{"type:enum('(','b')", "notnull"}},
		{"type:enum('a)b','c'),notnull", []string{"type:enum('a)b','c')", "notnull"}},
		{"type:enum('it''s','x,y')", []string{"type:enum('it''s','x,y')"}},
		{"type:enum('a','b'", []string{"type:enum('a'", "'b'"}},
		{"comment:it's,index:x'y", []string{"comment:it's", "index:x'y"}},
	}
	for _, c := range cases {
		if got := SplitOptions(c.raw); !slices.Equal(got, c.want) {
			t.Errorf("SplitOptions(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
	g, probs := Parse("type:decimal(10,2),notnull")
	if len(probs) != 0 || g.Type != "decimal(10,2)" || !g.NotNull {
		t.Errorf("Parse = %+v, %v", g, probs)
	}
}
