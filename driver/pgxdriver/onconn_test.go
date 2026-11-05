// SPDX-License-Identifier: Apache-2.0

package pgxdriver_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/testdb"
)

type countHook struct{ n int }

func (c *countHook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
	c.n++
	return ctx
}

func (c *countHook) AfterQuery(ctx context.Context, e *momo.QueryEvent) {}

// openDB builds a *momo.DB on the test Postgres. internal/testdb/testopen
// (momo-03 Task 1) is not on this branch yet; it replaces this helper.
func openDB(t *testing.T, opts ...momo.Option) *momo.DB {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxdriver.NewPool(ctx, testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// pool.Close blocks on a leaked connection; fail fast instead.
		done := make(chan struct{})
		go func() { pool.Close(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Errorf("pool.Close blocked for 5s: %d connection(s) leaked", pool.Stat().AcquiredConns())
		}
	})
	return momo.New(pgxdriver.New(pool), opts...)
}

func TestOnConnSessionAndHooks(t *testing.T) {
	hook := &countHook{}
	db := openDB(t, momo.WithHook(hook))
	ctx := context.Background()
	pc, err := db.Driver().(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := pc.Release(); err != nil {
			t.Error(err)
		}
	})
	oc := db.OnConn(pc)
	if _, err := momo.Exec(ctx, oc, "SET application_name = 'momo-onconn'"); err != nil {
		t.Fatal(err)
	}
	rows, err := pc.Query(ctx, "SHOW application_name", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatalf("SHOW returned no row: %v", rows.Err())
	}
	var name string
	if err := rows.Scan(&name); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if name != "momo-onconn" {
		t.Fatalf("statement through OnConn did not set session state on the pinned connection: %q", name)
	}
	before := hook.n
	if _, err := momo.Exec(ctx, oc, "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	if hook.n != before+1 {
		t.Fatalf("hook registered on db fired %d times for an OnConn statement, want 1", hook.n-before)
	}
	if _, err := oc.Begin(ctx, momo.TxOptions{}); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("Begin on a pinned handle err = %v, want ErrUnsupported", err)
	}
	if err := oc.Tx(ctx, momo.TxOptions{}, func(*momo.Tx) error { return nil }); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("Tx on a pinned handle err = %v, want ErrUnsupported", err)
	}
	// The original handle is untouched: it still begins transactions on the pool.
	tx, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatalf("Begin on the original db after OnConn: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
}
