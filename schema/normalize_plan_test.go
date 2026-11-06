// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestEqualExtensionVersionAndFormatVersion(t *testing.T) {
	mk := func(fv int, ver string) *Schema {
		return &Schema{FormatVersion: fv, Dialect: "postgres", Extensions: []Extension{{Name: "vector", Version: ver}}}
	}
	if !Equal(mk(1, "0.8.7"), mk(0, "")) || !Equal(mk(0, ""), mk(1, "0.8.7")) {
		t.Fatal("an unset extension version matches any installed version, symmetrically, and FormatVersion is ignored")
	}
	if Equal(mk(1, "0.8.7"), mk(1, "0.7.0")) {
		t.Fatal("two set, different versions must differ")
	}
	if Equal(mk(1, ""), &Schema{Dialect: "postgres"}) {
		t.Fatal("a missing extension must differ")
	}
}

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
		{dialect.SQLite, "bigint", "BIGINT"},
		{dialect.SQLite, "varchar( 255 )", "VARCHAR(255)"},
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
		{"  (lower( name ))  ", "lower(name)"},
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

func TestNormalizedFunctionBody(t *testing.T) {
	a := &Schema{Dialect: "postgres", Functions: []Function{{Name: "f", Body: "BEGIN\n  RETURN  lower( x );\nEND"}}}
	b := &Schema{Dialect: "postgres", Functions: []Function{{Name: "f", Body: "BEGIN RETURN lower(x); END"}}}
	if !Equal(a, b) {
		t.Fatal("function bodies differing only in whitespace and case must be equal")
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
