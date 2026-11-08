// SPDX-License-Identifier: Apache-2.0

//go:build cgo && !windows

package sqlitedriver

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/mattn/go-sqlite3"
)

type sqlDriver struct{ db *sql.DB }

func newDriver(db *sql.DB) momo.Driver { return &sqlDriver{db: db} }

func (d *sqlDriver) Dialect() *dialect.Dialect { return dialect.For(dialect.SQLite) }

// Capabilities reports vector support through sqlite-vec. There is no COPY or
// batch path.
func (d *sqlDriver) Capabilities() momo.Capabilities { return momo.Capabilities{Vectors: true} }

// ClassifyError implements momo.ErrorClassifier.
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

// Begin starts a transaction. SQLite is serializable, so the default and
// sql.LevelSerializable are accepted and every other level is
// momo.ErrUnsupported. A read-only transaction runs PRAGMA query_only=ON on a
// dedicated connection and resets it when the transaction finishes.
func (d *sqlDriver) Begin(ctx context.Context, opts momo.TxOptions) (momo.DriverTx, error) {
	if err := checkIsolation(opts); err != nil {
		return nil, err
	}
	if !opts.ReadOnly {
		tx, err := beginTx(ctx, d.db.BeginTx, opts)
		if err != nil {
			return nil, err
		}
		return &sqlTx{tx: tx}, nil
	}
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return nil, ClassifyError(err)
	}
	tx, err := beginReadOnly(ctx, conn)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	tx.onDone = func() { _ = conn.Close() }
	return tx, nil
}

func checkIsolation(opts momo.TxOptions) error {
	switch opts.Isolation {
	case sql.LevelDefault, sql.LevelSerializable:
		return nil
	}
	return &momo.Error{
		Kind: momo.ErrUnsupported,
		Op:   "begin",
		Err:  errors.New("sqlitedriver: SQLite is serializable; other isolation levels are not supported"),
	}
}

// resetQueryOnly turns query_only off again. A failure leaves the connection
// read-only, so it is discarded instead of reused.
func resetQueryOnly(conn *sql.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), resetTimeout)
	defer cancel()
	if _, err := conn.ExecContext(ctx, "PRAGMA query_only=OFF"); err != nil {
		_ = conn.Raw(func(any) error { return errBadConn })
	}
}

const resetTimeout = 5 * time.Second

// beginTx begins a transaction that outlives ctx: database/sql rolls a
// transaction back when the context given to BeginTx is cancelled, so ctx
// bounds the begin call alone, as it does for pgxpool.
func beginTx(ctx context.Context, begin func(context.Context, *sql.TxOptions) (*sql.Tx, error), _ momo.TxOptions) (*sql.Tx, error) {
	if err := ctx.Err(); err != nil {
		return nil, ClassifyError(err)
	}
	bctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stop := context.AfterFunc(ctx, cancel)
	tx, err := begin(bctx, nil)
	if !stop() {
		if tx != nil {
			_ = tx.Rollback()
		}
		return nil, ClassifyError(ctx.Err())
	}
	if err != nil {
		cancel()
		return nil, ClassifyError(err)
	}
	return tx, nil
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
func (r sqlRows) Scan(dest ...any) error { return scanErr(r.r.Scan(wrapDests(dest)...)) }

// scanErr hides the text of a conversion failure: database/sql quotes the
// stored value in it ("converting driver.Value type string ("...")"). The
// cause stays reachable through errors.Unwrap, as with pgxdriver data
// exceptions.
func scanErr(err error) error {
	if err == nil {
		return nil
	}
	c := ClassifyError(err)
	var se sqlite3.Error
	var me *momo.Error
	if errors.As(c, &se) || errors.As(c, &me) {
		return c
	}
	err = c
	return driverError{cause: err, msg: "sqlitedriver: cannot convert the column value to the destination type"}
}
func (r sqlRows) Err() error   { return ClassifyError(r.r.Err()) }
func (r sqlRows) Close() error { return ClassifyError(r.r.Close()) }

// sqlTx adapts *sql.Tx to momo.DriverTx.
type sqlTx struct {
	tx     *sql.Tx
	onDone func() // called after Commit or Rollback; may be nil
}

func (t *sqlTx) finished() {
	if t.onDone != nil {
		t.onDone()
	}
}

func (t *sqlTx) Query(ctx context.Context, q string, args []any) (momo.Rows, error) {
	rows, err := t.tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, txErr("select", err)
	}
	return sqlRows{rows}, nil
}

func (t *sqlTx) Exec(ctx context.Context, q string, args []any) (int64, error) {
	res, err := t.tx.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, txErr("exec", err)
	}
	return rowsAffected(res)
}

func (t *sqlTx) Commit(_ context.Context) error {
	defer t.finished()
	return txErr("commit", t.tx.Commit())
}

func (t *sqlTx) Rollback(_ context.Context) error {
	defer t.finished()
	return txErr("rollback", t.tx.Rollback())
}

// txErr maps a finished transaction to momo.ErrTxDone and classifies everything else.
func txErr(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrTxDone) {
		return &momo.Error{Kind: momo.ErrTxDone, Op: op, Err: err}
	}
	return ClassifyError(err)
}

var (
	_ momo.Pinner             = (*sqlDriver)(nil)
	_ momo.PoolStater         = (*sqlDriver)(nil)
	_ momo.CapabilityReporter = (*sqlDriver)(nil)
	_ momo.ErrorClassifier    = (*sqlDriver)(nil)
	_ momo.Discarder          = (*sqlPinned)(nil)
)
