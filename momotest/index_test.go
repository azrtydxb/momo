// SPDX-License-Identifier: Apache-2.0

package momotest

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// TestAssertUsesIndex drives AssertUsesIndex with scripted plans: it passes for
// an indexed lookup, fails for a sequential scan, and runs a plain EXPLAIN
// only (the statement itself is never sent).
func TestAssertUsesIndex(t *testing.T) {
	lookup := momo.Compiled{SQL: `SELECT id FROM items WHERE sku = $1`, Args: []any{"a"}}

	run := func(d dialect.Name, rows [][]any, index string) (*recTB, *fakedriver.Driver) {
		drv := fakedriver.New(d)
		drv.SetRows(rows)
		rt := &recTB{TB: t}
		runAssert(func() { AssertUsesIndex(rt, momo.New(drv), lookup, index) })
		return rt, drv
	}

	t.Run("indexed lookup passes", func(t *testing.T) {
		rt, drv := run(dialect.Postgres, [][]any{{"Index Scan using items_sku_idx on items  (cost=0.15..8.17 rows=1 width=4)"}, {"  Index Cond: (sku = $1)"}}, "items_sku_idx")
		if len(rt.errs) != 0 {
			t.Fatalf("unexpected failure: %v", rt.errs)
		}
		if rt.helpers == 0 {
			t.Fatal("AssertUsesIndex did not call tb.Helper")
		}
		calls := drv.Calls()
		if len(calls) != 1 || calls[0].SQL != "EXPLAIN "+lookup.SQL || strings.Contains(strings.ToUpper(calls[0].SQL), "ANALYZE") {
			t.Fatalf("calls = %+v, want one plain EXPLAIN of the statement", calls)
		}
	})
	t.Run("sqlite covering index passes", func(t *testing.T) {
		rt, drv := run(dialect.SQLite, [][]any{{int64(2), int64(0), int64(0), "SEARCH items USING COVERING INDEX items_sku_idx (sku=?)"}}, "items_sku_idx")
		if len(rt.errs) != 0 {
			t.Fatalf("unexpected failure: %v", rt.errs)
		}
		if got := drv.Calls()[0].SQL; got != "EXPLAIN QUERY PLAN "+lookup.SQL {
			t.Fatalf("SQL = %q, want EXPLAIN QUERY PLAN", got)
		}
	})
	t.Run("sequential scan fails and prints the plan", func(t *testing.T) {
		rt, _ := run(dialect.Postgres, [][]any{{"Seq Scan on items  (cost=0.00..1.04 rows=1 width=4)"}, {"  Filter: (sku = $1)"}}, "items_sku_idx")
		if len(rt.errs) != 1 || !strings.Contains(rt.errs[0], "sequential scan") || !strings.Contains(rt.errs[0], "Seq Scan on items") {
			t.Fatalf("errors = %v, want one naming a sequential scan and showing the plan", rt.errs)
		}
	})
	t.Run("a different index fails", func(t *testing.T) {
		rt, _ := run(dialect.Postgres, [][]any{{"Index Scan using items_sku_idx on items"}}, "items_other_idx")
		if len(rt.errs) != 1 || !strings.Contains(rt.errs[0], `"items_other_idx"`) || strings.Contains(rt.errs[0], "sequential scan") {
			t.Fatalf("errors = %v, want one naming the missing index", rt.errs)
		}
	})
	t.Run("a longer index name is not a match", func(t *testing.T) {
		rt, _ := run(dialect.Postgres, [][]any{{"Index Scan using items_sku_idx2 on items"}}, "items_sku_idx")
		if len(rt.errs) != 1 {
			t.Fatalf("errors = %v, want a failure for a prefix of the used index", rt.errs)
		}
	})
	t.Run("an unsupported dialect fails the test", func(t *testing.T) {
		rt, drv := run(dialect.MySQL, nil, "items_sku_idx")
		if len(rt.errs) != 1 || !strings.Contains(rt.errs[0], "unsupported dialect") {
			t.Fatalf("errors = %v, want an unsupported dialect failure", rt.errs)
		}
		if n := len(drv.Calls()); n != 0 {
			t.Fatalf("an unsupported dialect issued %d statements", n)
		}
	})
}
