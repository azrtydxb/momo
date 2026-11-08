// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestNormalizeType(t *testing.T) {
	cases := []struct {
		d       dialect.Name
		in, out string
	}{
		{dialect.Postgres, "INT4", "integer"},
		{dialect.Postgres, "int", "integer"},
		{dialect.Postgres, "int8", "bigint"},
		{dialect.Postgres, "bool", "boolean"},
		{dialect.Postgres, "varchar(255)", "character varying(255)"},
		{dialect.Postgres, "timestamptz", "timestamp with time zone"},
		{dialect.Postgres, "vector( 3 )", "vector(3)"},
		{dialect.Postgres, "  Text  ", "text"},
		{dialect.Postgres, "timestamptz(3)", "timestamp(3) with time zone"},
		{dialect.Postgres, "timestamp(3) with time zone", "timestamp(3) with time zone"},
		{dialect.Postgres, "TIMESTAMP(6) WITHOUT TIME ZONE", "timestamp(6) without time zone"},
		{dialect.Postgres, "timestamp(3)", "timestamp(3) without time zone"},
		{dialect.Postgres, "time(2) with time zone", "time(2) with time zone"},
		{dialect.Postgres, "timetz", "time with time zone"},
		{dialect.Postgres, "timestamp with time zone", "timestamp with time zone"},
		{dialect.SQLite, "integer", "INTEGER"},
		{dialect.SQLite, "varchar(20)", "VARCHAR(20)"},
		// An unbalanced argument list is kept as written (found by FuzzNormalize).
		{dialect.Postgres, "TIME(0000", "time(0000"},
		{dialect.Postgres, "varchar( 3", "varchar( 3"},
		{dialect.SQLite, "time(0000", "TIME(0000"},
	}
	for _, c := range cases {
		if got := NormalizeType(c.d, c.in); got != c.out {
			t.Errorf("NormalizeType(%s, %q) = %q, want %q", c.d, c.in, got, c.out)
		}
	}
}

func TestNormalizeVectorTypes(t *testing.T) {
	cases := map[string]string{
		"vector(1536)":    "vector(1536)",
		"VECTOR(1536)":    "vector(1536)",
		"vector( 3 )":     "vector(3)",
		"halfvec(3072)":   "halfvec(3072)",
		"sparsevec(1000)": "sparsevec(1000)",
		"bit(8)":          "bit(8)",
		"BIT(8)":          "bit(8)",
	}
	for in, want := range cases {
		if got := NormalizeType(dialect.Postgres, in); got != want {
			t.Errorf("NormalizeType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeExpr(t *testing.T) {
	cases := []struct {
		d       dialect.Name
		in, out string
	}{
		{dialect.Postgres, "((now()))", "now()"},
		{dialect.Postgres, "  COALESCE( a ,  0 )  ", "coalesce(a , 0)"},
		{dialect.Postgres, "'Mixed CASE'::text", "'Mixed CASE'"},
		{dialect.Postgres, `"Quoted" > 0`, `"Quoted" > 0`},
		{dialect.SQLite, "(a > 0)", "a > 0"},
	}
	for _, c := range cases {
		if got := NormalizeExpr(c.d, c.in); got != c.out {
			t.Errorf("NormalizeExpr(%s, %q) = %q, want %q", c.d, c.in, got, c.out)
		}
	}
}

func TestEqualIgnoresOrderAndConcurrently(t *testing.T) {
	a := populated()
	b := populated()
	b.Tables[0].Indexes[0].Concurrently = false
	a.Tables[0].Indexes[0].Concurrently = true
	b.Tables[0].Columns[0].Type = "BIGINT"
	if !Equal(a, b) {
		t.Fatal("schemas differing only in Concurrently and type case must be equal")
	}
	c := populated()
	c.Tables[0].Columns[0].Type = "text"
	if Equal(a, c) {
		t.Fatal("schemas with different column types must not be equal")
	}
}

func FuzzNormalizeType(f *testing.F) {
	f.Add("int4")
	f.Add("varchar(255)")
	f.Fuzz(func(t *testing.T, s string) {
		once := NormalizeType(dialect.Postgres, s)
		if NormalizeType(dialect.Postgres, once) != once {
			t.Fatalf("NormalizeType is not idempotent for %q", s)
		}
	})
}
