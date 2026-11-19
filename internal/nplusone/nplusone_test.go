// SPDX-License-Identifier: Apache-2.0

package nplusone_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/azrtydxb/momo/internal/nplusone"
)

func TestShape(t *testing.T) {
	cases := []struct{ in, want string }{
		{`SELECT * FROM "a" WHERE "a"."id" IN (?, ?, ?)`, `SELECT * FROM "a" WHERE "a"."id" IN (?)`},
		{`SELECT * FROM "a" WHERE "a"."id" IN ($1, $2)`, `SELECT * FROM "a" WHERE "a"."id" IN (?)`},
		{`SELECT * FROM "a" WHERE "a"."id" = $3`, `SELECT * FROM "a" WHERE "a"."id" = ?`},
		{`SELECT * FROM "a" WHERE "a"."id" = ?`, `SELECT * FROM "a" WHERE "a"."id" = ?`},
		{`SELECT * FROM "a" WHERE ("x", "y") IN ((?, ?), (?, ?), (?, ?))`, `SELECT * FROM "a" WHERE ("x", "y") IN ((?))`},
	}
	for _, c := range cases {
		if got := nplusone.Shape(c.in); got != c.want {
			t.Errorf("Shape(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStatementShapeCap(t *testing.T) {
	ctx := nplusone.With(context.Background())
	for i := 0; i < nplusone.MaxKeys; i++ {
		if _, _, _, ok := nplusone.Statement(ctx, "select", fmt.Sprintf(`SELECT %d FROM "t%d"`, i, i), 1); !ok {
			t.Fatalf("shape %d was not tracked below the cap", i)
		}
	}
	if _, _, _, ok := nplusone.Statement(ctx, "select", `SELECT 0 FROM "over"`, 1); ok {
		t.Fatal("a new shape was tracked past the cap")
	}
	// A shape that is already tracked keeps counting once the cap is reached.
	_, n, fire, ok := nplusone.Statement(ctx, "select", `SELECT 0 FROM "t0"`, 1)
	if !ok || n != 2 || !fire {
		t.Fatalf("known shape stopped counting at the cap: n=%d fire=%v ok=%v", n, fire, ok)
	}
}

func TestRelationCap(t *testing.T) {
	ctx := nplusone.With(context.Background())
	for i := 0; i < nplusone.MaxKeys; i++ {
		if _, _, ok := nplusone.Relation(ctx, fmt.Sprintf("p.R%d", i), 1); !ok {
			t.Fatalf("relation %d was not tracked below the cap", i)
		}
	}
	if _, _, ok := nplusone.Relation(ctx, "p.Over", 1); ok {
		t.Fatal("a new relation was tracked past the cap")
	}
	if n, fire, ok := nplusone.Relation(ctx, "p.R0", 1); !ok || n != 2 || !fire {
		t.Fatalf("known relation stopped counting at the cap: n=%d fire=%v ok=%v", n, fire, ok)
	}
}

func TestStatementsAndRelationsCountSeparately(t *testing.T) {
	ctx := nplusone.With(context.Background())
	// The two tables are separate, so filling one leaves room in the other.
	for i := 0; i < nplusone.MaxKeys; i++ {
		nplusone.Statement(ctx, "select", fmt.Sprintf(`SELECT %d`, i), 1)
	}
	if _, _, ok := nplusone.Relation(ctx, "p.R", 1); !ok {
		t.Fatal("a full statement table blocked the relation table")
	}
}

func TestStatementIgnoresControlStatementsAndUnscopedContexts(t *testing.T) {
	ctx := nplusone.With(context.Background())
	for _, op := range []string{"begin", "commit", "rollback", "savepoint", "explain"} {
		if _, _, _, ok := nplusone.Statement(ctx, op, "X", 1); ok {
			t.Errorf("op %q was counted", op)
		}
	}
	if _, _, _, ok := nplusone.Statement(context.Background(), "select", "SELECT 1", 1); ok {
		t.Error("a context without a scope was counted")
	}
	if _, _, ok := nplusone.Relation(context.Background(), "p.R", 1); ok {
		t.Error("a context without a scope was counted")
	}
}
