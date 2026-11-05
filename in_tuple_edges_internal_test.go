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
	if n := keysPerStatement(&dialect.Dialect{}, 2, 0); n < 1<<20 {
		t.Fatalf("unlimited dialect: got %d", n)
	}
}
