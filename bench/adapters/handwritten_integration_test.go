// SPDX-License-Identifier: Apache-2.0

//go:build integration

package adapters_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/azrtydxb/momo/bench/adapters"
	"github.com/azrtydxb/momo/bench/adapters/pgxhand"
	"github.com/azrtydxb/momo/bench/adapters/sqlhand"
	"github.com/azrtydxb/momo/bench/workload"
)

func openImpls(t *testing.T) (context.Context, []workload.Impl) {
	t.Helper()
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
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	adapters.ApplySQLDBSettings(db)
	return ctx, []workload.Impl{pgxhand.New(pool), sqlhand.New(db)}
}

func TestSeedIsDeterministic(t *testing.T) {
	ctx, impls := openImpls(t)
	first, err := impls[0].RecentOrders(ctx, "DE", 10)
	if err != nil {
		t.Fatal(err)
	}
	url := os.Getenv("BENCH_PG_URL")
	pool, err := adapters.NewPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := workload.Seed(ctx, pool, 200, 5); err != nil {
		t.Fatal(err)
	}
	second, err := impls[0].RecentOrders(ctx, "DE", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(workload.Canonical(first), workload.Canonical(second)) {
		t.Fatalf("seed is not deterministic:\n%v\n%v", workload.Canonical(first), workload.Canonical(second))
	}
}

func TestHandwrittenAdaptersAgree(t *testing.T) {
	ctx, impls := openImpls(t)

	var customers [][]string
	var recents [][]string
	var withOrders [][]string
	for _, impl := range impls {
		c, err := impl.CustomerByID(ctx, 1)
		if err != nil {
			t.Fatalf("%s CustomerByID: %v", impl.Name(), err)
		}
		customers = append(customers, workload.Canonical([]workload.Customer{c}))

		r, err := impl.RecentOrders(ctx, "DE", 20)
		if err != nil {
			t.Fatalf("%s RecentOrders: %v", impl.Name(), err)
		}
		if len(r) == 0 {
			t.Fatalf("%s RecentOrders returned no rows", impl.Name())
		}
		for i := 1; i < len(r); i++ {
			prev, cur := r[i-1], r[i]
			if cur.CreatedAt.After(prev.CreatedAt) || (cur.CreatedAt.Equal(prev.CreatedAt) && cur.ID > prev.ID) {
				t.Fatalf("%s RecentOrders not ordered by created_at DESC, id DESC at %d", impl.Name(), i)
			}
		}
		recents = append(recents, workload.Canonical(r))

		w, err := impl.CustomersWithOrders(ctx, "DE", 5)
		if err != nil {
			t.Fatalf("%s CustomersWithOrders: %v", impl.Name(), err)
		}
		var lines []string
		for _, cw := range w {
			lines = append(lines, workload.Canonical([]workload.Customer{cw.Customer})...)
			for i := 1; i < len(cw.Orders); i++ {
				if cw.Orders[i].ID <= cw.Orders[i-1].ID {
					t.Fatalf("%s orders of customer %d not ascending by id", impl.Name(), cw.Customer.ID)
				}
			}
			lines = append(lines, workload.Canonical(cw.Orders)...)
		}
		withOrders = append(withOrders, lines)
	}
	for _, set := range []struct {
		name string
		rows [][]string
	}{{"CustomerByID", customers}, {"RecentOrders", recents}, {"CustomersWithOrders", withOrders}} {
		if !reflect.DeepEqual(set.rows[0], set.rows[1]) {
			t.Errorf("%s differs between %s and %s:\n%v\n%v", set.name, impls[0].Name(), impls[1].Name(), set.rows[0], set.rows[1])
		}
	}
}

func TestHandwrittenAdaptersWrite(t *testing.T) {
	ctx, impls := openImpls(t)
	for _, impl := range impls {
		t.Run(impl.Name(), func(t *testing.T) {
			if _, err := impl.CustomerByID(ctx, 999999); !errors.Is(err, workload.ErrNoRow) {
				t.Fatalf("missing customer error = %v, want ErrNoRow", err)
			}
			id, err := impl.InsertOrder(ctx, workload.Order{CustomerID: 1, AmountCents: 500, Status: "new", CreatedAt: time.Now()})
			if err != nil || id < 1 {
				t.Fatalf("InsertOrder = %d, %v", id, err)
			}
			n, err := impl.SetCustomerStatus(ctx, 1, "inactive")
			if err != nil || n != 1 {
				t.Fatalf("SetCustomerStatus = %d, %v; want 1 row", n, err)
			}
			n, err = impl.SetCustomerStatus(ctx, 999999, "inactive")
			if err != nil || n != 0 {
				t.Fatalf("SetCustomerStatus on a missing row = %d, %v; want 0 rows", n, err)
			}
			// 4500 rows span three chunks of workload.BulkChunkRows.
			rows := make([]workload.Order, 4500)
			for i := range rows {
				rows[i] = workload.Order{CustomerID: int64(1 + i%200), AmountCents: int64(i), Status: "paid", CreatedAt: time.Now()}
			}
			n, err = impl.BulkInsertOrders(ctx, rows)
			if err != nil || n != int64(len(rows)) {
				t.Fatalf("BulkInsertOrders = %d, %v; want %d", n, err, len(rows))
			}
		})
	}
}
