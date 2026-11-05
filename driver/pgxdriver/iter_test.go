// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
)

// iterTable is unique per test process so concurrent test runs against the
// same database cannot collide.
var iterTable = fmt.Sprintf("iter_rows_%d", os.Getpid())

type iterRow struct{ ID int64 }

func (*iterRow) MomoMeta() *momo.Meta[iterRow] {
	return &momo.Meta[iterRow]{
		Table:   iterTable,
		Columns: []momo.ColumnMeta{{Name: "id"}},
		Ptr:     func(m *iterRow, i int) any { return &m.ID },
		Value:   func(m *iterRow, i int) any { return m.ID },
		// Index 0 would otherwise read as the soft-delete column and add
		// "id IS NULL" to every query.
		Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	}
}

func seedIter(t *testing.T, d momo.Driver) {
	t.Helper()
	ctx := context.Background()
	if _, err := d.Exec(ctx, fmt.Sprintf("CREATE TABLE %q (id bigint)", iterTable), nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.Exec(context.Background(), fmt.Sprintf("DROP TABLE IF EXISTS %q", iterTable), nil)
	})
	if _, err := d.Exec(ctx, fmt.Sprintf("INSERT INTO %q SELECT generate_series(1,100)", iterTable), nil); err != nil {
		t.Fatal(err)
	}
}

func TestIterClosesCursor(t *testing.T) {
	pool := openPool(t)
	drv := New(pool)
	seedIter(t, drv)
	db := momo.New(drv)
	q := momo.Select[iterRow](db)
	ctx := context.Background()
	stats := func() int { return drv.(momo.PoolStater).PoolStats().AcquiredConns }
	goroutines := runtime.NumGoroutine()

	t.Run("break", func(t *testing.T) {
		for _, err := range q.Iter(ctx) {
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		if n := stats(); n != 0 {
			t.Fatalf("cursor left open after break: %d acquired", n)
		}
	})
	t.Run("panic in loop body", func(t *testing.T) {
		func() {
			defer func() {
				if r := recover(); r != "boom" {
					t.Fatalf("recovered %v, want boom", r)
				}
			}()
			for _, err := range q.Iter(ctx) {
				if err != nil {
					t.Fatal(err)
				}
				panic("boom")
			}
		}()
		if n := stats(); n != 0 {
			t.Fatalf("cursor left open after panic: %d acquired", n)
		}
	})
	t.Run("panic in Each callback", func(t *testing.T) {
		func() {
			defer func() { _ = recover() }()
			_ = q.Each(ctx, func(*iterRow) error { panic("boom") })
		}()
		if n := stats(); n != 0 {
			t.Fatalf("cursor left open after Each panic: %d acquired", n)
		}
	})
	t.Run("callback error", func(t *testing.T) {
		sentinel := errors.New("stop")
		err := q.Each(ctx, func(*iterRow) error { return sentinel })
		if !errors.Is(err, sentinel) {
			t.Fatalf("err = %v", err)
		}
		if n := stats(); n != 0 {
			t.Fatalf("cursor left open after callback error: %d", n)
		}
	})
	t.Run("cancelled Each", func(t *testing.T) {
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()
		seen := 0
		err := q.Each(cctx, func(*iterRow) error {
			seen++
			if seen == 3 {
				cancel()
			}
			return nil
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Each after cancel err = %v, want context.Canceled", err)
		}
		if n := stats(); n != 0 {
			t.Fatalf("cursor left open after cancel: %d", n)
		}
	})
	t.Run("cancelled Iter", func(t *testing.T) {
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()
		seen := 0
		var last error
		for _, err := range q.Iter(cctx) {
			last = err
			if err != nil {
				break
			}
			seen++
			if seen == 3 {
				cancel()
			}
		}
		if !errors.Is(last, context.Canceled) {
			t.Fatalf("Iter after cancel err = %v, want context.Canceled", last)
		}
		if n := stats(); n != 0 {
			t.Fatalf("cursor left open after cancel: %d", n)
		}
	})
	t.Run("pinned connection", func(t *testing.T) {
		pc, err := drv.(momo.Pinner).Pin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		pdb := db.OnConn(pc)
		for _, err := range momo.Select[iterRow](pdb).Iter(ctx) {
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		// An open cursor would make Release report the connection busy.
		if err := pc.Release(); err != nil {
			t.Fatalf("Release after early exit: %v", err)
		}
		if n := stats(); n != 0 {
			t.Fatalf("pinned connection leaked: %d acquired", n)
		}
	})
	t.Run("transaction stays usable", func(t *testing.T) {
		err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			for _, err := range momo.Select[iterRow](tx).Iter(ctx) {
				if err != nil {
					return err
				}
				break
			}
			sentinel := errors.New("stop")
			if err := momo.Select[iterRow](tx).Each(ctx, func(*iterRow) error { return sentinel }); !errors.Is(err, sentinel) {
				return fmt.Errorf("Each err = %v", err)
			}
			n, err := momo.Select[iterRow](tx).Count(ctx)
			if err != nil {
				return fmt.Errorf("tx unusable after early exit: %w", err)
			}
			if n != 100 {
				return fmt.Errorf("count = %d, want 100", n)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if n := stats(); n != 0 {
			t.Fatalf("tx leaked: %d acquired", n)
		}
	})

	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > goroutines && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > goroutines {
		t.Fatalf("goroutines %d -> %d: leak", goroutines, got)
	}
}
