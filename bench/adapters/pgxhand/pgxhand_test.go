// SPDX-License-Identifier: Apache-2.0

//go:build integration

package pgxhand_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/bench/adapters"
	"github.com/azrtydxb/momo/bench/adapters/pgxhand"
	"github.com/azrtydxb/momo/bench/workload"
)

const goldenPath = "../../workload/testdata/golden/pgx.txt"

func TestPgxhandGolden(t *testing.T) {
	url := os.Getenv("BENCH_PG_URL")
	if url == "" {
		t.Skip("BENCH_PG_URL is not set")
	}
	ctx := context.Background()
	pool, err := adapters.NewPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := workload.Seed(ctx, pool, 200, 5); err != nil {
		t.Fatal(err)
	}
	impl := pgxhand.New(pool)

	customer, err := impl.CustomerByID(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	recent, err := impl.RecentOrders(ctx, "DE", 5)
	if err != nil {
		t.Fatal(err)
	}
	withOrders, err := impl.CustomersWithOrders(ctx, "DE", 3)
	if err != nil {
		t.Fatal(err)
	}

	var lines []string
	lines = append(lines, "customer:")
	lines = append(lines, workload.Canonical([]workload.Customer{customer})...)
	lines = append(lines, "recent:")
	lines = append(lines, workload.Canonical(recent)...)
	lines = append(lines, "with-orders:")
	for _, cw := range withOrders {
		lines = append(lines, workload.Canonical([]workload.Customer{cw.Customer})...)
		lines = append(lines, workload.Canonical(cw.Orders)...)
	}
	got := strings.Join(lines, "\n") + "\n"

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(goldenPath, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with UPDATE_GOLDEN=1 once): %v", err)
	}
	if got != string(want) {
		t.Fatalf("golden mismatch:\n--- got\n%s--- want\n%s", got, want)
	}
}
