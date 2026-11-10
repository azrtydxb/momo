// SPDX-License-Identifier: Apache-2.0

package obs_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/obs"
)

// always makes every statement count as slow.
const always = time.Duration(-1)

type reports struct{ qs []obs.SlowQuery }

func (r *reports) add(q obs.SlowQuery) { r.qs = append(r.qs, q) }

func (r *reports) has(prefixes ...string) bool {
	for _, q := range r.qs {
		for _, p := range prefixes {
			if strings.HasPrefix(q.SQL, p) {
				return true
			}
		}
	}
	return false
}

func explainStatements(calls []fakedriver.Call) int {
	n := 0
	for _, c := range calls {
		if strings.HasPrefix(c.SQL, "EXPLAIN ") {
			n++
		}
	}
	return n
}

// TestSlowQuerySkipsInterruptedStatements: a statement that failed because its
// context ended, or the server stopped it, gets no plan. Capturing one would
// run EXPLAIN against a connection that is already being torn down.
func TestSlowQuerySkipsInterruptedStatements(t *testing.T) {
	for name, err := range map[string]error{
		"canceled":         fmt.Errorf("select: %w", context.Canceled),
		"deadline":         fmt.Errorf("select: %w", context.DeadlineExceeded),
		"momo canceled":    &momo.Error{Kind: momo.ErrCanceled, Op: "select"},
		"statement timout": errors.New("ERROR: canceling statement due to statement timeout (SQLSTATE 57014)"),
		"mysql timeout":    errors.New("Error 3024: Query execution was interrupted, maximum statement execution time exceeded"),
	} {
		t.Run(name, func(t *testing.T) {
			drv := fakedriver.New(dialect.Postgres)
			var r reports
			h := obs.NewSlowQuery(always, r.add, obs.WithPlainExplain(momo.New(drv)))
			h.AfterQuery(t.Context(), &momo.QueryEvent{SQL: "SELECT 1", Duration: time.Second, Err: err})
			if len(r.qs) != 1 || r.qs[0].Plan != nil || !strings.Contains(r.qs[0].PlanSkipped, "cancelled or timed out") {
				t.Fatalf("report = %+v, want one report without a plan that says why", r.qs)
			}
			if n := len(drv.Calls()); n != 0 {
				t.Fatalf("%d statements issued for an interrupted statement, want none", n)
			}
		})
	}

	t.Run("a statement that succeeded is explained", func(t *testing.T) {
		drv := fakedriver.New(dialect.Postgres)
		drv.SetRows([][]any{{"Result"}})
		var r reports
		h := obs.NewSlowQuery(always, r.add, obs.WithPlainExplain(momo.New(drv)))
		h.AfterQuery(t.Context(), &momo.QueryEvent{SQL: "SELECT 1", Duration: time.Second})
		if len(r.qs) != 1 || len(r.qs[0].Plan) != 1 || explainStatements(drv.Calls()) != 1 {
			t.Fatalf("report = %+v calls = %+v, want one plan from one EXPLAIN", r.qs, drv.Calls())
		}
	})
}

// TestSlowQueryGuardInTx: the statements a capture issues on the caller's own
// transaction (savepoint, EXPLAIN, rollback to savepoint, release) pass through
// the same hook. Without the recursion guard each of them is reported and
// explained in turn, so the count never settles.
func TestSlowQueryGuardInTx(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{"Result"}})
	var r reports
	db := momo.New(drv, momo.WithHook(obs.NewSlowQuery(always, func(q obs.SlowQuery) {
		r.add(q)
		if len(r.qs) > 50 {
			t.Fatalf("the capture keeps reporting its own statements: %d reports", len(r.qs))
		}
	}, obs.WithPlainExplain(momo.New(drv)))))
	err := db.Tx(t.Context(), momo.TxOptions{}, func(tx *momo.Tx) error {
		_, err := momo.Exec(t.Context(), tx, `SELECT 1`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.has("EXPLAIN ", "SAVEPOINT", "ROLLBACK TO", "RELEASE") {
		t.Fatalf("the hook reported its own capture statements: %+v", r.qs)
	}
	var sel *obs.SlowQuery
	for i := range r.qs {
		if r.qs[i].SQL == "SELECT 1" {
			sel = &r.qs[i]
		}
	}
	if sel == nil || len(sel.Plan) == 0 {
		t.Fatalf("reports %+v, want SELECT 1 with a plan captured in the transaction", r.qs)
	}
}

// TestSlowQueryGuardOutsideTx: the querier that runs the EXPLAIN may carry a
// slow-query hook of its own; it must not report the EXPLAIN statement.
func TestSlowQueryGuardOutsideTx(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{"Result"}})
	var first, second reports
	observed := momo.New(drv, momo.WithHook(obs.NewSlowQuery(always, second.add)))
	db := momo.New(drv, momo.WithHook(obs.NewSlowQuery(always, first.add, obs.WithPlainExplain(observed))))
	if _, err := momo.Exec(t.Context(), db, `SELECT 1`); err != nil {
		t.Fatal(err)
	}
	if second.has("EXPLAIN ") {
		t.Fatalf("an EXPLAIN statement was reported: %+v", second.qs)
	}
	if len(first.qs) != 1 || len(first.qs[0].Plan) == 0 {
		t.Fatalf("first reports = %+v, want one with a plan", first.qs)
	}
}
