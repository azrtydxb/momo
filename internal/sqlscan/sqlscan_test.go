// SPDX-License-Identifier: Apache-2.0

package sqlscan

import (
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestScanTokens(t *testing.T) {
	pg, lite, my := dialect.For(dialect.Postgres), dialect.For(dialect.SQLite), dialect.For(dialect.MySQL)
	cases := []struct {
		name string
		sql  string
		d    *dialect.Dialect
		want []Tok
	}{
		{"positional", "a = ?", pg, []Tok{{Text, "a = "}, {Param, ""}}},
		{"named", "a = @x AND b = @y", lite, []Tok{{Text, "a = "}, {Named, "x"}, {Text, " AND b = "}, {Named, "y"}}},
		{"escaped question mark", "a ?? b", pg, []Tok{{Text, "a "}, {Text, "?"}, {Text, " b"}}},
		{"question mark in string", "'?' = ?", pg, []Tok{{Text, "'?' = "}, {Param, ""}}},
		{"jsonb operators are text", "a ?| b ?& c", pg, []Tok{{Text, "a ?| b ?& c"}}},
		{"native dollar", "a = $1 OR b = $12", pg, []Tok{{Text, "a = "}, {Dollar, "1"}, {Text, " OR b = "}, {Dollar, "12"}}},
		{"dollar quote is text", "$q$ ? $q$", pg, []Tok{{Text, "$q$ ? $q$"}}},
		{"comment is text", "-- ?\n?", pg, []Tok{{Text, "-- ?\n"}, {Param, ""}}},
		{"mysql system variable", "@@sql_mode", my, []Tok{{Text, "@@sql_mode"}}},
		{"containment is not named", "a <@ b", pg, []Tok{{Text, "a <@ b"}}},
		{"sqlite bracket identifier", "[a?b] = ?", lite, []Tok{{Text, "[a?b] = "}, {Param, ""}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Scan(c.sql, c.d)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, c.want) {
				t.Fatalf("Scan(%q) = %v, want %v", c.sql, got, c.want)
			}
		})
	}
}

func TestScanUnterminated(t *testing.T) {
	pg := dialect.For(dialect.Postgres)
	for sql, what := range map[string]string{
		"'abc":       "string literal",
		`"abc`:       "quoted identifier",
		"/* abc":     "block comment",
		"$q$ abc":    "dollar quote",
		"a = 'it''s": "string literal",
	} {
		_, err := Scan(sql, pg)
		if err == nil || !strings.Contains(err.Error(), "unterminated "+what) {
			t.Errorf("Scan(%q) error = %v, want unterminated %s", sql, err, what)
		}
	}
}

func TestIndexFrom(t *testing.T) {
	if got := indexFrom("abcabc", "bc", 2); got != 4 {
		t.Fatalf("indexFrom = %d, want 4", got)
	}
	if got := indexFrom("abc", "zz", 0); got != -1 {
		t.Fatalf("indexFrom = %d, want -1", got)
	}
}
