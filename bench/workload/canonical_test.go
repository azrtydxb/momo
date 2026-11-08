// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"strings"
	"testing"
	"time"
)

func TestCanonical(t *testing.T) {
	ts := time.Date(2026, 11, 4, 12, 0, 0, 123456789, time.FixedZone("CET", 3600))
	got := Canonical([]Order{{ID: 2, CustomerID: 7, AmountCents: 1250, Status: "paid", CreatedAt: ts}})
	want := "2|7|1250|paid|2026-11-04T11:00:00.123456Z"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("Canonical = %q, want [%q]", got, want)
	}
}

func TestWorkloadSQLSemantics(t *testing.T) {
	for name, q := range map[string]string{
		"RecentOrders":        sqlRecentOrders,
		"CustomersWithOrders": sqlCustomersWithOrders,
		"OrdersForCustomers":  sqlOrdersForCustomers,
	} {
		if !containsAll(q, "ORDER BY") {
			t.Errorf("%s has no ORDER BY; equivalence needs deterministic ordering", name)
		}
	}
}

func TestBulkInsertSQL(t *testing.T) {
	got := BulkInsertSQL(2)
	want := "INSERT INTO bench_orders (customer_id, amount_cents, status, created_at) VALUES ($1,$2,$3,$4),($5,$6,$7,$8)"
	if got != want {
		t.Fatalf("BulkInsertSQL(2) = %q, want %q", got, want)
	}
	if !strings.HasSuffix(BulkInsertSQL(BulkChunkRows), "$8000)") {
		t.Fatalf("BulkInsertSQL(%d) does not end at parameter 8000", BulkChunkRows)
	}
}

func TestCanonicalPanicsOnForeignRow(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Canonical accepted a type without a canonical encoding")
		}
	}()
	Canonical([]string{"x"})
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
