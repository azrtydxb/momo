// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momotest

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
)

// TestAssertUsesIndexPostgres plans a lookup on a real PostgreSQL table: with
// the index the assertion passes, without it the plan is a sequential scan and
// the assertion fails.
func TestAssertUsesIndexPostgres(t *testing.T) {
	ctx := t.Context()
	db := New(t)
	for _, ddl := range []string{
		`CREATE TABLE stock (id bigint PRIMARY KEY, sku text NOT NULL, qty int NOT NULL)`,
		`INSERT INTO stock (id, sku, qty) SELECT n, 'sku-' || n, n % 7 FROM generate_series(1, 5000) AS n`,
		`CREATE INDEX "Stock_Sku_Idx" ON stock (sku)`,
		`ANALYZE stock`,
	} {
		if _, err := momo.Exec(ctx, db, ddl); err != nil {
			t.Fatal(err)
		}
	}
	lookup := momo.Compiled{SQL: `SELECT id FROM stock WHERE sku = $1`, Args: []any{"sku-42"}}

	rt := &recTB{TB: t}
	runAssert(func() { AssertUsesIndex(rt, db, lookup, "Stock_Sku_Idx") })
	if len(rt.errs) != 0 {
		t.Fatalf("indexed lookup failed: %v", rt.errs)
	}

	if _, err := momo.Exec(ctx, db, `DROP INDEX "Stock_Sku_Idx"`); err != nil {
		t.Fatal(err)
	}
	rt = &recTB{TB: t}
	runAssert(func() { AssertUsesIndex(rt, db, lookup, "Stock_Sku_Idx") })
	if len(rt.errs) != 1 || !strings.Contains(rt.errs[0], "sequential scan") {
		t.Fatalf("errors = %v, want one mentioning a sequential scan", rt.errs)
	}

	// The lookup was only planned: a statement that writes is not executed.
	del := momo.Compiled{SQL: `DELETE FROM stock WHERE sku = $1`, Args: []any{"sku-42"}}
	rt = &recTB{TB: t}
	runAssert(func() { AssertUsesIndex(rt, db, del, "stock_pkey") })
	var n int64
	rows, err := db.Driver().Query(ctx, `SELECT count(*) FROM stock WHERE sku = 'sku-42'`, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() || rows.Scan(&n) != nil || n != 1 {
		t.Fatalf("the planned DELETE ran: rows left = %d", n)
	}
}
