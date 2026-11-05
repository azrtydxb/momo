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
// runs one statement at a time.
//
// Release contract: Release is safe to call concurrently with statements and
// never touches the connection while pgx is using it. If a Query, Exec or
// Begin call is running, or Rows from Query are still open (not yet closed or
// exhausted), Release returns errInUse and does nothing; the connection stays
// pinned and Release may be called again once the work has finished. A
// transaction begun on the connection is not tracked: finish it before
// Release, or the pool discards the session with it.
type pinned struct {
	d    *driver
	mu   sync.Mutex // guards conn and busy
	conn *pgxpool.Conn
	busy int // statements and open Rows currently using conn
}

// errInUse is returned by Release while a statement or Rows is using the
// connection.
var errInUse = errors.New("pgxdriver: pinned connection in use by a running statement or open Rows")

// live marks the connection busy and returns it, or errReleased once Release
// has run. Every successful live must be paired with one idle.
func (p *pinned) live() (*pgxpool.Conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn == nil {
		return nil, errReleased
	}
	p.busy++
	return p.conn, nil
}

func (p *pinned) idle() {
	p.mu.Lock()
	p.busy--
	p.mu.Unlock()
}

// pinnedRows keeps the connection busy until the rows are closed or run out.
type pinnedRows struct {
	pgxRows
	p    *pinned
	once sync.Once
}

func (r *pinnedRows) finish() { r.once.Do(r.p.idle) }

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
	rows, err := c.Query(ctx, sql, p.d.withMode(args)...)
	if err != nil {
		p.idle()
		return nil, ClassifyError(err)
	}
	return &pinnedRows{pgxRows: pgxRows{rows}, p: p}, nil
}

func (p *pinned) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	c, err := p.live()
	if err != nil {
		return 0, err
	}
	defer p.idle()
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
	defer p.idle()
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

// Release returns the connection to the pool exactly once. Later calls after
// a successful Release do nothing and return nil, so a deferred Release after
// an explicit one is safe; they never touch a connection the pool has handed
// to someone else. While a statement or Rows is in flight it returns errInUse
// and leaves the connection pinned (see pinned). A transaction left open on
// the connection is discarded by the pool together with the session.
func (p *pinned) Release() error {
	p.mu.Lock()
	if p.conn == nil {
		p.mu.Unlock()
		return nil
	}
	if p.busy > 0 {
		p.mu.Unlock()
		return errInUse
	}
	c := p.conn
	p.conn = nil
	p.mu.Unlock()
	c.Release()
	return nil
}
