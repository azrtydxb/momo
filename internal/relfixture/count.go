// SPDX-License-Identifier: Apache-2.0

package relfixture

import (
	"context"
	"sync/atomic"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// Counter counts driver-level round trips: one per Query, one per Exec and one
// per SendBatch, however many statements the batch carries.
type Counter struct {
	Queries atomic.Int64
	Execs   atomic.Int64
	Batches atomic.Int64
}

// Reset zeroes every count.
func (c *Counter) Reset() {
	c.Queries.Store(0)
	c.Execs.Store(0)
	c.Batches.Store(0)
}

// countDriver counts the calls it forwards to inner. The error classifier and
// capability report are forwarded so wrapped errors keep their kinds.
type countDriver struct {
	inner momo.Driver
	c     *Counter
}

func (d *countDriver) Dialect() *dialect.Dialect { return d.inner.Dialect() }

func (d *countDriver) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	d.c.Queries.Add(1)
	return d.inner.Query(ctx, sql, args)
}

func (d *countDriver) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	d.c.Execs.Add(1)
	return d.inner.Exec(ctx, sql, args)
}

func (d *countDriver) Begin(ctx context.Context, opts momo.TxOptions) (momo.DriverTx, error) {
	tx, err := d.inner.Begin(ctx, opts)
	if err != nil {
		return nil, err
	}
	ct := &countTx{inner: tx, c: d.c}
	if _, ok := tx.(momo.Batcher); ok {
		return countBatchTx{ct}, nil
	}
	return ct, nil
}

func (d *countDriver) ClassifyError(err error) error { return classify(d.inner, err) }

func (d *countDriver) Capabilities() momo.Capabilities { return capabilities(d.inner) }

type countTx struct {
	inner momo.DriverTx
	c     *Counter
}

func (t *countTx) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	t.c.Queries.Add(1)
	return t.inner.Query(ctx, sql, args)
}

func (t *countTx) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	t.c.Execs.Add(1)
	return t.inner.Exec(ctx, sql, args)
}

func (t *countTx) Commit(ctx context.Context) error   { return t.inner.Commit(ctx) }
func (t *countTx) Rollback(ctx context.Context) error { return t.inner.Rollback(ctx) }

// countBatchDriver and countBatchTx add SendBatch, so the wrapper implements
// momo.Batcher only when the wrapped driver or transaction does.
type countBatchDriver struct{ *countDriver }

func (d countBatchDriver) SendBatch(ctx context.Context, qs []momo.Compiled) ([]momo.Rows, error) {
	d.c.Batches.Add(1)
	return d.inner.(momo.Batcher).SendBatch(ctx, qs)
}

type countBatchTx struct{ *countTx }

func (t countBatchTx) SendBatch(ctx context.Context, qs []momo.Compiled) ([]momo.Rows, error) {
	t.c.Batches.Add(1)
	return t.inner.(momo.Batcher).SendBatch(ctx, qs)
}

// Counting wraps inner and counts its round trips, including those of its
// transactions. The returned driver implements momo.Batcher (also on its
// transactions) only when inner does, so the batched and sequential load paths
// stay selectable.
func Counting(inner momo.Driver) (momo.Driver, *Counter) {
	c := &Counter{}
	cd := &countDriver{inner: inner, c: c}
	if _, ok := inner.(momo.Batcher); ok {
		return countBatchDriver{cd}, c
	}
	return cd, c
}

// noBatch hides every optional interface of its driver except the error
// classifier and capabilities, so loads on it run sequentially.
type noBatch struct{ inner momo.Driver }

// WithoutBatch returns inner without its momo.Batcher implementation (and
// without that of its transactions): the sequential reference for pipelining
// tests, and a stand-in for a driver that cannot batch.
func WithoutBatch(inner momo.Driver) momo.Driver { return noBatch{inner} }

func (d noBatch) Dialect() *dialect.Dialect { return d.inner.Dialect() }

func (d noBatch) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	return d.inner.Query(ctx, sql, args)
}

func (d noBatch) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	return d.inner.Exec(ctx, sql, args)
}

func (d noBatch) Begin(ctx context.Context, opts momo.TxOptions) (momo.DriverTx, error) {
	tx, err := d.inner.Begin(ctx, opts)
	if err != nil {
		return nil, err
	}
	return noBatchTx{tx}, nil
}

func (d noBatch) ClassifyError(err error) error { return classify(d.inner, err) }

func (d noBatch) Capabilities() momo.Capabilities {
	c := capabilities(d.inner)
	c.Batch = false
	return c
}

// noBatchTx exposes only the DriverTx methods of the embedded transaction.
type noBatchTx struct{ momo.DriverTx }

func classify(d momo.Driver, err error) error {
	if c, ok := d.(momo.ErrorClassifier); ok {
		return c.ClassifyError(err)
	}
	return err
}

func capabilities(d momo.Driver) momo.Capabilities {
	if c, ok := d.(momo.CapabilityReporter); ok {
		return c.Capabilities()
	}
	return momo.Capabilities{}
}
