// SPDX-License-Identifier: Apache-2.0

package mysql

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

func TestMySQLNormalizeType(t *testing.T) {
	cases := map[string]string{
		"int(11)":              "int",
		"bigint(20) unsigned":  "bigint unsigned",
		"varchar(255)":         "varchar(255)",
		"tinyint(1)":           "tinyint(1)",
		"VECTOR(1536)":         "vector(1536)",
		"longtext":             "longtext",
		"INTEGER":              "int",
		"bool":                 "tinyint(1)",
		"boolean":              "tinyint(1)",
		"int unsigned":         "int unsigned",
		"smallint(5) unsigned": "smallint unsigned",
		"tinyint(4)":           "tinyint",
		"double precision":     "double",
		"numeric(10, 2)":       "decimal(10,2)",
		"DATETIME(6)":          "datetime(6)",
	}
	for in, want := range cases {
		got := schema.NormalizeType(dialect.MySQL, in)
		if got != want {
			t.Errorf("NormalizeType(%q) = %q, want %q", in, got, want)
		}
		if again := schema.NormalizeType(dialect.MySQL, got); again != got {
			t.Errorf("NormalizeType is not idempotent for %q: %q then %q", in, got, again)
		}
	}
}

// TestMySQLNormalizeExpr covers the spellings the servers store for the same
// expression: identifier quotes, character set introducers and parentheses.
func TestMySQLNormalizeExpr(t *testing.T) {
	cases := []struct{ a, b string }{
		{"(`pa` + `pb`)", "pa + pb"},
		{"`qty` >= 0", "qty >= 0"},
		{"(`name` = _utf8mb4'x')", "name = 'x'"},
		{"json_valid(`attrs`)", "json_valid(attrs)"},
		// Default spellings: the servers store these alike.
		{"CURRENT_TIMESTAMP", "current_timestamp()"},
		{"NOW()", "CURRENT_TIMESTAMP"},
		{"localtime", "now()"},
		{"NOW(6)", "current_timestamp(6)"},
		{"now(0)", "current_timestamp"},
		{"TRUE", "1"},
		{"false", "0"},
		// MySQL parenthesises every part of a stored CHECK.
		{"((`level` >= 0) and ((`level` < 10) or (`level` in (20,30))))", "level >= 0 and (level < 10 or level in (20, 30))"},
		{"(`a` > 1) or (`b` is null)", "a > 1 or b is null"},
	}
	for _, c := range cases {
		if got, want := schema.NormalizeExpr(dialect.MySQL, c.a), schema.NormalizeExpr(dialect.MySQL, c.b); got != want {
			t.Errorf("NormalizeExpr(%q) = %q, want %q (the form of %q)", c.a, got, want, c.b)
		}
	}
	// Parentheses that carry meaning stay.
	for _, keep := range []string{"(a + b) * c", "f(a > 1,2)", "a = (b + 1) * 2", "not (a > 1 or b > 2)", "not (a > 1)", "c between 1 and (d < 2)", "(a between 1 and 2) and c > 0", `a = 'it\'S  X'`} {
		if got := schema.NormalizeExpr(dialect.MySQL, keep); got != keep {
			t.Errorf("NormalizeExpr(%q) = %q, the parentheses carry meaning", keep, got)
		}
	}
	// Text in a string literal is data and stays as it is.
	if got := schema.NormalizeExpr(dialect.MySQL, "a = 'x`y_utf8mb4'"); got != "a = 'x`y_utf8mb4'" {
		t.Errorf("a string literal changed: %q", got)
	}
}
