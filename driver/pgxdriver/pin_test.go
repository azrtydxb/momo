// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/jackc/pgx/v5/pgxpool"
)

// scanOne runs a one-column query on a pinned connection and scans the value.
func scanOne(t *testing.T, p momo.PinnedConn, sql string, dest any) {
	t.Helper()
	rows, err := p.Query(context.Background(), sql, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatalf("%s returned no row: %v", sql, rows.Err())
	}
	if err := rows.Scan(dest); err != nil {
		t.Fatal(err)
	}
}

func TestPinnedConn(t *testing.T) {
	pool := openPool(t)
	d := New(pool)
	ctx := context.Background()
	p, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			_ = p.Release()
		}
	}()
	if _, err := p.Exec(ctx, "SET application_name = 'momo-pin'", nil); err != nil {
		t.Fatal(err)
	}
	rows, err := p.Query(ctx, "SHOW application_name", nil)
	if err != nil {
		t.Fatal(err)
	}
	rows.Next()
	var name string
	if err := rows.Scan(&name); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	if name != "momo-pin" {
		t.Fatalf("session state lost: application_name = %q", name)
	}
	if st := d.(momo.PoolStater).PoolStats(); st.AcquiredConns < 1 {
		t.Fatalf("AcquiredConns = %d while pinned", st.AcquiredConns)
	}
	tx, err := p.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "SELECT 1", nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := p.Release(); err != nil {
		t.Fatal(err)
	}
	released = true
	if st := d.(momo.PoolStater).PoolStats(); st.AcquiredConns != 0 {
		t.Fatalf("AcquiredConns = %d after release", st.AcquiredConns)
	}
}

// Every statement on a pinned connection, including those inside a pinned
// transaction, runs on the same server session, however busy the pool is.
func TestPinnedStatementsShareOneBackend(t *testing.T) {
	pool := openInstrumentedPool(t, 4, true)
	d := New(pool)
	ctx := context.Background()
	p, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Release() }()
	var first int
	scanOne(t, p, "SELECT pg_backend_pid()", &first)
	if _, err := p.Exec(ctx, "SET application_name = 'momo-pin-tx'", nil); err != nil {
		t.Fatal(err)
	}
	// Churn the pool so a statement that left the pinned connection would land elsewhere.
	for range 8 {
		if _, err := d.Exec(ctx, "SELECT 1", nil); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := p.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := showTx(t, tx, "application_name"); got != "momo-pin-tx" {
		t.Errorf("pinned transaction lost session state: application_name = %q", got)
	}
	rows, err := tx.Query(ctx, "SELECT pg_backend_pid()", nil)
	if err != nil {
		t.Fatal(err)
	}
	var inTx int
	rows.Next()
	if err := rows.Scan(&inTx); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if inTx != first {
		t.Fatalf("pinned transaction ran on backend %d, pinned connection is %d", inTx, first)
	}
	var after int
	scanOne(t, p, "SELECT pg_backend_pid()", &after)
	if after != first {
		t.Fatalf("statement after the transaction ran on backend %d, want %d", after, first)
	}
}

// The transaction on a pinned connection is the same wrapper as the pool's:
// it ends with ErrTxDone, and its connection stays pinned afterwards.
func TestPinnedTxReusesTxWrapper(t *testing.T) {
	d := New(openPool(t))
	ctx := context.Background()
	p, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Release() }()
	tx, err := p.Begin(ctx, momo.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tx.(*pgxTx); !ok {
		t.Fatalf("pinned Begin returned %T, want the shared *pgxTx wrapper", tx)
	}
	if got := showTx(t, tx, "transaction_read_only"); got != "on" {
		t.Errorf("transaction_read_only = %q on a read-only pinned transaction", got)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); !errors.Is(err, momo.ErrTxDone) {
		t.Errorf("second Commit err = %v, want ErrTxDone", err)
	}
	if _, err := p.Exec(ctx, "SELECT 1", nil); err != nil {
		t.Errorf("pinned connection unusable after its transaction ended: %v", err)
	}
}

func TestPinnedBeginUnknownIsolationIsUnsupported(t *testing.T) {
	d := New(openPool(t))
	ctx := context.Background()
	p, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Release() }()
	if _, err := p.Begin(ctx, momo.TxOptions{Isolation: 99}); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

// Release returns the connection once. A second Release is a no-op that
// returns nil and must not free a connection another caller has since pinned.
func TestPinnedDoubleReleaseReturnsConnectionOnce(t *testing.T) {
	pool := openInstrumentedPool(t, 1, true)
	d := New(pool)
	ctx := context.Background()
	p1, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := p1.Release(); err != nil {
		t.Fatal(err)
	}
	p2, err := d.(momo.Pinner).Pin(ctx) // now owns the only connection
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p2.Release() }()
	if err := p1.Release(); err != nil {
		t.Fatalf("second Release err = %v, want nil", err)
	}
	if st := d.(momo.PoolStater).PoolStats(); st.AcquiredConns != 1 {
		t.Fatalf("AcquiredConns = %d after a stale double Release, want 1 (p2 still holds it)", st.AcquiredConns)
	}
	var one int
	scanOne(t, p2, "SELECT 1", &one)
}

func TestPinnedUseAfterReleaseFails(t *testing.T) {
	d := New(openPool(t))
	ctx := context.Background()
	p, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Query(ctx, "SELECT 1", nil); !errors.Is(err, errReleased) {
		t.Errorf("Query after Release err = %v, want errReleased", err)
	}
	if _, err := p.Exec(ctx, "SELECT 1", nil); !errors.Is(err, errReleased) {
		t.Errorf("Exec after Release err = %v, want errReleased", err)
	}
	if _, err := p.Begin(ctx, momo.TxOptions{}); !errors.Is(err, errReleased) {
		t.Errorf("Begin after Release err = %v, want errReleased", err)
	}
}

func TestPinnedErrorsAreClassified(t *testing.T) {
	d := New(openPool(t))
	ctx := context.Background()
	p, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Release() }()
	if _, err := p.Exec(ctx, "CREATE TEMP TABLE pin_u (id int PRIMARY KEY)", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "INSERT INTO pin_u VALUES ($1)", []any{1}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "INSERT INTO pin_u VALUES ($1)", []any{1}); !errors.Is(err, momo.ErrUniqueViolation) {
		t.Errorf("Exec err = %v, want ErrUniqueViolation", err)
	}
	rows, err := p.Query(ctx, "INSERT INTO pin_u VALUES ($1)", []any{1})
	if err == nil {
		for rows.Next() {
		}
		err = rows.Err()
		_ = rows.Close()
	}
	if !errors.Is(err, momo.ErrUniqueViolation) {
		t.Errorf("Query err = %v, want ErrUniqueViolation", err)
	}
}

// Releasing a pin must not leak the connection even if a pinned transaction
// was left open: the pool discards it, so nothing stays acquired.
func TestPinnedReleaseWithOpenTxDoesNotLeak(t *testing.T) {
	pool := openInstrumentedPool(t, 1, true)
	d := New(pool)
	ctx := context.Background()
	p, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Begin(ctx, momo.TxOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := p.Release(); err != nil {
		t.Fatal(err)
	}
	// pgxpool discards a connection with an open transaction asynchronously.
	deadline := time.Now().Add(3 * time.Second)
	for pool.Stat().AcquiredConns() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("AcquiredConns = %d 3s after Release with an open tx", pool.Stat().AcquiredConns())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := d.Exec(ctx, "SELECT 1", nil); err != nil {
		t.Fatalf("pool unusable after releasing a pin with an open tx: %v", err)
	}
}

func TestPinCancelledContext(t *testing.T) {
	pool := openInstrumentedPool(t, 1, true)
	d := New(pool)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.(momo.Pinner).Pin(ctx); err == nil {
		t.Fatal("Pin with a cancelled context succeeded")
	}
	if st := pool.Stat(); st.AcquiredConns() != 0 {
		t.Fatalf("AcquiredConns = %d after a failed Pin", st.AcquiredConns())
	}
}

func TestPinRejectedExecMode(t *testing.T) {
	d := New(openPool(t), WithExecMode(-1))
	if _, err := d.(momo.Pinner).Pin(context.Background()); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("Pin err = %v, want ErrUnsupported", err)
	}
}

func TestPoolStatsReflectPool(t *testing.T) {
	var pool *pgxpool.Pool = openInstrumentedPool(t, 3, true)
	d := New(pool)
	ctx := context.Background()
	p, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Release() }()
	st := d.(momo.PoolStater).PoolStats()
	if st.MaxConns != 3 {
		t.Errorf("MaxConns = %d, want 3", st.MaxConns)
	}
	if st.AcquiredConns != 1 || st.TotalConns < 1 {
		t.Errorf("stats = %+v, want 1 acquired and at least 1 total", st)
	}
	if st.AcquireCount < 1 {
		t.Errorf("AcquireCount = %d, want >= 1", st.AcquireCount)
	}
	if want := pool.Stat().EmptyAcquireWaitTime().Nanoseconds(); st.EmptyAcquireWaitNS < 0 || st.EmptyAcquireWaitNS > want {
		t.Errorf("EmptyAcquireWaitNS = %d, pool reports %d", st.EmptyAcquireWaitNS, want)
	}
}
