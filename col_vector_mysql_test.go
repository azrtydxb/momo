// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vector"
)

// A vector value is wrapped into the server's text-to-vector function on
// every typed bind path, not only in a distance, so an insert, an update or an
// equality works on a VECTOR column.
func TestMySQLVectorBindWrapping(t *testing.T) {
	col := NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	q := vector.Vector{1, 2, 3}
	cases := []struct {
		flavor dialect.Flavor
		want   string
	}{
		{dialect.FlavorMariaDB, "`docs`.`emb` = VEC_FromText(?)"},
		{dialect.FlavorMySQL, "`docs`.`emb` = STRING_TO_VECTOR(?)"},
	}
	for _, c := range cases {
		got, err := compile(dialect.MySQLFlavor(c.flavor), col.Eq(q).AppendSQL)
		if err != nil {
			t.Fatalf("%s: %v", c.flavor, err)
		}
		if got.SQL != c.want || len(got.Args) != 1 || got.Args[0] != "[1,2,3]" {
			t.Fatalf("%s: %q %#v, want %q with the text form", c.flavor, got.SQL, got.Args, c.want)
		}
	}
	// Other types keep the plain placeholder.
	n := NewColumn[int64]("docs", "id", 0)
	if got, _ := compile(dialect.MySQLFlavor(dialect.FlavorMariaDB), n.Eq(5).AppendSQL); strings.Contains(got.SQL, "VEC_") {
		t.Fatalf("integer bind wrapped: %s", got.SQL)
	}
}

// The bound vector of an equality shows as RedactedArg in hook events when the
// column is sensitive, through the wrapper too. The distance case is covered
// in package vecop.
func TestMySQLVectorSensitiveArg(t *testing.T) {
	ma := dialect.MySQLFlavor(dialect.FlavorMariaDB)
	q := vector.Vector{1, 2, 3}
	for _, c := range []struct {
		flags ColumnFlags
		want  []int
	}{{FlagSensitive, []int{0}}, {0, nil}} {
		col := NewVectorColumn[vector.Vector]("docs", "emb", 3, c.flags)
		eq, err := compile(ma, col.Eq(q).AppendSQL)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(eq.sens, c.want) {
			t.Fatalf("flags %d: Eq sensitive args = %v, want %v", c.flags, eq.sens, c.want)
		}
	}
}

// A nullable vector field is a *vector.Vector: non-nil binds wrapped, nil binds
// NULL without the wrapper.
func TestMySQLVectorNullablePointer(t *testing.T) {
	col := NewNullVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	v := vector.Vector{1, 2, 3}
	for _, c := range []struct {
		flavor dialect.Flavor
		want   string
	}{
		{dialect.FlavorMariaDB, "VEC_FromText(?)"},
		{dialect.FlavorMySQL, "STRING_TO_VECTOR(?)"},
	} {
		d := dialect.MySQLFlavor(c.flavor)
		got, err := compile(d, func(b *Builder) { b.argFor(0, &v) })
		if err != nil {
			t.Fatal(err)
		}
		if got.SQL != c.want || len(got.Args) != 1 || got.Args[0] != "[1,2,3]" {
			t.Fatalf("%s non-nil pointer: %q %#v, want %q with the text form", c.flavor, got.SQL, got.Args, c.want)
		}
		var none *vector.Vector
		got, err = compile(d, func(b *Builder) { b.argFor(0, none) })
		if err != nil {
			t.Fatal(err)
		}
		if got.SQL != "?" || len(got.Args) != 1 || got.Args[0] != nil {
			t.Fatalf("%s nil pointer: %q %#v, want a bare NULL bind", c.flavor, got.SQL, got.Args)
		}
	}
	if _, err := compile(dialect.MySQLFlavor(dialect.FlavorMariaDB), col.Eq(v).AppendSQL); err != nil {
		t.Fatal(err)
	}
}
