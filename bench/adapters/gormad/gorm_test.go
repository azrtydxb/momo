// SPDX-License-Identifier: Apache-2.0

//go:build integration

package gormad_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/azrtydxb/momo/bench/adapters"
	"github.com/azrtydxb/momo/bench/adapters/gormad"
	"github.com/azrtydxb/momo/bench/workload"
)

func TestGormAdapter(t *testing.T) {
	ctx := context.Background()
	url := os.Getenv("BENCH_PG_URL")
	if url == "" {
		t.Skip("BENCH_PG_URL is not set")
	}
	gdb := gormad.Open(t, url)
	pool, err := adapters.NewPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := workload.Seed(ctx, pool, 50, 4); err != nil {
		t.Fatal(err)
	}
	impl := gormad.New(gdb)

	t.Run("missing customer maps to ErrNoRow", func(t *testing.T) {
		if _, err := impl.CustomerByID(ctx, 1<<40); !errors.Is(err, workload.ErrNoRow) {
			t.Fatalf("err = %v, want workload.ErrNoRow", err)
		}
	})

	t.Run("recent orders are newest first with id tie-break", func(t *testing.T) {
		orders, err := impl.RecentOrders(ctx, "DE", 20)
		if err != nil {
			t.Fatal(err)
		}
		for i := 1; i < len(orders); i++ {
			prev, cur := orders[i-1], orders[i]
			if prev.CreatedAt.Before(cur.CreatedAt) || (prev.CreatedAt.Equal(cur.CreatedAt) && prev.ID < cur.ID) {
				t.Fatalf("order %d (%v, id %d) sorts before %d (%v, id %d)", i-1, prev.CreatedAt, prev.ID, i, cur.CreatedAt, cur.ID)
			}
		}
	})

	t.Run("bulk insert of 1000 rows affects 1000 rows", func(t *testing.T) {
		rows := make([]workload.Order, 1000)
		for i := range rows {
			rows[i] = workload.Order{CustomerID: 1, AmountCents: int64(i), Status: "new", CreatedAt: time.Unix(1_700_000_000+int64(i), 0).UTC()}
		}
		n, err := impl.BulkInsertOrders(ctx, rows)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1000 {
			t.Fatalf("rows affected = %d, want 1000", n)
		}
	})
}
