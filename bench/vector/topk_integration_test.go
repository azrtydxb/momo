// SPDX-License-Identifier: Apache-2.0

//go:build integration

package vector

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	models "github.com/azrtydxb/momo/bench/models/momo"
	"github.com/azrtydxb/momo/bench/models/momo/item"
	"github.com/azrtydxb/momo/bench/stats"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/vecsearch"
	momovector "github.com/azrtydxb/momo/vector"
)

func datasetSize(b *testing.B) int {
	b.Helper()
	if s := os.Getenv("BENCH_VECTOR_N"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			b.Fatal(err)
		}
		return n
	}
	return 100000
}

// BenchmarkVectorTopK compares the momo query builder with a hand-written
// statement at the ef_search a recall sweep chose on the same data, so both
// run at the same recall. The dataset size comes from BENCH_VECTOR_N (default
// 100000, dimension 384).
func BenchmarkVectorTopK(b *testing.B) {
	ctx := context.Background()
	pool := oneConnPool(b)
	ds := Generate(datasetSize(b), 100, 384, 7)
	loadItems(b, ctx, pool, "topk_items", ds)
	raw := pgSweepDB{pool: pool, table: "topk_items"}
	ef, recall, err := Sweep(ctx, raw, ds, 10, 0.95, []int{10, 20, 40, 80, 160, 320, 640})
	if err != nil {
		b.Fatal(err)
	}

	b.Run("pgxraw", func(b *testing.B) {
		samples := make([]time.Duration, 0, b.N)
		b.ReportAllocs()
		b.ResetTimer()
		err := raw.WithSearchEf(ctx, ef, func() error {
			for i := 0; i < b.N; i++ {
				start := time.Now()
				if _, err := raw.TopK(ctx, ds.Queries[i%len(ds.Queries)], 10); err != nil {
					return err
				}
				samples = append(samples, time.Since(start))
			}
			return nil
		})
		b.StopTimer()
		if err != nil {
			b.Fatal(err)
		}
		stats.Report(b, samples)
		b.ReportMetric(recall, "recall@10")
	})

	b.Run("momo", func(b *testing.B) {
		momoPool := oneConnPool(b)
		db := momo.New(pgxdriver.New(momoPool))
		samples := make([]time.Duration, 0, b.N)
		b.ReportAllocs()
		b.ResetTimer()
		err := vecsearch.With(ctx, db, vecsearch.Settings{HNSWEfSearch: ef}, func(tx *momo.Tx) error {
			for i := 0; i < b.N; i++ {
				q := momovector.Vector(ds.Queries[i%len(ds.Queries)])
				start := time.Now()
				_, err := momo.Select[models.Item](tx).
					OrderBy(momo.L2Distance(item.Embedding, q).Asc(), item.ID.Asc()).
					Limit(10).
					All(ctx)
				if err != nil {
					return err
				}
				samples = append(samples, time.Since(start))
			}
			return nil
		})
		b.StopTimer()
		if err != nil {
			b.Fatal(err)
		}
		stats.Report(b, samples)
		b.ReportMetric(recall, "recall@10")
	})
}
