// SPDX-License-Identifier: Apache-2.0

//go:build integration

package bench

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/azrtydxb/momo/bench/adapters"
	"github.com/azrtydxb/momo/bench/workload"
)

// flattenWithOrders encodes customers with their orders in one canonical list.
func flattenWithOrders(cs []workload.CustomerWithOrders) []string {
	var out []string
	for _, cw := range cs {
		out = append(out, workload.Canonical([]workload.Customer{cw.Customer})...)
		out = append(out, workload.Canonical(cw.Orders)...)
	}
	return out
}

// TestBenchEquivalence seeds one database, takes the handwritten pgx adapter as
// the reference, and checks that every other adapter returns identical
// canonical rows for every read and leaves identical state after every write.
func TestBenchEquivalence(t *testing.T) {
	ctx := context.Background()
	url := os.Getenv("BENCH_PG_URL")
	if url == "" {
		t.Skip("BENCH_PG_URL is not set")
	}
	pool, err := adapters.NewPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := workload.Seed(ctx, pool, 200, 5); err != nil {
		t.Fatal(err)
	}
	impls := openAllImpls(t, url, pool)
	ref, others := impls[0], impls[1:]
	for _, impl := range others {
		t.Run(impl.Name(), func(t *testing.T) {
			for _, country := range []string{"DE", "FR"} {
				want, err := ref.RecentOrders(ctx, country, 20)
				if err != nil {
					t.Fatal(err)
				}
				got, err := impl.RecentOrders(ctx, country, 20)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(workload.Canonical(got), workload.Canonical(want)) {
					t.Fatalf("RecentOrders(%s) differs from the pgx reference:\n got %v\nwant %v", country, workload.Canonical(got), workload.Canonical(want))
				}
			}

			wantC, err := ref.CustomerByID(ctx, 3)
			if err != nil {
				t.Fatal(err)
			}
			gotC, err := impl.CustomerByID(ctx, 3)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(workload.Canonical([]workload.Customer{gotC}), workload.Canonical([]workload.Customer{wantC})) {
				t.Fatalf("CustomerByID differs: got %+v want %+v", gotC, wantC)
			}
			if _, err := impl.CustomerByID(ctx, 999999); !errors.Is(err, workload.ErrNoRow) {
				t.Fatalf("missing customer error = %v, want ErrNoRow", err)
			}

			wantCW, err := ref.CustomersWithOrders(ctx, "DE", 10)
			if err != nil {
				t.Fatal(err)
			}
			gotCW, err := impl.CustomersWithOrders(ctx, "DE", 10)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(flattenWithOrders(gotCW), flattenWithOrders(wantCW)) {
				t.Fatalf("CustomersWithOrders differs from the pgx reference:\n got %v\nwant %v", flattenWithOrders(gotCW), flattenWithOrders(wantCW))
			}

			// Writes: the same status change must be visible through the reference.
			n, err := impl.SetCustomerStatus(ctx, 2, "vip")
			if err != nil {
				t.Fatal(err)
			}
			if n != 1 {
				t.Fatalf("SetCustomerStatus affected %d rows, want 1", n)
			}
			c, err := ref.CustomerByID(ctx, 2)
			if err != nil {
				t.Fatal(err)
			}
			if c.Status != "vip" {
				t.Fatalf("reference sees status %q after %s write, want vip", c.Status, impl.Name())
			}
			if _, err := ref.SetCustomerStatus(ctx, 2, "active"); err != nil {
				t.Fatal(err)
			}

			id, err := impl.InsertOrder(ctx, workload.Order{CustomerID: 2, AmountCents: 777, Status: "paid", CreatedAt: time.Now()})
			if err != nil || id < 1 {
				t.Fatalf("InsertOrder = %d, %v", id, err)
			}
			bulk := make([]workload.Order, 2500)
			for k := range bulk {
				bulk[k] = workload.Order{CustomerID: int64(1 + k%200), AmountCents: int64(k), Status: "new", CreatedAt: time.Now()}
			}
			n, err = impl.BulkInsertOrders(ctx, bulk)
			if err != nil || n != int64(len(bulk)) {
				t.Fatalf("BulkInsertOrders = %d, %v; want %d", n, err, len(bulk))
			}
		})
	}
}
