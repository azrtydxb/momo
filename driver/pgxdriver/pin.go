// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"errors"
	"sync"

	"github.com/azrtydxb/momo"
	"github.com/jackc/pgx/v5/pgxpool"
)

// errReleased is returned by a pinned connection used after Release.
var errReleased = errors.New("pgxdriver: pinned connection already released")

// Pin acquires one pool connection; statements through the returned
// PinnedConn share its session. The caller must Release it, once the work is
// done, even when a statement failed.
func (d *driver) Pin(ctx context.Context) (momo.PinnedConn, error) {
	if d.modeErr != nil {
		return nil, d.modeErr
	}
	conn, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return &pinned{d: d, conn: conn}, nil
}

// PoolStats reports pgxpool statistics.
func (d *driver) PoolStats() momo.PoolStats {
	s := d.pool.Stat()
	return momo.PoolStats{
		MaxConns:           int(s.MaxConns()),
		TotalConns:         int(s.TotalConns()),
		AcquiredConns:      int(s.AcquiredConns()),
		IdleConns:          int(s.IdleConns()),
		AcquireCount:       s.AcquireCount(),
		EmptyAcquireCount:  s.EmptyAcquireCount(),
		EmptyAcquireWaitNS: s.EmptyAcquireWaitTime().Nanoseconds(),
	}
}

// pinned is one connection checked out of the pool. Like a pgx connection it
// runs one statement at a time. See momo.PinnedConn for the Release contract:
// Release closes Rows left open, and returns momo.ErrPinnedBusy while a
// Query, Exec or Begin call is executing in another goroutine.
type pinned struct {
	d     *driver
	mu    sync.Mutex // guards conn, calls and rows
	conn  *pgxpool.Conn
	calls int                      // Query, Exec and Begin calls executing now
	rows  map[*pinnedRows]struct{} // Rows not yet closed or exhausted
}

// errBusy is the error Release returns while a call is executing.
func errBusy() error {
	return &momo.Error{Kind: momo.ErrPinnedBusy, Op: "release", Err: errors.New("pgxdriver: a statement is executing on the pinned connection")}
}

// live marks a call as executing and returns the connection, or errReleased
// once Release has run. Every successful live must be paired with one done.
func (p *pinned) live() (*pgxpool.Conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn == nil {
		return nil, errReleased
	}
	p.calls++
	return p.conn, nil
}

func (p *pinned) done() {
	p.mu.Lock()
	p.calls--
	p.mu.Unlock()
}

// pinnedRows is registered on its pinned connection until closed or exhausted,
// so Release can close Rows the caller forgot.
type pinnedRows struct {
	pgxRows
	p *pinned
}

func (r *pinnedRows) finish() {
	r.p.mu.Lock()
	delete(r.p.rows, r)
	r.p.mu.Unlock()
}

func (r *pinnedRows) Next() bool {
	ok := r.pgxRows.Next()
	if !ok {
		r.finish() // pgx closed the rows
	}
	return ok
}

func (r *pinnedRows) Close() error {
	err := r.pgxRows.Close()
	r.finish()
	return err
}

func (p *pinned) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	c, err := p.live()
	if err != nil {
		return nil, err
	}
	defer p.done()
	rows, err := c.Query(ctx, sql, p.d.withMode(args)...)
	if err != nil {
		return nil, ClassifyError(err)
	}
	pr := &pinnedRows{pgxRows: pgxRows{rows}, p: p}
	p.mu.Lock()
	if p.rows == nil {
		p.rows = make(map[*pinnedRows]struct{})
	}
	p.rows[pr] = struct{}{}
	p.mu.Unlock()
	return pr, nil
}

func (p *pinned) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	c, err := p.live()
	if err != nil {
		return 0, err
	}
	defer p.done()
	tag, err := c.Exec(ctx, sql, p.d.withMode(args)...)
	if err != nil {
		return 0, ClassifyError(err)
	}
	return tag.RowsAffected(), nil
}

// Begin starts a transaction on the pinned session. It uses the same pgxTx
// wrapper as the pool driver, so isolation, read-only access, exec mode and
// ErrTxDone behave identically.
func (p *pinned) Begin(ctx context.Context, opts momo.TxOptions) (momo.DriverTx, error) {
	c, err := p.live()
	if err != nil {
		return nil, err
	}
	defer p.done()
	to, err := txOptions(opts)
	if err != nil {
		return nil, err
	}
	tx, err := c.BeginTx(ctx, to)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return &pgxTx{tx: tx, d: p.d}, nil
}

// Release closes any Rows still open, then returns the connection to the pool
// exactly once. Later calls after a successful Release do nothing and return
// nil, so a deferred Release after an explicit one is safe; they never touch a
// connection the pool has handed to someone else. While a Query, Exec or Begin
// is executing in another goroutine it returns an error matching
// momo.ErrPinnedBusy and leaves the connection pinned. A transaction left open
// on the connection is discarded by the pool together with the session.
func (p *pinned) Release() error {
	p.mu.Lock()
	if p.conn == nil {
		p.mu.Unlock()
		return nil
	}
	if p.calls > 0 {
		p.mu.Unlock()
		return errBusy()
	}
	for r := range p.rows {
		r.pgxRows.Rows.Close()
	}
	p.rows = nil
	c := p.conn
	p.conn = nil
	p.mu.Unlock()
	c.Release()
	return nil
}
