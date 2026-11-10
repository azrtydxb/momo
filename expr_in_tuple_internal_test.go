// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func renderPred(t *testing.T, d dialect.Name, p Predicate) Compiled {
	t.Helper()
	c, err := compile(dialect.For(d), func(b *Builder) { p.AppendSQL(b) })
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTupleInRendering(t *testing.T) {
	single := renderPred(t, dialect.Postgres, inTuple{table: "c", cols: []string{"a"}, keys: [][]any{{int64(1)}, {int64(2)}}})
	if single.SQL != `"c"."a" IN ($1, $2)` || len(single.Args) != 2 {
		t.Fatalf("single: got %q args=%d", single.SQL, len(single.Args))
	}
	pgComposite := renderPred(t, dialect.Postgres, inTuple{table: "c", cols: []string{"a", "b"}, keys: [][]any{{int64(1), "x"}}})
	if pgComposite.SQL != `("c"."a", "c"."b") IN (($1, $2))` {
		t.Fatalf("pg composite: got %q", pgComposite.SQL)
	}
	liteComposite := renderPred(t, dialect.SQLite, inTuple{table: "c", cols: []string{"a", "b"}, keys: [][]any{{int64(1), "x"}, {int64(2), "y"}}})
	if strings.Contains(liteComposite.SQL, ") IN (") || strings.Count(liteComposite.SQL, " OR ") != 1 {
		t.Fatalf("sqlite composite must use OR of ANDs: got %q", liteComposite.SQL)
	}
	empty := renderPred(t, dialect.Postgres, inTuple{table: "c", cols: []string{"a"}})
	if empty.SQL != "1 = 0" || len(empty.Args) != 0 {
		t.Fatalf("empty IN must render always-false: got %q", empty.SQL)
	}
}

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
}

func TestTupleInSingleColumnOnEveryDialect(t *testing.T) {
	keys := [][]any{{int64(1)}, {int64(2)}}
	want := map[dialect.Name]string{
		dialect.Postgres: `"c"."a" IN ($1, $2)`,
		dialect.SQLite:   `"c"."a" IN (?, ?)`,
		dialect.MySQL:    "`c`.`a` IN (?, ?)",
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
