// SPDX-License-Identifier: Apache-2.0

//go:build integration

package sqlitetest_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/momotest"
	"github.com/azrtydxb/momo/momotest/sqlitetest"
)

// recTB records failures instead of failing the real test. Fatalf also panics
// (see runAssert) so the code after it does not run, as with a real TB.
type recTB struct {
	testing.TB
	errs []string
}

func (r *recTB) Helper() {}

func (r *recTB) Errorf(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
}

type fatalSentinel struct{}

func (r *recTB) Fatalf(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
	panic(fatalSentinel{})
}

// runAssert runs fn and absorbs the panic a recTB.Fatalf raises.
func runAssert(fn func()) {
	defer func() {
		if p := recover(); p != nil {
			if _, ok := p.(fatalSentinel); !ok {
				panic(p)
			}
		}
	}()
	fn()
}

// TestAssertUsesIndexSQLite plans a lookup on a real SQLite table.
func TestAssertUsesIndexSQLite(t *testing.T) {
	ctx := t.Context()
	db := sqlitetest.New(t)
	for _, ddl := range []string{
		`CREATE TABLE items (id INTEGER PRIMARY KEY, sku TEXT NOT NULL, qty INTEGER NOT NULL)`,
		`CREATE INDEX items_sku_idx ON items (sku)`,
	} {
		if _, err := momo.Exec(ctx, db, ddl); err != nil {
			t.Fatal(err)
		}
	}
	indexed := momo.Compiled{SQL: `SELECT id FROM items WHERE sku = ?`, Args: []any{"a"}}
	scan := momo.Compiled{SQL: `SELECT id FROM items WHERE qty = ?`, Args: []any{1}}

	rt := &recTB{TB: t}
	runAssert(func() { momotest.AssertUsesIndex(rt, db, indexed, "items_sku_idx") })
	if len(rt.errs) != 0 {
		t.Fatalf("indexed lookup failed: %v", rt.errs)
	}
	rt = &recTB{TB: t}
	runAssert(func() { momotest.AssertUsesIndex(rt, db, scan, "items_sku_idx") })
	if len(rt.errs) != 1 || !strings.Contains(rt.errs[0], "sequential scan") {
		t.Fatalf("errors = %v, want one mentioning a sequential scan", rt.errs)
	}
	rt = &recTB{TB: t}
	runAssert(func() { momotest.AssertUsesIndex(rt, db, indexed, "items_other_idx") })
	if len(rt.errs) != 1 || !strings.Contains(rt.errs[0], `"items_other_idx"`) {
		t.Fatalf("errors = %v, want one naming the missing index", rt.errs)
	}
}
