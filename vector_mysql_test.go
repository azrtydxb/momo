// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vector"
)

func TestMySQLVectorSQL(t *testing.T) {
	ma := dialect.MySQLFlavor(dialect.FlavorMariaDB)
	my := dialect.MySQLFlavor(dialect.FlavorMySQL)
	col := NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	q := vector.Vector{1, 2, 3}

	if got, want := renderForTest(t, ma, CosineDistance(col, q)), "VEC_DISTANCE_COSINE(`docs`.`emb`, VEC_FromText(?))"; got != want {
		t.Fatalf("MariaDB cosine = %s, want %s", got, want)
	}
	if got, want := renderForTest(t, ma, L2Distance(col, q)), "VEC_DISTANCE_EUCLIDEAN(`docs`.`emb`, VEC_FromText(?))"; got != want {
		t.Fatalf("MariaDB L2 = %s, want %s", got, want)
	}
	if args := renderArgs(t, ma, L2Distance(col, q)); len(args) != 1 || args[0] != "[1,2,3]" {
		t.Fatalf("MariaDB args = %#v, want the text form [1,2,3]", args)
	}
	for _, e := range []Expression[float64]{L2Distance(col, q), CosineDistance(col, q), InnerProductDistance(col, q)} {
		if _, err := renderErr(my, e); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("MySQL distance = %v, want ErrUnsupported", err)
		}
	}
	// MariaDB offers neither inner product nor L1: no silent substitute.
	for _, e := range []Expression[float64]{InnerProductDistance(col, q), L1Distance(col, q)} {
		if _, err := renderErr(ma, e); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("MariaDB unsupported metric = %v, want ErrUnsupported", err)
		}
	}
	// Half precision, sparse and bit vectors have no MariaDB type.
	hc := NewVectorColumn[vector.HalfVector]("docs", "emb", 3, 0)
	if _, err := renderErr(ma, L2Distance(hc, vector.HalfVector{1, 2, 3})); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("MariaDB halfvec = %v, want ErrUnsupported", err)
	}
	if _, err := MySQLVectorType(dialect.FlavorMySQL, "8.4.13", 3); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("MySQL 8.4 VECTOR = %v, want ErrUnsupported", err)
	}
}

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

// The query vector shows as RedactedArg in hook events when the column is
// sensitive, through the wrapper too.
func TestMySQLVectorSensitiveArg(t *testing.T) {
	ma := dialect.MySQLFlavor(dialect.FlavorMariaDB)
	q := vector.Vector{1, 2, 3}
	for _, c := range []struct {
		flags uint16
		want  []int
	}{{FlagSensitive, []int{0}}, {0, nil}} {
		col := NewVectorColumn[vector.Vector]("docs", "emb", 3, c.flags)
		got, err := compile(ma, CosineDistance(col, q).AppendSQL)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got.sens, c.want) {
			t.Fatalf("flags %d: sensitive args = %v, want %v", c.flags, got.sens, c.want)
		}
		eq, err := compile(ma, col.Eq(q).AppendSQL)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(eq.sens, c.want) {
			t.Fatalf("flags %d: Eq sensitive args = %v, want %v", c.flags, eq.sens, c.want)
		}
	}
}

func TestMySQLVectorType(t *testing.T) {
	cases := []struct {
		name    string
		flavor  dialect.Flavor
		version string
		dim     int
		want    string
		bad     bool
	}{
		{"mysql 9.7", dialect.FlavorMySQL, "9.7.4", 3, "VECTOR(3)", false},
		{"mysql 9.0", dialect.FlavorMySQL, "9.0.0", 1536, "VECTOR(1536)", false},
		{"mysql 9.7 suffix", dialect.FlavorMySQL, "9.7.4-log", 3, "VECTOR(3)", false},
		{"mysql 8.4", dialect.FlavorMySQL, "8.4.13", 3, "", true},
		{"mysql 8.0", dialect.FlavorMySQL, "8.0.40", 3, "", true},
		{"mariadb 11.8", dialect.FlavorMariaDB, "11.8.9-MariaDB", 8, "VECTOR(8)", false},
		{"mariadb 11.8 replication prefix", dialect.FlavorMariaDB, "5.5.5-11.8.9-MariaDB-ubu2404", 8, "VECTOR(8)", false},
		{"mariadb 11.7", dialect.FlavorMariaDB, "11.7.2-MariaDB", 8, "VECTOR(8)", false},
		{"mariadb 10.11", dialect.FlavorMariaDB, "10.11.6-MariaDB", 8, "", true},
		{"mariadb 11.6", dialect.FlavorMariaDB, "11.6.2-MariaDB", 8, "", true},
		{"no dimension", dialect.FlavorMariaDB, "11.8.9-MariaDB", 0, "", true},
		{"garbage version", dialect.FlavorMySQL, "abc", 3, "", true},
		{"unknown flavor", dialect.Flavor("oracle"), "9.7.4", 3, "", true},
	}
	for _, c := range cases {
		got, err := MySQLVectorType(c.flavor, c.version, c.dim)
		if c.bad {
			if !errors.Is(err, ErrUnsupported) {
				t.Errorf("%s: err = %v, want ErrUnsupported", c.name, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: %q, %v; want %q", c.name, got, err, c.want)
		}
	}
	_, err := MySQLVectorType(dialect.FlavorMySQL, "8.4.13", 3)
	if err == nil || !strings.Contains(err.Error(), "8.4.13") {
		t.Fatalf("diagnostic %v does not name the server version", err)
	}
	if _, err := MySQLVectorType(dialect.FlavorMariaDB, "10.11.6-MariaDB", 3); err == nil || !strings.Contains(err.Error(), "VECTOR requires MySQL 9.0 or MariaDB 11.7") {
		t.Fatalf("MariaDB diagnostic %v lacks the requirement text", err)
	}
}
