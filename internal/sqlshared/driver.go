// SPDX-License-Identifier: Apache-2.0

// Package sqlshared holds the part of the SQLite adapters that does not depend
// on the SQLite driver: the momo.Driver implementation over database/sql,
// transactions, read-only transactions, pinned connections and time scanning.
// driver/sqlitedriver (mattn, cgo) and driver/sqlitepure (modernc, pure Go)
// both build on it and supply only what differs: opening, DSN rules, error
// classification and capabilities.
//
// The package has no build tags and imports neither mattn/go-sqlite3 nor
// modernc.org/sqlite, so linking it never pulls in a SQLite implementation.
package sqlshared

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/azrtydxb/momo"
)

// Hooks carries what differs between the two SQLite drivers.
type Hooks struct {
	// Classify maps a driver error to a *momo.Error. op names the operation
	// ("select", "exec", "begin", ...) or is empty. It must return errors it
	// does not recognise unchanged and must accept nil.
	Classify func(op string, err error) error
	// IsDriverErr reports whether err is, or wraps, an error of the SQLite
	// driver itself. Scan uses it to tell a driver error from a database/sql
	// conversion failure, whose text can quote a stored value.
	IsDriverErr func(error) bool
}

// DriverError carries a driver error while exposing only a fixed message. The
// cause stays reachable through errors.As and Unwrap.
type DriverError struct {
	Cause error
	Msg   string
}

func (e DriverError) Error() string { return e.Msg }
func (e DriverError) Unwrap() error { return e.Cause }

// Driver implements momo.Driver, momo.Pinner, momo.PoolStater and
// momo.ErrorClassifier over a *sql.DB. The adapters embed it and add Dialect
// and Capabilities.
type Driver struct {
	db *sql.DB
	h  *Hooks
}

// New returns a Driver over db.
func New(db *sql.DB, h Hooks) *Driver { return &Driver{db: db, h: &h} }

// ClassifyError implements momo.ErrorClassifier.
func (d *Driver) ClassifyError(err error) error { return d.h.Classify("", err) }

// Query runs a query on the pool.
func (d *Driver) Query(ctx context.Context, q string, args []any) (momo.Rows, error) {
	rows, err := d.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, d.h.Classify("select", err)
	}
	return sqlRows{r: rows, h: d.h}, nil
}

// Exec runs a statement on the pool.
func (d *Driver) Exec(ctx context.Context, q string, args []any) (int64, error) {
	res, err := d.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, d.h.Classify("exec", err)
	}
	return d.h.rowsAffected(res)
}

// Begin starts a transaction. SQLite is serializable, so the default and
// sql.LevelSerializable are accepted and every other level is
// momo.ErrUnsupported. A read-only transaction runs PRAGMA query_only=ON on a
// dedicated connection and resets it when the transaction finishes.
func (d *Driver) Begin(ctx context.Context, opts momo.TxOptions) (momo.DriverTx, error) {
	if err := checkIsolation(opts); err != nil {
		return nil, err
	}
	if !opts.ReadOnly {
		tx, err := d.h.beginTx(ctx, d.db.BeginTx)
		if err != nil {
			return nil, err
		}
		return &sqlTx{tx: tx, h: d.h}, nil
	}
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return nil, d.h.Classify("begin", err)
	}
	tx, err := d.h.beginReadOnly(ctx, conn)
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
		Err:  errors.New("sqlite: SQLite is serializable; other isolation levels are not supported"),
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
func (h *Hooks) beginTx(ctx context.Context, begin func(context.Context, *sql.TxOptions) (*sql.Tx, error)) (*sql.Tx, error) {
	if err := ctx.Err(); err != nil {
		return nil, h.Classify("begin", err)
	}
	bctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stop := context.AfterFunc(ctx, cancel)
	tx, err := begin(bctx, nil)
	if !stop() {
		if tx != nil {
			_ = tx.Rollback()
		}
		return nil, h.Classify("begin", ctx.Err())
	}
	if err != nil {
		cancel()
		return nil, h.Classify("begin", err)
	}
	return tx, nil
}

// Pin checks one connection out of the database/sql pool.
func (d *Driver) Pin(ctx context.Context) (momo.PinnedConn, error) {
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return nil, d.h.Classify("pin", err)
	}
	return &sqlPinned{conn: conn, h: d.h}, nil
}

// PoolStats maps sql.DBStats onto momo.PoolStats.
func (d *Driver) PoolStats() momo.PoolStats {
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

func (h *Hooks) rowsAffected(res sql.Result) (int64, error) {
	n, err := res.RowsAffected()
	if err != nil {
		return 0, h.Classify("exec", err)
	}
	return n, nil
}

// sqlRows adapts *sql.Rows to momo.Rows. sql.Rows.Close is already idempotent.
type sqlRows struct {
	r *sql.Rows
	h *Hooks
}

func (r sqlRows) Next() bool             { return r.r.Next() }
func (r sqlRows) Scan(dest ...any) error { return r.h.scanErr(r.r.Scan(wrapDests(dest)...)) }

// scanErr hides the text of a conversion failure: database/sql quotes the
// stored value in it ("converting driver.Value type string ("...")"). Only
// errors that look like a conversion failure are masked; cancellation, a
// closed Rows and driver errors pass through. The cause stays reachable
// through errors.Unwrap, as with pgxdriver data exceptions.
func (h *Hooks) scanErr(err error) error {
	if err == nil {
		return nil
	}
	c := h.Classify("", err)
	var me *momo.Error
	if h.IsDriverErr(c) || errors.As(c, &me) ||
		errors.Is(c, context.Canceled) || errors.Is(c, context.DeadlineExceeded) || errors.Is(c, sql.ErrNoRows) {
		return c
	}
	msg := c.Error()
	if !strings.Contains(msg, "converting") && !strings.Contains(msg, "Scan error on column") {
		return c
	}
	return DriverError{Cause: c, Msg: "sqlite: cannot convert the column value to the destination type"}
}

func (r sqlRows) Err() error   { return r.h.Classify("", r.r.Err()) }
func (r sqlRows) Close() error { return r.h.Classify("", r.r.Close()) }

// Columns reports the result column names (momo.ColumnNamer).
func (r sqlRows) Columns() ([]string, error) {
	cols, err := r.r.Columns()
	return cols, r.h.Classify("", err)
}

// sqlTx adapts *sql.Tx to momo.DriverTx.
type sqlTx struct {
	tx     *sql.Tx
	h      *Hooks
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
		return nil, t.h.txErr("select", err)
	}
	return sqlRows{r: rows, h: t.h}, nil
}

func (t *sqlTx) Exec(ctx context.Context, q string, args []any) (int64, error) {
	res, err := t.tx.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, t.h.txErr("exec", err)
	}
	return t.h.rowsAffected(res)
}

func (t *sqlTx) Commit(_ context.Context) error {
	defer t.finished()
	return t.h.txErr("commit", t.tx.Commit())
}

func (t *sqlTx) Rollback(_ context.Context) error {
	defer t.finished()
	return t.h.txErr("rollback", t.tx.Rollback())
}

// txErr maps a finished transaction to momo.ErrTxDone and classifies everything else.
func (h *Hooks) txErr(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrTxDone) {
		return &momo.Error{Kind: momo.ErrTxDone, Op: op, Err: err}
	}
	return h.Classify(op, err)
}

var (
	_ momo.Pinner          = (*Driver)(nil)
	_ momo.PoolStater      = (*Driver)(nil)
	_ momo.ErrorClassifier = (*Driver)(nil)
	_ momo.Discarder       = (*sqlPinned)(nil)
)
