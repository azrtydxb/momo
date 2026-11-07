// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/azrtydxb/momo"
)

// batchStub wraps a batching driver and lets a test rewrite or reject the batch.
type batchStub struct {
	momo.Driver
	mutate func([]momo.Compiled) []momo.Compiled
	wrap   func([]momo.Rows) []momo.Rows
	err    error
	mu     sync.Mutex
	sent   int
}

func (d *batchStub) SendBatch(ctx context.Context, qs []momo.Compiled) ([]momo.Rows, error) {
	d.mu.Lock()
	d.sent++
	d.mu.Unlock()
	if d.err != nil {
		return nil, d.err
	}
	if d.mutate != nil {
		qs = d.mutate(append([]momo.Compiled(nil), qs...))
	}
	rows, err := d.Driver.(momo.Batcher).SendBatch(ctx, qs)
	if err == nil && d.wrap != nil {
		rows = d.wrap(rows)
	}
	return rows, err
}

func (d *batchStub) ClassifyError(err error) error {
	if c, ok := d.Driver.(momo.ErrorClassifier); ok {
		return c.ClassifyError(err)
	}
	return err
}

// scanFailRows fails every Scan and counts Close calls.
type scanFailRows struct {
	momo.Rows
	fail   bool
	closes *atomic.Int64
}

func (r *scanFailRows) Scan(dest ...any) error {
	if r.fail {
		return errors.New("injected scan failure")
	}
	return r.Rows.Scan(dest...)
}

func (r *scanFailRows) Close() error {
	r.closes.Add(1)
	return r.Rows.Close()
}

// pairHook records every Before/After pair of statements and relations.
type pairHook struct {
	mu               sync.Mutex
	beforeQ, beforeR int
	relErrs          []error
	queryErrs        []error
}

func (h *pairHook) BeforeQuery(ctx context.Context, _ *momo.QueryEvent) context.Context {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.beforeQ++
	return ctx
}

func (h *pairHook) AfterQuery(_ context.Context, e *momo.QueryEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.queryErrs = append(h.queryErrs, e.Err)
}

func (h *pairHook) BeforeRelation(ctx context.Context, _ *momo.RelationEvent) context.Context {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.beforeR++
	return ctx
}

func (h *pairHook) AfterRelation(_ context.Context, e *momo.RelationEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.relErrs = append(h.relErrs, e.Err)
}
