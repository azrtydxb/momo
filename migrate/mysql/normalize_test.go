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
	}
	for _, c := range cases {
		if got, want := schema.NormalizeExpr(dialect.MySQL, c.a), schema.NormalizeExpr(dialect.MySQL, c.b); got != want {
			t.Errorf("NormalizeExpr(%q) = %q, want %q (the form of %q)", c.a, got, want, c.b)
		}
	}
	// Text in a string literal is data and stays as it is.
	if got := schema.NormalizeExpr(dialect.MySQL, "a = 'x`y_utf8mb4'"); got != "a = 'x`y_utf8mb4'" {
		t.Errorf("a string literal changed: %q", got)
	}
}
