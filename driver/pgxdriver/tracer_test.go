// SPDX-License-Identifier: Apache-2.0

//go:build integration

package pgxdriver

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// poolWaitHook keeps the largest PoolWait it sees.
type poolWaitHook struct{ maxNS atomic.Int64 }

func (h *poolWaitHook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
	return ctx
}

func (h *poolWaitHook) AfterQuery(ctx context.Context, e *momo.QueryEvent) {
	for {
		cur := h.maxNS.Load()
		if int64(e.PoolWait) <= cur || h.maxNS.CompareAndSwap(cur, int64(e.PoolWait)) {
			return
		}
	}
}

func openInstrumentedPool(t *testing.T, maxConns int32, instrument bool) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = maxConns
	if instrument {
		InstrumentPool(cfg)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closePool(t, pool) })
	return pool
}

// blockedQueryWait holds the only connection of a one-connection pool, runs a
// statement that must wait for it, and returns the largest PoolWait reported.
func blockedQueryWait(t *testing.T, instrument bool) time.Duration {
	t.Helper()
	ctx := context.Background()
	pool := openInstrumentedPool(t, 1, instrument)
	hook := &poolWaitHook{}
	db := momo.New(New(pool), momo.WithHook(hook))
	held, err := New(pool).(momo.Pinner).Pin(ctx) // holds the only connection
	if err != nil {
		t.Fatal(err)
	}
	released := false
	t.Cleanup(func() {
		if !released {
			_ = held.Release()
		}
	})
	done := make(chan error, 1)
	go func() {
		_, err := momo.Exec(ctx, db, "SELECT 1") // waits for the pool
		done <- err
	}()
	time.Sleep(100 * time.Millisecond)
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	released = true
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	return time.Duration(hook.maxNS.Load())
}

func TestPoolWaitRecordedWhenExhausted(t *testing.T) {
	if got := blockedQueryWait(t, true); got < 50*time.Millisecond {
		t.Fatalf("PoolWait = %v, want the blocked query to report its wait of about 100ms", got)
	}
}

// Mutation partner of the test above: without InstrumentPool nothing is
// recorded, which proves the recorded wait comes from the tracer.
func TestPoolWaitNotRecordedWithoutInstrumentation(t *testing.T) {
	if got := blockedQueryWait(t, false); got != 0 {
		t.Fatalf("PoolWait = %v without InstrumentPool, want 0", got)
	}
}

// An acquire that finds an idle connection did not wait and must not record
// anything.
func TestPoolWaitNotRecordedWhenIdle(t *testing.T) {
	ctx := context.Background()
	pool := openInstrumentedPool(t, 2, true)
	hook := &poolWaitHook{}
	db := momo.New(New(pool), momo.WithHook(hook))
	if _, err := momo.Exec(ctx, db, "SELECT 1"); err != nil { // dials the first connection
		t.Fatal(err)
	}
	hook.maxNS.Store(0)
	for range 5 {
		if _, err := momo.Exec(ctx, db, "SELECT 1"); err != nil {
			t.Fatal(err)
		}
	}
	if got := hook.maxNS.Load(); got != 0 {
		t.Fatalf("PoolWait = %v on warm idle acquires, want 0", time.Duration(got))
	}
}

// listWaitHook keeps every PoolWait it sees.
type listWaitHook struct {
	mu    sync.Mutex
	waits []time.Duration
}

func (h *listWaitHook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
	return ctx
}

func (h *listWaitHook) AfterQuery(ctx context.Context, e *momo.QueryEvent) {
	h.mu.Lock()
	h.waits = append(h.waits, e.PoolWait)
	h.mu.Unlock()
}

func (h *listWaitHook) take() []time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	w := h.waits
	h.waits = nil
	return w
}

// startCountingTracer counts acquires that have started, then delegates.
type startCountingTracer struct {
	*poolTracer
	starts atomic.Int64
}

func (c *startCountingTracer) TraceAcquireStart(ctx context.Context, pool *pgxpool.Pool, d pgxpool.TraceAcquireStartData) context.Context {
	c.starts.Add(1)
	return c.poolTracer.TraceAcquireStart(ctx, pool, d)
}

// With the only connection held, N concurrent statements all wait: every one
// must record PoolWait > 0, and none may report more than the real elapsed
// time. Afterwards idle acquires must not record bogus waits.
func TestPoolWaitConcurrentAcquires(t *testing.T) {
	const n = 8
	ctx := context.Background()
	tr := &startCountingTracer{poolTracer: &poolTracer{}}
	cfg, err := pgxpool.ParseConfig(testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	cfg.ConnConfig.Tracer = tr
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closePool(t, pool) })
	hook := &listWaitHook{}
	db := momo.New(New(pool), momo.WithHook(hook))
	held, err := New(pool).(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := momo.Exec(ctx, db, "SELECT 1")
			errs <- err
		}()
	}
	// Wait until every goroutine has started its acquire (pgxpool exposes no
	// waiter count, so a counting tracer wrapping the real one reports it).
	deadline := time.Now().Add(10 * time.Second)
	for tr.starts.Load() < n+1 { // n statements plus the held Pin
		if time.Now().After(deadline) {
			t.Fatalf("only %d acquires started, want %d", tr.starts.Load(), n+1)
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond) // a measurable wait for every waiter
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	elapsed := time.Since(start)
	waits := hook.take()
	if len(waits) != n {
		t.Fatalf("got %d events, want %d", len(waits), n)
	}
	for i, w := range waits {
		if w <= 0 || w > elapsed {
			t.Fatalf("statement %d: PoolWait = %v, want in (0, %v]", i, w, elapsed)
		}
	}
	// The waiters are done and the connection is idle: nothing to wait for.
	for range 5 {
		if _, err := momo.Exec(ctx, db, "SELECT 1"); err != nil {
			t.Fatal(err)
		}
	}
	for i, w := range hook.take() {
		if w > 40*time.Millisecond { // far below the 50ms+ every blocked waiter saw
			t.Fatalf("idle acquire %d recorded PoolWait = %v, want none", i, w)
		}
	}
}

// A statement that gives up while waiting still reports the time it waited.
func TestPoolWaitRecordedWhenAcquireTimesOut(t *testing.T) {
	pool := openInstrumentedPool(t, 1, true)
	held, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	hook := &poolWaitHook{}
	db := momo.New(New(pool), momo.WithHook(hook))
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	if _, err := momo.Exec(ctx, db, "SELECT 1"); err == nil {
		t.Fatal("want an error from the exhausted pool")
	}
	if got := time.Duration(hook.maxNS.Load()); got < 50*time.Millisecond {
		t.Fatalf("PoolWait = %v, want about 80ms for the timed-out acquire", got)
	}
}

type recordingTracer struct{ starts, ends atomic.Int64 }

func (r *recordingTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	r.starts.Add(1)
	return ctx
}

func (r *recordingTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {
	r.ends.Add(1)
}

func TestInstrumentPoolDelegatesQueryTracing(t *testing.T) {
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	rt := &recordingTracer{}
	cfg.ConnConfig.Tracer = rt
	InstrumentPool(cfg)
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closePool(t, pool) })
	if _, err := pool.Exec(ctx, "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	if rt.starts.Load() == 0 || rt.ends.Load() == 0 {
		t.Fatalf("previous query tracer not delegated to: starts=%d ends=%d", rt.starts.Load(), rt.ends.Load())
	}
}

func TestNewPoolInstrumentsAndRejectsBadURL(t *testing.T) {
	ctx := context.Background()
	if _, err := NewPool(ctx, "://bad"); err == nil {
		t.Fatal("want a parse error")
	}
	pool, err := NewPool(ctx, testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closePool(t, pool) })
	if _, err := pool.Exec(ctx, "SELECT 1"); err != nil {
		t.Fatal(err)
	}
}
