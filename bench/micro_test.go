// SPDX-License-Identifier: Apache-2.0

//go:build integration

package bench

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/azrtydxb/momo/bench/adapters"
	"github.com/azrtydxb/momo/bench/stats"
	"github.com/azrtydxb/momo/bench/workload"
)

// benchImpls seeds the database and opens the six adapters on the shared pool;
// each benchmark calls it once.
func benchImpls(b *testing.B) []workload.Impl {
	b.Helper()
	url := os.Getenv("BENCH_PG_URL")
	if url == "" {
		b.Skip("BENCH_PG_URL is not set")
	}
	ctx := context.Background()
	pool, err := adapters.NewPool(ctx, url)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(pool.Close)
	if err := workload.Seed(ctx, pool, 200, 5); err != nil {
		b.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "ANALYZE"); err != nil {
		b.Fatal(err)
	}
	return openAllImpls(b, url, pool)
}

func timed(b *testing.B, op func(i int) error) {
	b.Helper()
	samples := make([]time.Duration, 0, b.N)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		if err := op(i); err != nil {
			b.Fatal(err)
		}
		samples = append(samples, time.Since(start))
	}
	b.StopTimer()
	stats.Report(b, samples)
}

var benchTime = time.Unix(1_700_000_000, 0).UTC()

func BenchmarkMicro(b *testing.B) {
	ctx := context.Background()
	for _, impl := range benchImpls(b) {
		b.Run(impl.Name()+"/CustomerByID", func(b *testing.B) {
			timed(b, func(i int) error {
				_, err := impl.CustomerByID(ctx, int64(i%200+1))
				return err
			})
		})
		b.Run(impl.Name()+"/RecentOrders", func(b *testing.B) {
			timed(b, func(int) error {
				_, err := impl.RecentOrders(ctx, "DE", 20)
				return err
			})
		})
		b.Run(impl.Name()+"/InsertOrder", func(b *testing.B) {
			timed(b, func(i int) error {
				_, err := impl.InsertOrder(ctx, workload.Order{CustomerID: 1, AmountCents: int64(i), Status: "new", CreatedAt: benchTime})
				return err
			})
		})
		b.Run(impl.Name()+"/SetCustomerStatus", func(b *testing.B) {
			timed(b, func(i int) error {
				_, err := impl.SetCustomerStatus(ctx, int64(i%200+1), "active")
				return err
			})
		})
		b.Run(impl.Name()+"/CustomersWithOrders", func(b *testing.B) {
			timed(b, func(int) error {
				_, err := impl.CustomersWithOrders(ctx, "DE", 100)
				return err
			})
		})
		b.Run(impl.Name()+"/BulkInsertOrders", func(b *testing.B) {
			rows := make([]workload.Order, 1000)
			for i := range rows {
				rows[i] = workload.Order{CustomerID: 1, AmountCents: int64(i), Status: "new", CreatedAt: benchTime}
			}
			timed(b, func(int) error {
				_, err := impl.BulkInsertOrders(ctx, rows)
				return err
			})
		})
	}
}
