// SPDX-License-Identifier: Apache-2.0

// Package mysqldriver adapts database/sql on github.com/go-sql-driver/mysql
// to momo for MySQL 8.4 and 9.7 LTS and MariaDB 11.8 LTS.
package mysqldriver

import (
	"context"
	"database/sql"
	sqldriver "database/sql/driver"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/sqlmask"
	"github.com/azrtydxb/momo/internal/sqlshared"
	"github.com/go-sql-driver/mysql"
)

// ConfigureDSN parses dsn and sets the connection parameters Momo relies on:
//
//   - parseTime=true and loc=UTC so DATETIME and TIMESTAMP scan into time.Time
//     in UTC, and the session time_zone is set to +00:00 so TIMESTAMP columns
//     and NOW() agree with that;
//   - clientFoundRows=true so an UPDATE reports the rows it matched, not only
//     the rows whose values changed (optimistic locking and not-found
//     detection depend on it). The same flag changes the RowsAffected of an
//     upsert: ON DUPLICATE KEY UPDATE reports 1 for an inserted row and also 1
//     for an existing row set to its current values, and 2 for an existing row
//     that changed. Do not read a count of 1 as proof of an insert;
//   - interpolateParams=false and multiStatements=false: values always travel
//     as bound parameters of a server-side prepared statement and never become
//     SQL text. A DSN that sets either is overridden, not rejected.
//
// Transport security is the DSN's business and defaults to none: go-sql-driver
// uses tls=false unless the DSN sets tls (true, skip-verify, preferred or a
// registered config), so credentials and rows cross the network in clear text.
// Set tls=true (or a registered config) for any server that is not on the same
// host.
func ConfigureDSN(dsn string) (*mysql.Config, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	if cfg.AllowAllFiles {
		return nil, dsnRefused("allowAllFiles=true lets the server read any local file through LOAD DATA LOCAL INFILE")
	}
	if cfg.AllowCleartextPasswords {
		return nil, dsnRefused("allowCleartextPasswords=true sends the password to the server unhashed")
	}
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.ClientFoundRows = true
	cfg.InterpolateParams = false
	cfg.MultiStatements = false
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	cfg.Params["time_zone"] = "'+00:00'"
	return cfg, nil
}

// dsnRefused is the momo.ErrValidation for a DSN setting momo refuses. The
// message never quotes the DSN, which carries the password.
func dsnRefused(why string) error {
	return &momo.Error{Kind: momo.ErrValidation, Op: "open", Err: errors.New("mysqldriver: " + why + "; momo refuses it")}
}

// Open opens a *sql.DB for dsn with ConfigureDSN applied and adopts it. Open
// pings the server, as the SQLite drivers do, so a bad DSN, a refused
// connection or bad credentials are reported here and not on the first
// statement. An unknown flavor, a nil result of New, is momo.ErrUnsupported.
// On any error the pool is closed and nothing is returned; on success the
// caller closes the returned *sql.DB.
//
// ConfigureDSN refuses allowAllFiles=true and allowCleartextPasswords=true
// with momo.ErrValidation.
func Open(dsn string, flavor dialect.Flavor) (*sql.DB, momo.Driver, error) {
	cfg, err := ConfigureDSN(dsn)
	if err != nil {
		return nil, nil, err
	}
	conn, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, nil, err
	}
	db := sql.OpenDB(conn)
	d := New(db, flavor)
	if bad, ok := d.(*driver); ok && bad.err != nil {
		_ = db.Close()
		return nil, nil, bad.err
	}
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		return nil, nil, classify(err, "")
	}
	return db, d, nil
}

// New wraps an opened *sql.DB. The DSN must come from ConfigureDSN (or set
// parseTime=true, loc=UTC, clientFoundRows=true, multiStatements=false and,
// above all, interpolateParams=false): New cannot see the DSN of a *sql.DB it
// is given, and a DSN with interpolateParams=true makes the driver write bound
// values into the SQL text on the client, which defeats parameter binding and
// the redaction of sensitive arguments. The adapter never opens
// connections itself and the caller closes db. An unknown flavor yields a
// driver whose every call returns momo.ErrUnsupported.
func New(db *sql.DB, flavor dialect.Flavor) momo.Driver {
	d := dialect.MySQLFlavor(flavor)
	if d == nil || db == nil {
		return &driver{err: &momo.Error{Kind: momo.ErrUnsupported, Op: "build",
			Err: errors.New("mysqldriver: nil *sql.DB or unknown flavor " + string(flavor))}}
	}
	return &driver{db: db, d: d}
}

type driver struct {
	db  *sql.DB
	d   *dialect.Dialect
	err error
}

func (d *driver) Dialect() *dialect.Dialect {
	if d.d == nil {
		return dialect.MySQLFlavor(dialect.FlavorMySQL)
	}
	return d.d
}

// Capabilities reports no COPY or batch path. Vectors is true on MariaDB,
// which has VEC_DISTANCE_EUCLIDEAN and VEC_DISTANCE_COSINE, and false on
// MySQL, which stores VECTOR columns from 9.0 but has no distance function.
func (d *driver) Capabilities() momo.Capabilities {
	return momo.Capabilities{Vectors: d.d != nil && len(d.d.Vector.Metrics) > 0}
}

// ClassifyError implements momo.ErrorClassifier.
func (d *driver) ClassifyError(err error) error { return classifier{}.ClassifyError(err) }

func (d *driver) Query(ctx context.Context, q string, args []any) (momo.Rows, error) {
	if d.err != nil {
		return nil, d.err
	}
	if err := sqlshared.CheckArgs("select", args); err != nil {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, classify(err, q)
	}
	return sqlRows{r: rows, stmt: q}, nil
}

func (d *driver) Exec(ctx context.Context, q string, args []any) (int64, error) {
	if d.err != nil {
		return 0, d.err
	}
	if err := sqlshared.CheckArgs("exec", args); err != nil {
		return 0, err
	}
	res, err := d.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, classify(err, q)
	}
	return affected(res, q)
}

func (d *driver) Begin(ctx context.Context, opts momo.TxOptions) (momo.DriverTx, error) {
	if d.err != nil {
		return nil, d.err
	}
	tx, err := beginTx(ctx, opts, d.db.BeginTx)
	if err != nil {
		return nil, err
	}
	return &sqlTx{tx: tx}, nil
}

// Pin checks one connection out of the pool.
func (d *driver) Pin(ctx context.Context) (momo.PinnedConn, error) {
	if d.err != nil {
		return nil, d.err
	}
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return nil, classify(err, "")
	}
	return &pinned{conn: conn}, nil
}

// PoolStats maps sql.DBStats onto momo.PoolStats.
func (d *driver) PoolStats() momo.PoolStats {
	if d.db == nil {
		return momo.PoolStats{}
	}
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

// ServerVersion is not reported here; migrate/mysql reads VERSION() itself.

func affected(res sql.Result, stmt string) (int64, error) {
	n, err := res.RowsAffected()
	if err != nil {
		return 0, classify(err, stmt)
	}
	return n, nil
}

// txOptions validates opts. MySQL supports the four standard levels.
func txOptions(o momo.TxOptions) error {
	switch o.Isolation {
	case sql.LevelDefault, sql.LevelReadUncommitted, sql.LevelReadCommitted,
		sql.LevelRepeatableRead, sql.LevelSerializable:
		return nil
	}
	return &momo.Error{Kind: momo.ErrUnsupported, Op: "begin",
		Err: fmt.Errorf("mysqldriver: isolation level %v not supported by MySQL", o.Isolation)}
}

// beginTx begins a transaction that outlives ctx: database/sql rolls a
// transaction back when the context given to BeginTx is cancelled, so ctx
// cancels the begin call alone, as with the PostgreSQL adapters.
func beginTx(ctx context.Context, opts momo.TxOptions, begin func(context.Context, *sql.TxOptions) (*sql.Tx, error)) (*sql.Tx, error) {
	if err := txOptions(opts); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, classify(err, "")
	}
	bctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stop := context.AfterFunc(ctx, cancel)
	tx, err := begin(bctx, &sql.TxOptions{Isolation: opts.Isolation, ReadOnly: opts.ReadOnly})
	if !stop() {
		if tx != nil {
			_ = tx.Rollback()
		}
		return nil, classify(ctx.Err(), "")
	}
	if err != nil {
		cancel()
		return nil, classify(err, "")
	}
	return tx, nil
}

// sqlRows adapts *sql.Rows to momo.Rows. sql.Rows.Close is already idempotent.
type sqlRows struct {
	r    *sql.Rows
	stmt string
}

func (r sqlRows) Next() bool             { return r.r.Next() }
func (r sqlRows) Scan(dest ...any) error { return scanErr(classify(r.r.Scan(dest...), r.stmt)) }

// scanErr hides the text of a conversion failure, which quotes the stored
// value; see sqlmask.ConversionErr. Server errors pass through: classify
// already redacts them.
func scanErr(err error) error {
	return sqlmask.ConversionErr(err, "mysql: cannot convert the column value to the destination type", func(err error) bool {
		var se *mysql.MySQLError
		return errors.As(err, &se)
	})
}

// Columns reports the result column names (momo.ColumnNamer), so Query and
// MapRows match columns by name.
func (r sqlRows) Columns() ([]string, error) {
	names, err := r.r.Columns()
	return names, classify(err, r.stmt)
}

func (r sqlRows) Err() error   { return classify(r.r.Err(), r.stmt) }
func (r sqlRows) Close() error { return classify(r.r.Close(), r.stmt) }

// pinnedRows is registered on its pinned connection until closed or exhausted,
// so Release can close Rows the caller forgot.
type pinnedRows struct {
	sqlRows
	p *pinned
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
	if err := sqlshared.CheckArgs("select", args); err != nil {
		return nil, err
	}
	rows, err := t.tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, txErr("select", err, q)
	}
	return sqlRows{r: rows, stmt: q}, nil
}

func (t *sqlTx) Exec(ctx context.Context, q string, args []any) (int64, error) {
	if err := sqlshared.CheckArgs("exec", args); err != nil {
		return 0, err
	}
	res, err := t.tx.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, txErr("exec", err, q)
	}
	return affected(res, q)
}

func (t *sqlTx) Commit(_ context.Context) error {
	defer t.finished()
	return txErr("commit", t.tx.Commit(), "")
}

func (t *sqlTx) Rollback(_ context.Context) error {
	defer t.finished()
	return txErr("rollback", t.tx.Rollback(), "")
}

// txErr maps a finished transaction to momo.ErrTxDone and classifies everything else.
func txErr(op string, err error, stmt string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrTxDone) {
		return &momo.Error{Kind: momo.ErrTxDone, Op: op, Err: err}
	}
	return classify(err, stmt)
}

// pinned is one *sql.Conn checked out of the pool. See momo.PinnedConn for the
// Release contract.
type pinned struct {
	mu    sync.Mutex // guards conn, calls, rows and txs
	conn  *sql.Conn
	calls int                      // Query, Exec and Begin calls executing now
	rows  map[*pinnedRows]struct{} // Rows not yet closed or exhausted
	txs   map[*sql.Tx]struct{}     // transactions begun on this connection and not yet finished
}

var errReleased = errors.New("mysqldriver: pinned connection already released")

func errBusy() error {
	return &momo.Error{Kind: momo.ErrPinnedBusy, Op: "release",
		Err: errors.New("mysqldriver: a statement is executing on the pinned connection")}
}

func (p *pinned) live() (*sql.Conn, error) {
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

func (p *pinned) Query(ctx context.Context, q string, args []any) (momo.Rows, error) {
	if err := sqlshared.CheckArgs("select", args); err != nil {
		return nil, err
	}
	c, err := p.live()
	if err != nil {
		return nil, err
	}
	defer p.done()
	rows, err := c.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, classify(err, q)
	}
	pr := &pinnedRows{sqlRows: sqlRows{r: rows, stmt: q}, p: p}
	p.mu.Lock()
	if p.rows == nil {
		p.rows = make(map[*pinnedRows]struct{})
	}
	p.rows[pr] = struct{}{}
	p.mu.Unlock()
	return pr, nil
}

func (p *pinned) Exec(ctx context.Context, q string, args []any) (int64, error) {
	if err := sqlshared.CheckArgs("exec", args); err != nil {
		return 0, err
	}
	c, err := p.live()
	if err != nil {
		return 0, err
	}
	defer p.done()
	res, err := c.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, classify(err, q)
	}
	return affected(res, q)
}

func (p *pinned) Begin(ctx context.Context, opts momo.TxOptions) (momo.DriverTx, error) {
	c, err := p.live()
	if err != nil {
		return nil, err
	}
	defer p.done()
	tx, err := beginTx(ctx, opts, c.BeginTx)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	if p.txs == nil {
		p.txs = make(map[*sql.Tx]struct{})
	}
	p.txs[tx] = struct{}{}
	p.mu.Unlock()
	return &sqlTx{tx: tx, onDone: func() { p.clearTx(tx) }}, nil
}

func (p *pinned) clearTx(tx *sql.Tx) {
	p.mu.Lock()
	delete(p.txs, tx)
	p.mu.Unlock()
}

// take marks the connection released after closing open Rows. It returns nil,
// nil when already released and errBusy while a call is executing.
func (p *pinned) take() (*sql.Conn, error) {
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
	// An open *sql.Tx holds the Conn's close lock, so roll each back first.
	for tx := range txs {
		_ = tx.Rollback()
	}
	return c, nil
}

// Release closes open Rows, rolls back open transactions, then returns the
// connection to the pool exactly once.
func (p *pinned) Release() error {
	c, err := p.take()
	if c == nil {
		return err
	}
	return classify(c.Close(), "")
}

// Discard closes the underlying driver connection so database/sql destroys it
// instead of reusing a session whose state (GET_LOCK locks, SET values)
// could not be restored. See momo.Discarder.
func (p *pinned) Discard() error {
	c, err := p.take()
	if c == nil {
		return err
	}
	rawErr := c.Raw(func(dc any) error {
		if cl, ok := dc.(interface{ Close() error }); ok {
			_ = cl.Close()
		}
		return sqldriver.ErrBadConn // database/sql drops a connection reporting ErrBadConn
	})
	closeErr := c.Close()
	if rawErr != nil && !errors.Is(rawErr, sqldriver.ErrBadConn) {
		return classify(rawErr, "")
	}
	if closeErr != nil && !errors.Is(closeErr, sql.ErrConnDone) {
		return classify(closeErr, "")
	}
	return nil
}

var (
	_ momo.Driver             = (*driver)(nil)
	_ momo.Discarder          = (*pinned)(nil)
	_ momo.Pinner             = (*driver)(nil)
	_ momo.PoolStater         = (*driver)(nil)
	_ momo.CapabilityReporter = (*driver)(nil)
	_ momo.ErrorClassifier    = (*driver)(nil)
)
