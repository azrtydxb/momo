// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// acquireStart is stored in the context TraceAcquireStart returns, which pgxpool
// hands back to TraceAcquireEnd.
type acquireStart struct {
	at         time.Time
	emptyCount int64 // pool.Stat().EmptyAcquireCount() when the acquire began
}

type acquireStartKey struct{}

type poolTracer struct{ next pgx.QueryTracer }

func (p *poolTracer) TraceAcquireStart(ctx context.Context, pool *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	return context.WithValue(ctx, acquireStartKey{}, acquireStart{
		at:         time.Now(),
		emptyCount: pool.Stat().EmptyAcquireCount(),
	})
}

// TraceAcquireEnd reports the wait only when this acquire really had to wait:
// pgxpool counts an acquire as "empty" when no idle connection was available,
// which covers waiting for a release and dialing a new connection. An acquire
// that failed waited until it gave up and is reported too. An acquire served from an idle
// connection records nothing.
func (p *poolTracer) TraceAcquireEnd(ctx context.Context, pool *pgxpool.Pool, end pgxpool.TraceAcquireEndData) {
	s, ok := ctx.Value(acquireStartKey{}).(acquireStart)
	if !ok {
		return
	}
	// A failed acquire (timeout, cancellation) is not counted by pgxpool but
	// did wait until it gave up.
	if end.Err == nil && pool.Stat().EmptyAcquireCount() == s.emptyCount {
		return
	}
	momo.RecordPoolWait(ctx, time.Since(s.at))
}

func (p *poolTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if p.next != nil {
		return p.next.TraceQueryStart(ctx, conn, data)
	}
	return ctx
}

func (p *poolTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	if p.next != nil {
		p.next.TraceQueryEnd(ctx, conn, data)
	}
}

// InstrumentPool installs the pool acquire tracer on cfg. It is set as
// cfg.ConnConfig.Tracer and delegates pgx query tracing to any tracer that was
// set before. Call it before pgxpool.NewWithConfig.
func InstrumentPool(cfg *pgxpool.Config) {
	cfg.ConnConfig.Tracer = &poolTracer{next: cfg.ConnConfig.Tracer}
}

// NewPool parses url, installs the tracer with InstrumentPool and opens the pool.
func NewPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	InstrumentPool(cfg)
	return pgxpool.NewWithConfig(ctx, cfg)
}
