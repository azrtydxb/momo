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
	if n := keysPerStatement(dialect.For(dialect.SQLite), 1, 16); n != 32766-16 {
		t.Fatalf("sqlite single-column budget: got %d", n)
	}
	if n := keysPerStatement(dialect.For(dialect.SQLite), 2, 16); n != maxCompositeTuplesSQLite {
		t.Fatalf("sqlite composite budget: got %d", n)
	}
	if n := keysPerStatement(dialect.For(dialect.Postgres), 1, 16); n != 65535-16 {
		t.Fatalf("postgres budget: got %d", n)
	}
}
