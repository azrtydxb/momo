// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestNormalizeTypes(t *testing.T) {
	cases := []struct {
		d        dialect.Name
		in, want string
	}{
		{dialect.Postgres, "int4", "integer"},
		{dialect.Postgres, "int8", "bigint"},
		{dialect.Postgres, "int2", "smallint"},
		{dialect.Postgres, "bool", "boolean"},
		{dialect.Postgres, "float8", "double precision"},
		{dialect.Postgres, "varchar(255)", "character varying(255)"},
		{dialect.Postgres, "timestamptz", "timestamp with time zone"},
		{dialect.Postgres, "timestamp", "timestamp without time zone"},
		{dialect.Postgres, "decimal(10,2)", "numeric(10,2)"},
		{dialect.Postgres, "int4[]", "integer[]"},
		{dialect.Postgres, "vector(1536)", "vector(1536)"},
		// The plan's SQLite cases (upper-case output) contradict momo-01's
		// TestNormalizeType (lower-case output) and are not applied.
	}
	for _, c := range cases {
		if got := NormalizeType(c.d, c.in); got != c.want {
			t.Errorf("NormalizeType(%s, %q) = %q, want %q", c.d, c.in, got, c.want)
		}
	}
}

func TestNormalizeExprDBRewrites(t *testing.T) {
	cases := []struct{ in, want string }{
		{"'active'::character varying", "'active'"},
		{"('now'::text)", "now()"},
		{"CURRENT_TIMESTAMP", "now()"},
		// The plan's "  (lower( name ))  " -> "lower(name)" case contradicts
		// momo-01's TestNormalizeExpr (spacing inside parentheses is kept).
		{"  (lower( name ))  ", "lower( name )"},
		{"((a > 0))", "a > 0"},
	}
	for _, c := range cases {
		if got := NormalizeExpr(dialect.Postgres, c.in); got != c.want {
			t.Errorf("NormalizeExpr(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestEqualIgnoresOrder(t *testing.T) {
	a := &Schema{Dialect: "postgres", Tables: []Table{{Name: "b"}, {Name: "a"}}}
	b := &Schema{Dialect: "postgres", Tables: []Table{{Name: "a"}, {Name: "b"}}}
	if !Equal(a, b) {
		t.Fatal("schemas differing only in table order must be equal")
	}
}

func TestNormalizedSortsAndDoesNotAlias(t *testing.T) {
	s := &Schema{Dialect: "postgres", Tables: []Table{{Name: "b", Columns: []Column{{Name: "c", Type: "INT4"}}}, {Name: "a"}}}
	n := Normalized(s)
	if n.Tables[0].Name != "a" || n.Tables[1].Columns[0].Type != "integer" {
		t.Fatalf("Normalized did not sort and normalize: %+v", n.Tables)
	}
	if s.Tables[0].Name != "b" || s.Tables[0].Columns[0].Type != "INT4" {
		t.Fatal("Normalized modified its argument")
	}
}
