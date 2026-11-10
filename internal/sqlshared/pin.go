// SPDX-License-Identifier: Apache-2.0

package sqlshared

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync"

	"github.com/azrtydxb/momo"
)

var (
	errReleased = errors.New("sqlite: pinned connection already released")
	errBadConn  = driver.ErrBadConn
)

func errBusy() error {
	return &momo.Error{Kind: momo.ErrPinnedBusy, Op: "release", Err: errors.New("sqlite: a statement is executing on the pinned connection")}
}

// sqlPinned is one *sql.Conn checked out of the pool. See momo.PinnedConn for
// the Release contract.
type sqlPinned struct {
	h     *Hooks
	mu    sync.Mutex // guards conn, calls, rows and txs
	conn  *sql.Conn
	calls int                      // Query, Exec and Begin calls executing now
	rows  map[*pinnedRows]struct{} // Rows not yet closed or exhausted
	txs   map[aborter]struct{}     // transactions begun here and not yet finished
}

func (p *sqlPinned) live() (*sql.Conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn == nil {
		return nil, errReleased
	}
	p.calls++
	return p.conn, nil
}

func (p *sqlPinned) done() {
	p.mu.Lock()
	p.calls--
	p.mu.Unlock()
}

// pinnedRows is registered on its pinned connection until closed or exhausted,
// so Release can close Rows the caller forgot.
type pinnedRows struct {
	sqlRows
	p *sqlPinned
}

func (r *pinnedRows) finish() {
	r.p.mu.Lock()
	delete(r.p.rows, r)
	r.p.mu.Unlock()
}

func (r *pinnedRows) Next() bool {
	ok := r.sqlRows.Next()
	if !ok {
		r.finish()
	}
	return ok
}

func (r *pinnedRows) Close() error {
	err := r.sqlRows.Close()
	r.finish()
	return err
}

func (p *sqlPinned) Query(ctx context.Context, q string, args []any) (momo.Rows, error) {
	c, err := p.live()
	if err != nil {
		return nil, err
	}
	defer p.done()
	if err := checkArgs("select", args); err != nil {
		return nil, err
	}
	rows, err := c.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, p.h.Classify("select", err)
	}
	pr := &pinnedRows{sqlRows: sqlRows{r: rows, h: p.h}, p: p}
	p.mu.Lock()
	if p.rows == nil {
		p.rows = make(map[*pinnedRows]struct{})
	}
	p.rows[pr] = struct{}{}
	p.mu.Unlock()
	return pr, nil
}

func (p *sqlPinned) Exec(ctx context.Context, q string, args []any) (int64, error) {
	c, err := p.live()
	if err != nil {
		return 0, err
	}
	defer p.done()
	if err := checkArgs("exec", args); err != nil {
		return 0, err
	}
	res, err := c.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, p.h.Classify("exec", err)
	}
	return p.h.rowsAffected(res)
}

// Begin starts a transaction on the pinned connection, with the isolation and
// read-only rules of the pool-level Begin.
func (p *sqlPinned) Begin(ctx context.Context, opts momo.TxOptions) (momo.DriverTx, error) {
	if err := checkIsolation(opts); err != nil {
		return nil, err
	}
	c, err := p.live()
	if err != nil {
		return nil, err
	}
	defer p.done()
	if opts.ReadOnly {
		tx, err := p.h.beginReadOnly(ctx, c)
		if err != nil {
			return nil, err
		}
		tx.onDone = func() { p.forget(tx) }
		p.track(tx)
		return tx, nil
	}
	tx, err := p.h.beginTx(ctx, c.BeginTx)
	if err != nil {
		return nil, err
	}
	st := &sqlTx{tx: tx, h: p.h}
	st.onDone = func() { p.forget(st) }
	p.track(st)
	return st, nil
}

func (p *sqlPinned) track(t aborter) {
	p.mu.Lock()
	if p.txs == nil {
		p.txs = make(map[aborter]struct{})
	}
	p.txs[t] = struct{}{}
	p.mu.Unlock()
}

func (p *sqlPinned) forget(t aborter) {
	p.mu.Lock()
	delete(p.txs, t)
	p.mu.Unlock()
}

// aborter is a transaction Release can roll back.
type aborter interface{ abort() }

func (t *sqlTx) abort() { _ = t.tx.Rollback() }

// take marks the connection released after closing open Rows. It returns nil,
// nil when already released and a busy error while a call is executing.
func (p *sqlPinned) take() (*sql.Conn, error) {
	p.mu.Lock()
	if p.conn == nil {
		p.mu.Unlock()
		return nil, nil
	}
	if p.calls > 0 {
		p.mu.Unlock()
		return nil, errBusy()
	}
	for r := range p.rows {
		_ = r.sqlRows.r.Close()
	}
	p.rows = nil
	txs := p.txs
	p.txs = nil
	c := p.conn
	p.conn = nil
	p.mu.Unlock()
	// Each open *sql.Tx holds the Conn's close lock, so Conn.Close would block
	// forever: roll them back first.
	for tx := range txs {
		tx.abort()
	}
	return c, nil
}

// Release closes open Rows, rolls back open transactions, then returns the
// connection to the pool exactly once.
func (p *sqlPinned) Release() error {
	c, err := p.take()
	if c == nil {
		return err
	}
	return p.h.Classify("release", c.Close())
}

// Discard closes the underlying connection so database/sql destroys it
// instead of reusing a session whose state could not be restored. See
// momo.Discarder. Discarding the connection of an in-memory database destroys
// that database.
func (p *sqlPinned) Discard() error {
	c, err := p.take()
	if c == nil {
		return err
	}
	_ = c.Raw(func(any) error { return errBadConn })
	if err := c.Close(); err != nil && !errors.Is(err, sql.ErrConnDone) {
		return p.h.Classify("discard", err)
	}
	return nil
}
