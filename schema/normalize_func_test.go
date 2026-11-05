// SPDX-License-Identifier: Apache-2.0

package schema_test

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

func fnSchema(args, returns string) *schema.Schema {
	return &schema.Schema{Dialect: dialect.Postgres, Functions: []schema.Function{
		{Name: "f", Args: args, Returns: returns, Language: "sql", Volatility: "VOLATILE", Body: "select 1"},
	}}
}

func TestFunctionSignatureNormalization(t *testing.T) {
	equal := [][4]string{
		{"x int", "int", "x integer", "integer"},
		{"IN x int4, y varchar(10)", "SETOF int", "x integer, y character varying(10)", "SETOF integer"},
		{"x timestamptz DEFAULT now()", "TABLE(a int, b text)", "x timestamp with time zone DEFAULT  now()", "TABLE(a integer, b text)"},
		{"int, double precision", "bool", "integer, double precision", "boolean"},
		{"INOUT a int, VARIADIC v int[]", "void", "INOUT a integer, VARIADIC v integer[]", "void"},
	}
	for _, c := range equal {
		if !schema.Equal(fnSchema(c[0], c[1]), fnSchema(c[2], c[3])) {
			t.Errorf("%q -> %q must equal %q -> %q", c[0], c[1], c[2], c[3])
		}
	}
	// Different argument types or names are different functions.
	if schema.Equal(fnSchema("x int", "int"), fnSchema("x bigint", "int")) {
		t.Error("int and bigint arguments must differ")
	}
	if schema.Equal(fnSchema("x int", "int"), fnSchema("y int", "int")) {
		t.Error("argument names are kept and must differ")
	}
	n := schema.Normalized(fnSchema("x int", "int"))
	if n.Functions[0].Args != "x integer" || n.Functions[0].Returns != "integer" {
		t.Errorf("normalized = %+v", n.Functions[0])
	}
	if again := schema.Normalized(n); again.Functions[0] != n.Functions[0] {
		t.Error("normalization must be idempotent")
	}
}
