// SPDX-License-Identifier: Apache-2.0

//go:build integration

package bench

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo/bench/workload"
)

// The BenchmarkAllocs_ family is what the CI allocation gate compares against
// bench/baseline/allocs.txt. Allocation counts are deterministic across runs;
// timings on a shared runner are not, so only allocs/op is gated.

func allocBench(b *testing.B, op func(impl workload.Impl, i int) error) {
	b.Helper()
	for _, impl := range benchImpls(b) {
		b.Run(impl.Name(), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := op(impl, i); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkAllocs_CustomerByID(b *testing.B) {
	ctx := context.Background()
	allocBench(b, func(impl workload.Impl, i int) error {
		_, err := impl.CustomerByID(ctx, int64(i%200+1))
		return err
	})
}

func BenchmarkAllocs_InsertOrder(b *testing.B) {
	ctx := context.Background()
	allocBench(b, func(impl workload.Impl, i int) error {
		_, err := impl.InsertOrder(ctx, workload.Order{CustomerID: 1, AmountCents: int64(i), Status: "new", CreatedAt: benchTime})
		return err
	})
}

func BenchmarkAllocs_SetCustomerStatus(b *testing.B) {
	ctx := context.Background()
	allocBench(b, func(impl workload.Impl, i int) error {
		_, err := impl.SetCustomerStatus(ctx, int64(i%200+1), "active")
		return err
	})
}

func BenchmarkAllocs_RecentOrders(b *testing.B) {
	ctx := context.Background()
	allocBench(b, func(impl workload.Impl, _ int) error {
		_, err := impl.RecentOrders(ctx, "DE", 20)
		return err
	})
}
