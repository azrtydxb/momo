// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestTupleInMySQLAndEdges(t *testing.T) {
	keys := [][]any{{int64(1), "x"}, {int64(2), "y"}}
	my := renderPred(t, dialect.MySQL, inTuple{table: "c", cols: []string{"a", "b"}, keys: keys})
	if my.SQL != "(`c`.`a`, `c`.`b`) IN ((?, ?), (?, ?))" || len(my.Args) != 4 {
		t.Fatalf("mysql composite: got %q args=%d", my.SQL, len(my.Args))
	}
	lite := renderPred(t, dialect.SQLite, inTuple{table: "c", cols: []string{"a", "b"}, keys: keys})
	if lite.SQL != `(("c"."a" = ? AND "c"."b" = ?) OR ("c"."a" = ? AND "c"."b" = ?))` || len(lite.Args) != 4 {
		t.Fatalf("sqlite composite: got %q", lite.SQL)
	}
	emptyComposite := renderPred(t, dialect.MySQL, inTuple{table: "c", cols: []string{"a", "b"}})
	if emptyComposite.SQL != "1 = 0" {
		t.Fatalf("empty composite: got %q", emptyComposite.SQL)
	}
	if n := keysPerStatement(dialect.For(dialect.Postgres), 0, 0); n != 65535 {
		t.Fatalf("zero cols: got %d", n)
	}
	if n := keysPerStatement(dialect.For(dialect.SQLite), 2, 40000); n != 1 {
		t.Fatalf("over-reserved must floor at 1: got %d", n)
	}
	if n := keysPerStatement(&dialect.Dialect{}, 1, 0); n < 1<<20 {
		t.Fatalf("unlimited dialect, single column: got %d", n)
	}
	if n := keysPerStatement(&dialect.Dialect{}, 2, 0); n != maxCompositeTuples {
		t.Fatalf("unlimited dialect, composite is still capped: got %d", n)
	}
}

func TestKeysPerStatementPerDialect(t *testing.T) {
	cases := []struct {
		name  dialect.Name
		ncols int
		want  int
	}{
		{dialect.Postgres, 1, 65535},
		{dialect.Postgres, 2, 2000},
		{dialect.Postgres, 3, 2000},
		{dialect.MySQL, 1, 65535},
		{dialect.MySQL, 2, 2000},
		{dialect.SQLite, 1, 32766},
		{dialect.SQLite, 2, 400},
		{dialect.SQLite, 5, 400},
	}
	for _, c := range cases {
		if got := keysPerStatement(dialect.For(c.name), c.ncols, 0); got != c.want {
			t.Errorf("%s ncols=%d: got %d, want %d", c.name, c.ncols, got, c.want)
		}
	}
	// A tight bind limit still wins over the cap.
	d := *dialect.For(dialect.Postgres)
	d.MaxParams = 10
	if got := keysPerStatement(&d, 2, 0); got != 5 {
		t.Errorf("tight limit: got %d, want 5", got)
	}
}

func TestTupleInSingleColumnOnEveryDialect(t *testing.T) {
	keys := [][]any{{int64(1)}, {int64(2)}}
	want := map[dialect.Name]string{
		dialect.SQLite: `"c"."a" IN (?, ?)`,
		dialect.MySQL:  "`c`.`a` IN (?, ?)",
	}
	for d, sql := range want {
		got := renderPred(t, d, inTuple{table: "c", cols: []string{"a"}, keys: keys})
		if got.SQL != sql || len(got.Args) != 2 {
			t.Errorf("%s: got %q args=%d", d, got.SQL, len(got.Args))
		}
	}
}

func TestTupleInEmptyOnEveryDialect(t *testing.T) {
	for _, d := range []dialect.Name{dialect.Postgres, dialect.MySQL, dialect.SQLite} {
		for _, cols := range [][]string{nil, {"a"}, {"a", "b"}} {
			if got := renderPred(t, d, inTuple{table: "c", cols: cols}); got.SQL != "1 = 0" {
				t.Errorf("%s cols=%v: got %q", d, cols, got.SQL)
			}
		}
	}
}

func TestTupleInBadInputSetsError(t *testing.T) {
	bad := map[string]inTuple{
		"keys without columns":      {table: "c", keys: [][]any{{int64(1)}}},
		"short composite tuple":     {table: "c", cols: []string{"a", "b"}, keys: [][]any{{int64(1), "x"}, {int64(2)}}},
		"long composite tuple":      {table: "c", cols: []string{"a", "b"}, keys: [][]any{{int64(1), "x", "y"}}},
		"single column long tuple":  {table: "c", cols: []string{"a"}, keys: [][]any{{int64(1), int64(2)}}},
		"single column empty tuple": {table: "c", cols: []string{"a"}, keys: [][]any{{}}},
	}
	for _, d := range []dialect.Name{dialect.Postgres, dialect.MySQL, dialect.SQLite} {
		for name, p := range bad {
			_, err := compile(dialect.For(d), func(b *Builder) { p.AppendSQL(b) })
			if err == nil {
				t.Errorf("%s/%s: want a build error, got none", d, name)
			}
		}
	}
}

func TestTupleInNumbersArgsAmongOthers(t *testing.T) {
	c, err := compile(dialect.For(dialect.Postgres), func(b *Builder) {
		b.Arg(int64(0))
		b.WriteString(" AND ")
		inTuple{table: "c", cols: []string{"a", "b"}, keys: [][]any{{int64(1), "x"}, {int64(2), "y"}}}.AppendSQL(b)
		b.WriteString(" AND ")
		b.Arg(int64(9))
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `$1 AND ("c"."a", "c"."b") IN (($2, $3), ($4, $5)) AND $6`
	if c.SQL != want || len(c.Args) != 6 {
		t.Fatalf("got %q args=%d, want %q", c.SQL, len(c.Args), want)
	}
}
