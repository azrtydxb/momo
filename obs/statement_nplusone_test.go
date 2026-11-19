// SPDX-License-Identifier: Apache-2.0

package obs_test

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/obs"
)

// The three declarations below name the deprecated forwarders in package momo.
// They are the code under test here, so the deprecation warning is expected.

//lint:ignore SA1019 the deprecated forwarder is the code under test
var forwardedHook = momo.NewNPlusOneHook //nolint:staticcheck // the deprecated forwarder is the code under test

//lint:ignore SA1019 the deprecated forwarder is the code under test
var forwardedScope = momo.WithQueryScope //nolint:staticcheck // the deprecated forwarder is the code under test

//lint:ignore SA1019 the deprecated forwarder is the code under test
type forwardedFinding = momo.NPlusOneFinding //nolint:staticcheck // the deprecated forwarder is the code under test

func selectEvent(sql string) *momo.QueryEvent { return &momo.QueryEvent{Op: "select", SQL: sql} }

// statements collects the findings of a StatementNPlusOne.
type statements struct {
	mu    sync.Mutex
	found []obs.StatementFinding
}

func (s *statements) add(f obs.StatementFinding) {
	s.mu.Lock()
	s.found = append(s.found, f)
	s.mu.Unlock()
}

func (s *statements) all() []obs.StatementFinding {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]obs.StatementFinding(nil), s.found...)
}

func TestStatementNPlusOne(t *testing.T) {
	const sel = `SELECT * FROM "a" WHERE "a"."id" = $1`
	t.Run("a repeated statement is reported once at the first excess", func(t *testing.T) {
		var s statements
		h := obs.NewStatementNPlusOne(3, s.add)
		ctx := obs.WithScope(context.Background())
		for i := 0; i < 6; i++ {
			h.BeforeQuery(ctx, selectEvent(sel))
		}
		got := s.all()
		if len(got) != 1 {
			t.Fatalf("want 1 finding, got %+v", got)
		}
		if f := got[0]; f.Count != 4 || f.Shape != `SELECT * FROM "a" WHERE "a"."id" = ?` || f.Example != sel {
			t.Fatalf("finding %+v", f)
		}
	})
	t.Run("exactly the threshold stays silent", func(t *testing.T) {
		var s statements
		h := obs.NewStatementNPlusOne(3, s.add)
		ctx := obs.WithScope(context.Background())
		for i := 0; i < 3; i++ {
			h.BeforeQuery(ctx, selectEvent(sel))
		}
		if got := s.all(); len(got) != 0 {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("a threshold below 1 counts as 1", func(t *testing.T) {
		var s statements
		h := obs.NewStatementNPlusOne(0, s.add)
		ctx := obs.WithScope(context.Background())
		h.BeforeQuery(ctx, selectEvent(sel))
		h.BeforeQuery(ctx, selectEvent(sel))
		if got := s.all(); len(got) != 1 || got[0].Count != 2 {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("IN lists of different sizes share one shape", func(t *testing.T) {
		var s statements
		h := obs.NewStatementNPlusOne(1, s.add)
		ctx := obs.WithScope(context.Background())
		h.BeforeQuery(ctx, selectEvent(`SELECT * FROM "a" WHERE "a"."id" IN ($1, $2)`))
		h.BeforeQuery(ctx, selectEvent(`SELECT * FROM "a" WHERE "a"."id" IN ($1, $2, $3, $4)`))
		if got := s.all(); len(got) != 1 {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("transaction control and EXPLAIN are ignored", func(t *testing.T) {
		var s statements
		h := obs.NewStatementNPlusOne(1, s.add)
		ctx := obs.WithScope(context.Background())
		for _, op := range []string{"begin", "commit", "rollback", "savepoint", "explain"} {
			for i := 0; i < 3; i++ {
				h.BeforeQuery(ctx, &momo.QueryEvent{Op: op, SQL: "X"})
			}
		}
		if got := s.all(); len(got) != 0 {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("a context without a scope is not tracked", func(t *testing.T) {
		var s statements
		h := obs.NewStatementNPlusOne(1, s.add)
		for i := 0; i < 6; i++ {
			h.BeforeQuery(context.Background(), selectEvent(sel))
		}
		if got := s.all(); len(got) != 0 {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("scopes are independent", func(t *testing.T) {
		var s statements
		h := obs.NewStatementNPlusOne(3, s.add)
		for g := 0; g < 2; g++ {
			ctx := obs.WithScope(context.Background())
			for i := 0; i < 3; i++ {
				h.BeforeQuery(ctx, selectEvent(sel))
			}
		}
		if got := s.all(); len(got) != 0 {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("a panic in the report function is recovered", func(t *testing.T) {
		h := obs.NewStatementNPlusOne(1, func(obs.StatementFinding) { panic("boom") })
		ctx := obs.WithScope(context.Background())
		for i := 0; i < 3; i++ {
			if got := h.BeforeQuery(ctx, selectEvent(sel)); got == nil {
				t.Fatal("BeforeQuery returned a nil context")
			}
		}
	})
	t.Run("a nil report function is allowed", func(t *testing.T) {
		h := obs.NewStatementNPlusOne(1, nil)
		ctx := obs.WithScope(context.Background())
		for i := 0; i < 3; i++ {
			h.BeforeQuery(ctx, selectEvent(sel))
		}
	})
	t.Run("concurrent repeats in one scope report once", func(t *testing.T) {
		var s statements
		h := obs.NewStatementNPlusOne(5, s.add)
		ctx := obs.WithScope(context.Background())
		var wg sync.WaitGroup
		for g := 0; g < 8; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 5; i++ {
					h.BeforeQuery(ctx, selectEvent(sel))
				}
			}()
		}
		wg.Wait()
		if got := s.all(); len(got) != 1 {
			t.Fatalf("want 1 finding, got %d", len(got))
		}
	})
	t.Run("a shape past the cap is not tracked, a known one still counts", func(t *testing.T) {
		var s statements
		h := obs.NewStatementNPlusOne(1, s.add)
		ctx := obs.WithScope(context.Background())
		for i := 0; i < 1024+1; i++ {
			h.BeforeQuery(ctx, selectEvent(fmt.Sprintf(`SELECT %d FROM "t%d"`, i, i)))
		}
		h.BeforeQuery(ctx, selectEvent(`SELECT 0 FROM "t0"`))
		if got := s.all(); len(got) != 1 || got[0].Example != `SELECT 0 FROM "t0"` {
			t.Fatalf("got %+v", got)
		}
		// The 1025th shape was never tracked, so repeating it stays silent.
		h.BeforeQuery(ctx, selectEvent(`SELECT 1024 FROM "t1024"`))
		if got := s.all(); len(got) != 1 {
			t.Fatalf("a shape past the cap was counted: %+v", got)
		}
	})
}

// TestOneScopeServesBothDetectors drives both detectors with one obs.WithScope:
// a relation Load in a loop is a relation finding and a repeated statement
// finding at once.
func TestOneScopeServesBothDetectors(t *testing.T) {
	var s statements
	var mu sync.Mutex
	var rels []obs.Finding
	stmt := obs.NewStatementNPlusOne(3, s.add)
	rel := obs.NewNPlusOne(3, func(f obs.Finding) {
		mu.Lock()
		rels = append(rels, f)
		mu.Unlock()
	})
	db := momo.New(fakedriver.New(dialect.Postgres), momo.WithHook(stmt), momo.WithHook(rel))
	loadEach(t, obs.WithScope(context.Background()), db, 6)

	mu.Lock()
	defer mu.Unlock()
	if len(rels) != 1 || rels[0].Count != 4 || rels[0].Relation != "obs_parents.Kids" {
		t.Fatalf("relation findings %+v, want one with Count 4 for obs_parents.Kids", rels)
	}
	if got := s.all(); len(got) != 1 || got[0].Count != 4 {
		t.Fatalf("statement findings %+v, want one with Count 4", got)
	}
}

// TestDeprecatedForwardersMatchObs runs the same statements through
// momo.NewNPlusOneHook and obs.NewStatementNPlusOne and compares what each
// reports, in the scope of each of the two scope constructors.
func TestDeprecatedForwardersMatchObs(t *testing.T) {
	sqls := []string{
		`SELECT * FROM "a" WHERE "a"."id" = $1`,
		`SELECT * FROM "a" WHERE "a"."id" = $2`,
		`SELECT * FROM "b" WHERE "b"."id" IN ($1, $2)`,
		`SELECT * FROM "b" WHERE "b"."id" IN ($1, $2, $3)`,
		`SELECT * FROM "a" WHERE "a"."id" = $1`,
		`SELECT * FROM "c"`,
		`SELECT * FROM "a" WHERE "a"."id" = $1`,
		`SELECT * FROM "b" WHERE "b"."id" IN ($1)`,
		`SELECT * FROM "b" WHERE "b"."id" IN ($1)`,
	}
	run := func(threshold int, newScope func(context.Context) context.Context) (oldGot []forwardedFinding, newGot []obs.StatementFinding) {
		oldHook := forwardedHook(threshold, func(f forwardedFinding) { oldGot = append(oldGot, f) })
		newHook := obs.NewStatementNPlusOne(threshold, func(f obs.StatementFinding) { newGot = append(newGot, f) })
		oldCtx, newCtx := newScope(context.Background()), newScope(context.Background())
		for _, sql := range sqls {
			oldHook.BeforeQuery(oldCtx, selectEvent(sql))
			newHook.BeforeQuery(newCtx, selectEvent(sql))
		}
		return oldGot, newGot
	}
	scopes := map[string]func(context.Context) context.Context{
		"momo.WithQueryScope": forwardedScope,
		"obs.WithScope":       obs.WithScope,
	}
	for name, newScope := range scopes {
		for _, threshold := range []int{-1, 0, 1, 2, 3, 10} {
			t.Run(fmt.Sprintf("%s threshold %d", name, threshold), func(t *testing.T) {
				oldGot, newGot := run(threshold, newScope)
				if len(oldGot) != len(newGot) {
					t.Fatalf("forwarder reported %+v, obs reported %+v", oldGot, newGot)
				}
				for i := range oldGot {
					want := obs.StatementFinding{Shape: oldGot[i].Shape, Example: oldGot[i].Example, Count: oldGot[i].Count}
					if !reflect.DeepEqual(want, newGot[i]) {
						t.Fatalf("finding %d: forwarder %+v, obs %+v", i, oldGot[i], newGot[i])
					}
				}
				if threshold == 1 && len(newGot) == 0 {
					t.Fatal("the fixture reports nothing at threshold 1, so the comparison proves nothing")
				}
			})
		}
	}
}

// TestDeprecatedForwarderIgnoresControlStatementsAndPanics pins the remaining
// rules both implementations share.
func TestDeprecatedForwarderIgnoresControlStatementsAndPanics(t *testing.T) {
	var old []forwardedFinding
	h := forwardedHook(1, func(f forwardedFinding) { old = append(old, f) })
	ctx := forwardedScope(context.Background())
	for _, op := range []string{"begin", "commit", "rollback", "savepoint", "explain"} {
		for i := 0; i < 3; i++ {
			h.BeforeQuery(ctx, &momo.QueryEvent{Op: op, SQL: "X"})
		}
	}
	h.BeforeQuery(context.Background(), selectEvent("SELECT 1"))
	h.BeforeQuery(context.Background(), selectEvent("SELECT 1"))
	if len(old) != 0 {
		t.Fatalf("forwarder reported %+v", old)
	}
	boom := forwardedHook(1, func(forwardedFinding) { panic("boom") })
	for i := 0; i < 3; i++ {
		if boom.BeforeQuery(ctx, selectEvent("SELECT 2")) == nil {
			t.Fatal("BeforeQuery returned a nil context")
		}
	}
}
