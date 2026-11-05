// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"database/sql"
	sqldriver "database/sql/driver"
	"errors"
	"sync"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// FromSQLDB adopts an existing *sql.DB opened with the pgx stdlib driver. The execution mode is the one
// set in that connection's pgx config; FromSQLDB cannot change it. The caller owns db and closes it.
//
// The returned driver supports pinning, pool statistics and error classification. It implements neither
// momo.Copier nor momo.Batcher.
func FromSQLDB(db *sql.DB) momo.Driver { return &sqlDriver{db: db} }

// FromConnConfig opens a *sql.DB from cfg with stdlib.OpenDB and adopts it. cfg.DefaultQueryExecMode
// is the execution mode of every statement. opts is accepted for signature parity with New; the
// database/sql path uses only error classification, so opts have no effect here. The caller closes
// the returned *sql.DB.
func FromConnConfig(cfg *pgx.ConnConfig, opts ...Option) (*sql.DB, momo.Driver) {
	_ = opts
	db := stdlib.OpenDB(*cfg)
	return db, FromSQLDB(db)
}

type sqlDriver struct{ db *sql.DB }

func (d *sqlDriver) Dialect() *dialect.Dialect { return dialect.For(dialect.Postgres) }

// Capabilities reports vector support, as New does. There is no COPY or batch path.
func (d *sqlDriver) Capabilities() momo.Capabilities { return momo.Capabilities{Vectors: true} }

// ClassifyError implements momo.ErrorClassifier. database/sql does not wrap driver
// errors, so the *pgconn.PgError is visible to ClassifyError.
func (d *sqlDriver) ClassifyError(err error) error { return ClassifyError(err) }

func (d *sqlDriver) Query(ctx context.Context, q string, args []any) (momo.Rows, error) {
	rows, err := d.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return sqlRows{rows}, nil
}

func (d *sqlDriver) Exec(ctx context.Context, q string, args []any) (int64, error) {
	res, err := d.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, ClassifyError(err)
	}
	return rowsAffected(res)
}

func (d *sqlDriver) Begin(ctx context.Context, opts momo.TxOptions) (momo.DriverTx, error) {
	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{Isolation: opts.Isolation, ReadOnly: opts.ReadOnly})
	if err != nil {
		return nil, ClassifyError(err)
	}
	return &sqlTx{tx: tx}, nil
}

// Pin checks one connection out of the database/sql pool.
func (d *sqlDriver) Pin(ctx context.Context) (momo.PinnedConn, error) {
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return &sqlPinned{conn: conn}, nil
}

// PoolStats maps sql.DBStats onto momo.PoolStats.
func (d *sqlDriver) PoolStats() momo.PoolStats {
	s := d.db.Stats()
	return momo.PoolStats{
		MaxConns:           s.MaxOpenConnections,
		TotalConns:         s.OpenConnections,
		AcquiredConns:      s.InUse,
		IdleConns:          s.Idle,
		AcquireCount:       s.WaitCount,
		EmptyAcquireWaitNS: s.WaitDuration.Nanoseconds(),
	}
}

func rowsAffected(res sql.Result) (int64, error) {
	n, err := res.RowsAffected()
	if err != nil {
		return 0, ClassifyError(err)
	}
	return n, nil
}

// sqlRows adapts *sql.Rows to momo.Rows. sql.Rows.Close is already idempotent.
type sqlRows struct{ r *sql.Rows }

func (r sqlRows) Next() bool             { return r.r.Next() }
func (r sqlRows) Scan(dest ...any) error { return ClassifyError(r.r.Scan(dest...)) }
func (r sqlRows) Err() error             { return ClassifyError(r.r.Err()) }
func (r sqlRows) Close() error           { return ClassifyError(r.r.Close()) }

// pinnedSQLRows is registered on its pinned connection until closed or exhausted,
// so Release can close Rows the caller forgot.
type pinnedSQLRows struct {
	sqlRows
	p *sqlPinned
}

func (r *pinnedSQLRows) finish() {
	r.p.mu.Lock()
	delete(r.p.rows, r)
	r.p.mu.Unlock()
}

func (r *pinnedSQLRows) Next() bool {
	ok := r.sqlRows.Next()
	if !ok {
		r.finish()
	}
	return ok
}

func (r *pinnedSQLRows) Close() error {
	err := r.sqlRows.Close()
	r.finish()
	return err
}

// sqlTx adapts *sql.Tx to momo.DriverTx.
type sqlTx struct{ tx *sql.Tx }

func (t *sqlTx) Query(ctx context.Context, q string, args []any) (momo.Rows, error) {
	rows, err := t.tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, sqlTxErr("select", err)
	}
	return sqlRows{rows}, nil
}

func (t *sqlTx) Exec(ctx context.Context, q string, args []any) (int64, error) {
	res, err := t.tx.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, sqlTxErr("exec", err)
	}
	return rowsAffected(res)
}

func (t *sqlTx) Commit(_ context.Context) error   { return sqlTxErr("commit", t.tx.Commit()) }
func (t *sqlTx) Rollback(_ context.Context) error { return sqlTxErr("rollback", t.tx.Rollback()) }

// sqlTxErr maps a finished transaction to momo.ErrTxDone and classifies everything else.
func sqlTxErr(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrTxDone) {
		return &momo.Error{Kind: momo.ErrTxDone, Op: op, Err: err}
	}
	return ClassifyError(err)
}

// sqlPinned is one *sql.Conn checked out of the pool. See momo.PinnedConn for the
// Release contract.
type sqlPinned struct {
	mu    sync.Mutex // guards conn, calls and rows
	conn  *sql.Conn
	calls int                         // Query, Exec and Begin calls executing now
	rows  map[*pinnedSQLRows]struct{} // Rows not yet closed or exhausted
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

func (p *sqlPinned) Query(ctx context.Context, q string, args []any) (momo.Rows, error) {
	c, err := p.live()
	if err != nil {
		return nil, err
	}
	defer p.done()
	rows, err := c.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, ClassifyError(err)
	}
	pr := &pinnedSQLRows{sqlRows: sqlRows{rows}, p: p}
	p.mu.Lock()
	if p.rows == nil {
		p.rows = make(map[*pinnedSQLRows]struct{})
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
	res, err := c.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, ClassifyError(err)
	}
	return rowsAffected(res)
}

func (p *sqlPinned) Begin(ctx context.Context, opts momo.TxOptions) (momo.DriverTx, error) {
	c, err := p.live()
	if err != nil {
		return nil, err
	}
	defer p.done()
	tx, err := c.BeginTx(ctx, &sql.TxOptions{Isolation: opts.Isolation, ReadOnly: opts.ReadOnly})
	if err != nil {
		return nil, ClassifyError(err)
	}
	return &sqlTx{tx: tx}, nil
}

// take marks the connection released after closing open Rows. It returns nil, nil when
// already released and errBusy while a call is executing.
func (p *sqlPinned) take() (*sql.Conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn == nil {
		return nil, nil
	}
	if p.calls > 0 {
		return nil, errBusy()
	}
	for r := range p.rows {
		_ = r.sqlRows.r.Close()
	}
	p.rows = nil
	c := p.conn
	p.conn = nil
	return c, nil
}

// Release closes open Rows, then returns the connection to the pool exactly once.
func (p *sqlPinned) Release() error {
	c, err := p.take()
	if c == nil {
		return err
	}
	return ClassifyError(c.Close())
}

// Discard closes the underlying driver connection so database/sql destroys it instead of
// reusing a session whose state could not be restored. See momo.Discarder.
func (p *sqlPinned) Discard() error {
	c, err := p.take()
	if c == nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), discardCloseTimeout)
	defer cancel()
	rawErr := c.Raw(func(dc any) error {
		sc, ok := dc.(*stdlib.Conn)
		if !ok {
			return sqldriver.ErrBadConn
		}
		if err := sc.Conn().Close(ctx); err != nil {
			return err
		}
		return sqldriver.ErrBadConn // database/sql drops a connection reporting ErrBadConn
	})
	closeErr := c.Close()
	if rawErr != nil && !errors.Is(rawErr, sqldriver.ErrBadConn) {
		return ClassifyError(rawErr)
	}
	if closeErr != nil && !errors.Is(closeErr, sql.ErrConnDone) {
		return ClassifyError(closeErr)
	}
	return nil
}

var (
	_ momo.Discarder          = (*sqlPinned)(nil)
	_ momo.Pinner             = (*sqlDriver)(nil)
	_ momo.PoolStater         = (*sqlDriver)(nil)
	_ momo.CapabilityReporter = (*sqlDriver)(nil)
)
