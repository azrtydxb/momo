// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"fmt"
	"testing"
)

func TestNormalizeShape(t *testing.T) {
	cases := []struct{ in, want string }{
		{`SELECT * FROM "a" WHERE "a"."id" IN (?, ?, ?)`, `SELECT * FROM "a" WHERE "a"."id" IN (?)`},
		{`SELECT * FROM "a" WHERE "a"."id" IN ($1, $2)`, `SELECT * FROM "a" WHERE "a"."id" IN (?)`},
		{`SELECT * FROM "a" WHERE "a"."id" = $3`, `SELECT * FROM "a" WHERE "a"."id" = ?`},
		{`SELECT * FROM "a" WHERE "a"."id" = ?`, `SELECT * FROM "a" WHERE "a"."id" = ?`},
		{`SELECT * FROM "a" WHERE ("x", "y") IN ((?, ?), (?, ?), (?, ?))`, `SELECT * FROM "a" WHERE ("x", "y") IN ((?))`},
	}
	for _, c := range cases {
		if got := normalizeShape(c.in); got != c.want {
			t.Errorf("normalizeShape(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func nplusoneEvent(sql string) *QueryEvent { return &QueryEvent{Op: "select", SQL: sql} }

func TestNPlusOneShapeCap(t *testing.T) {
	var found int
	h := NewNPlusOneHook(1, func(NPlusOneFinding) { found++ })
	ctx := WithQueryScope(context.Background())
	for i := 0; i < maxScopeShapes+1; i++ {
		h.BeforeQuery(ctx, nplusoneEvent(fmt.Sprintf(`SELECT %d FROM "t%d"`, i, i)))
	}
	sc := ctx.Value(queryScopeKey{}).(*queryScope)
	if len(sc.counts) != maxScopeShapes {
		t.Fatalf("tracked %d shapes, want the cap %d", len(sc.counts), maxScopeShapes)
	}
	// A shape that is already tracked keeps counting once the cap is reached.
	h.BeforeQuery(ctx, nplusoneEvent(`SELECT 0 FROM "t0"`))
	if found != 1 {
		t.Fatalf("known shape stopped counting at the cap: %d findings", found)
	}
}

func TestNPlusOneReportPanicRecovered(t *testing.T) {
	h := NewNPlusOneHook(1, func(NPlusOneFinding) { panic("boom") })
	ctx := WithQueryScope(context.Background())
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic escaped the hook: %v", r)
		}
	}()
	for i := 0; i < 3; i++ {
		if got := h.BeforeQuery(ctx, nplusoneEvent(`SELECT 1`)); got == nil {
			t.Fatal("BeforeQuery returned a nil context")
		}
	}
}
