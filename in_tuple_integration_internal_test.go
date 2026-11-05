// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb"
)

func TestTupleInPostgres(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Release)
	if _, err := conn.Exec(ctx, `CREATE TEMP TABLE c (a bigint, b text)`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO c VALUES (1,'x'),(1,'y'),(2,'x'),(3,'z')`); err != nil {
		t.Fatal(err)
	}
	count := func(p Predicate) int {
		t.Helper()
		comp, err := compile(dialect.For(dialect.Postgres), func(b *Builder) {
			b.WriteString("SELECT count(*) FROM c WHERE ")
			p.AppendSQL(b)
		})
		if err != nil {
			t.Fatal(err)
		}
		var n int
		if err := conn.QueryRow(ctx, comp.SQL, comp.Args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", comp.SQL, err)
		}
		return n
	}
	if n := count(inTuple{table: "c", cols: []string{"a"}, keys: [][]any{{int64(1)}, {int64(3)}}}); n != 3 {
		t.Fatalf("single: %d", n)
	}
	if n := count(inTuple{table: "c", cols: []string{"a", "b"}, keys: [][]any{{int64(1), "y"}, {int64(2), "x"}, {int64(9), "x"}}}); n != 2 {
		t.Fatalf("composite: %d", n)
	}
	if n := count(inTuple{table: "c", cols: []string{"a"}}); n != 0 {
		t.Fatalf("empty must match nothing: %d", n)
	}
	// A full-budget statement executes: keysPerStatement single-column keys
	// bind exactly the server's limit or fewer. (Composite row-value lists of
	// that size exceed PostgreSQL's parser stack depth, so only the
	// single-column list is exercised at the full budget.)
	d := dialect.For(dialect.Postgres)
	per := keysPerStatement(d, 1, 0)
	keys := make([][]any, per)
	for i := range keys {
		keys[i] = []any{int64(i + 1)}
	}
	if n := count(inTuple{table: "c", cols: []string{"a"}, keys: keys}); n != 4 {
		t.Fatalf("budget-sized single-column: %d", n)
	}
}
