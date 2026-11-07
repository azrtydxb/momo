// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/azrtydxb/momo"
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

type recordingTracer struct{ starts, ends atomic.Int64 }

func (r *recordingTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	r.starts.Add(1)
	return ctx
}

func (r *recordingTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {
	r.ends.Add(1)
}
