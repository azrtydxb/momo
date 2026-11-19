// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"database/sql"
	sqldriver "database/sql/driver"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/sqlmask"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
)

// FromSQLDB is the old name of NewSQL.
//
// Deprecated: use NewSQL. FromSQLDB is removed in the next release.
func FromSQLDB(db *sql.DB) momo.Driver { return NewSQL(db) }

// NewSQL adopts an existing *sql.DB opened with the pgx stdlib driver. The execution mode is the one
// set in that connection's pgx config; NewSQL cannot change it. The caller owns db and closes it; the
// driver never does. Use OpenSQL to have the driver open and own the pool.
//
// opts is accepted so that every constructor has one shape. The pool is already open, so no option of the
// native driver can apply to it: an option given makes every call of the returned driver fail with
// momo.ErrValidation, naming the option. Set the exec mode on the *pgx.ConnConfig before stdlib.OpenDB.
//
// The returned driver supports pinning, pool statistics and error classification. It implements neither
// momo.Copier nor momo.Batcher.
//
// A nil db, or a db that is not backed by the pgx stdlib driver, is rejected: every Query, Exec, Begin and
// Pin then returns momo.ErrUnsupported. So is a db whose pgx connections use simple protocol, which
// interpolates arguments client-side; as database/sql hides the pgx config, this is detected on the first
// statement, from a connection, rather than at construction.
//
// Differences from the native driver (New):
//
//   - Scanning goes through database/sql conversion, not pgx type mapping. Arrays, jsonb and the pgvector
//     types arrive as database/sql sees them (the vector types implement sql.Scanner and driver.Valuer,
//     which is what is used); destinations that rely on pgx-native decoding, for example a []int32 for
//     int4[], need a sql.Scanner.
//   - Transactions outlive the context passed to Begin, as with pgxpool: that context bounds only the
//     begin call. database/sql would otherwise roll the transaction back when it is cancelled.
//   - Supported isolation levels are those of the native driver; anything else is momo.ErrUnsupported.
//     database/sql with pgx does not send READ WRITE for TxOptions{ReadOnly: false}, so such a
//     transaction takes the session's default_transaction_read_only (the native driver sends it explicitly).
//   - A transaction still open on a pinned connection when it is released or discarded is rolled back.
func NewSQL(db *sql.DB, opts ...Option) momo.Driver {
	if _, set, err := sqlOptions(opts); err != nil || set {
		if err == nil {
			err = sqlOptionError("WithExecMode", "set DefaultQueryExecMode on the *pgx.ConnConfig before stdlib.OpenDB")
		}
		return &sqlDriver{err: err}
	}
	return newSQLDriver(db)
}

// sqlOptions applies opts to a probe and reports what a database/sql pool can
// take from them: the exec mode, when WithExecMode was given. Any other option
// cannot apply, and an exec mode that simple protocol or an unknown mode
// makes unusable is the error WithExecMode itself records.
func sqlOptions(opts []Option) (mode pgx.QueryExecMode, set bool, err error) {
	const unset = pgx.QueryExecMode(-1)
	p := &driver{mode: unset}
	for _, o := range opts {
		o(p)
	}
	if p.vecFormat != VectorText {
		return 0, false, sqlOptionError("WithVectorFormat", "the pgx vector codec belongs to the native pool; register it on your own pgx config")
	}
	if p.modeErr != nil {
		return 0, false, p.modeErr
	}
	return p.mode, p.mode != unset, nil
}

// sqlOptionError is the momo.ErrValidation for an option a database/sql pool cannot take.
func sqlOptionError(name, hint string) error {
	return &momo.Error{Kind: momo.ErrValidation, Op: "build", Err: errors.New("pgxdriver: " + name + " cannot apply to a database/sql pool: " + hint)}
}

func newSQLDriver(db *sql.DB) *sqlDriver {
	if db == nil {
		return &sqlDriver{err: unsupportedDB("nil *sql.DB")}
	}
	if _, ok := db.Driver().(*stdlib.Driver); !ok {
		return &sqlDriver{db: db, err: unsupportedDB("*sql.DB is not backed by the pgx stdlib driver")}
	}
	return &sqlDriver{db: db}
}

// OpenSQL parses dsn, opens a *sql.DB on it with the pgx stdlib driver and returns a driver that owns
// it: close it through the io.Closer the returned driver implements, which closes the database. The
// execution mode is the one the DSN selects, or the one WithExecMode sets: pgx.ParseConfig defaults it to
// pgx.QueryExecModeCacheStatement, which is not PgBouncer transaction-mode safe, so a program behind PgBouncer
// gives WithExecMode(pgx.QueryExecModeExec) or adds default_query_exec_mode=exec to the DSN. Simple protocol
// (from the DSN or from WithExecMode) is refused with momo.ErrUnsupported, and so is any other option of the
// native driver, such as WithVectorFormat, with momo.ErrValidation: a database/sql pool cannot take it.
// OpenSQL pings the server on ctx, as Open does, so a bad DSN, a refused connection or bad credentials are
// reported here. On any error the database is closed and nothing is returned. See NewSQL for what the
// database/sql path supports.
func OpenSQL(ctx context.Context, dsn string, opts ...Option) (momo.Driver, error) {
	mode, set, err := sqlOptions(opts)
	if err != nil {
		return nil, err
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if set {
		cfg.DefaultQueryExecMode = mode
	}
	db, d := openConnConfig(cfg)
	if d.err != nil {
		_ = db.Close()
		return nil, d.err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, ClassifyError(err)
	}
	return &ownedSQLDriver{sqlDriver: d, db: db}, nil
}

// ownedSQLDriver is the driver OpenSQL returns: it owns the database and closes it.
type ownedSQLDriver struct {
	*sqlDriver
	db *sql.DB
}

// Close closes the database the driver opened.
func (o *ownedSQLDriver) Close() error { return o.db.Close() }

func unsupportedDB(why string) error {
	return &momo.Error{Kind: momo.ErrUnsupported, Op: "build", Err: errors.New("pgxdriver: " + why)}
}

// FromConnConfig opens a *sql.DB from cfg with stdlib.OpenDB and adopts it. cfg.DefaultQueryExecMode
// is the execution mode of every statement and is used as given: pgx.ParseConfig defaults it to
// pgx.QueryExecModeCacheStatement, which is not PgBouncer transaction-mode safe, so a caller behind
// PgBouncer must set cfg.DefaultQueryExecMode = pgx.QueryExecModeExec. opts is accepted for signature
// parity with New; the database/sql path uses only error classification, so opts have no effect here.
// The caller closes the returned *sql.DB.
//
// Simple protocol (pgx.QueryExecModeSimpleProtocol, or prefer_simple_protocol in a DSN) interpolates
// arguments client-side, which breaks the rule that values are always bound. A cfg that selects it is
// refused: the returned driver then fails every Query, Exec, Begin and Pin with momo.ErrUnsupported,
// and the returned *sql.DB is still returned so the caller can close it.
//
// Deprecated: use OpenSQL, which takes a DSN and returns a driver that owns the pool, or
// stdlib.OpenDB(*cfg) with NewSQL for a *pgx.ConnConfig you built. FromConnConfig is removed in the
// next release.
func FromConnConfig(cfg *pgx.ConnConfig, opts ...Option) (*sql.DB, momo.Driver) {
	_ = opts
	db, d := openConnConfig(cfg)
	return db, d
}

// openConnConfig opens the *sql.DB of cfg and adopts it, refusing simple protocol.
func openConnConfig(cfg *pgx.ConnConfig) (*sql.DB, *sqlDriver) {
	db := stdlib.OpenDB(*cfg)
	d := newSQLDriver(db)
	if d.err == nil {
		if cfg.DefaultQueryExecMode == pgx.QueryExecModeSimpleProtocol {
			d.err = errSimpleProtocol
		} else {
			d.modeOK.Store(true)
		}
	}
	return db, d
}

// errSimpleProtocol is returned by a database/sql driver whose pgx connections use simple protocol.
var errSimpleProtocol = &momo.Error{
	Kind: momo.ErrUnsupported,
	Op:   "build",
	Err:  errors.New("pgxdriver: simple protocol interpolates arguments client-side; use QueryExecModeExec or another extended-protocol mode"),
}

// ready reports the construction error, then, once, checks the execution mode of the pgx connections
// behind an adopted *sql.DB. database/sql hides the pgx config, so NewSQL can only look at it through
// a connection: the first statement checks one out, reads its DefaultQueryExecMode and refuses simple
// protocol for good. A failure to get a connection is returned and the check repeats on the next call.
func (d *sqlDriver) ready(ctx context.Context) error {
	if d.modeOK.Load() {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return d.err
	}
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return ClassifyError(err)
	}
	defer func() { _ = conn.Close() }()
	simple := false
	rawErr := conn.Raw(func(dc any) error {
		if sc, ok := dc.(*stdlib.Conn); ok && sc.Conn().Config().DefaultQueryExecMode == pgx.QueryExecModeSimpleProtocol {
			simple = true
		}
		return nil
	})
	if rawErr != nil {
		return ClassifyError(rawErr)
	}
	if simple {
		d.err = errSimpleProtocol
		return d.err
	}
	d.modeOK.Store(true)
	return nil
}

type sqlDriver struct {
	db  *sql.DB
	err error // non-nil when the adopted database was rejected; guarded by mu once ready may run
	mu  sync.Mutex
	// modeOK is set once the connections are known not to use simple protocol.
	modeOK atomic.Bool
}

func (d *sqlDriver) Dialect() *dialect.Dialect { return dialect.For(dialect.Postgres) }

// Capabilities reports vector support, as New does. There is no COPY or batch path.
func (d *sqlDriver) Capabilities() momo.Capabilities { return momo.Capabilities{Vectors: true} }

// ClassifyError implements momo.ErrorClassifier. database/sql does not wrap driver
// errors, so the *pgconn.PgError is visible to ClassifyError.
func (d *sqlDriver) ClassifyError(err error) error { return ClassifyError(err) }

func (d *sqlDriver) Query(ctx context.Context, q string, args []any) (momo.Rows, error) {
	if err := d.ready(ctx); err != nil {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return sqlRows{rows}, nil
}

func (d *sqlDriver) Exec(ctx context.Context, q string, args []any) (int64, error) {
	if err := d.ready(ctx); err != nil {
		return 0, err
	}
	res, err := d.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, ClassifyError(err)
	}
	return rowsAffected(res)
}

func (d *sqlDriver) Begin(ctx context.Context, opts momo.TxOptions) (momo.DriverTx, error) {
	if err := d.ready(ctx); err != nil {
		return nil, err
	}
	tx, err := beginTx(ctx, opts, d.db.BeginTx)
	if err != nil {
		return nil, err
	}
	return &sqlTx{tx: tx}, nil
}

// beginTx validates opts as the native driver does, then begins a transaction that outlives ctx. database/sql
// rolls a transaction back when the context given to BeginTx is cancelled; pgxpool only uses it for the begin
// call. ctx therefore cancels the begin call alone.
func beginTx(ctx context.Context, opts momo.TxOptions, begin func(context.Context, *sql.TxOptions) (*sql.Tx, error)) (*sql.Tx, error) {
	if _, err := txOptions(opts); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, ClassifyError(err)
	}
	bctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stop := context.AfterFunc(ctx, cancel)
	tx, err := begin(bctx, &sql.TxOptions{Isolation: opts.Isolation, ReadOnly: opts.ReadOnly})
	if !stop() {
		// ctx was cancelled while beginning: database/sql may already be rolling tx back.
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
	if err := d.ready(ctx); err != nil {
		return nil, err
	}
	conn, err := d.db.Conn(ctx)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return &sqlPinned{conn: conn}, nil
}

// PoolStats maps sql.DBStats onto momo.PoolStats.
func (d *sqlDriver) PoolStats() momo.PoolStats {
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
func (r sqlRows) Scan(dest ...any) error { return scanErr(ClassifyError(r.r.Scan(dest...))) }

// scanErr hides the text of a conversion failure, which quotes the stored
// value; see sqlmask.ConversionErr. Server errors pass through.
func scanErr(err error) error {
	return sqlmask.ConversionErr(err, "pgx: cannot convert the column value to the destination type", func(err error) bool {
		var pe *pgconn.PgError
		return errors.As(err, &pe)
	})
}

func (r sqlRows) Err() error   { return ClassifyError(r.r.Err()) }
func (r sqlRows) Close() error { return ClassifyError(r.r.Close()) }

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

func (t *sqlTx) Commit(_ context.Context) error {
	defer t.finished()
	return sqlTxErr("commit", t.tx.Commit())
}

func (t *sqlTx) Rollback(_ context.Context) error {
	defer t.finished()
	return sqlTxErr("rollback", t.tx.Rollback())
}

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
	mu    sync.Mutex // guards conn, calls, rows and txs
	conn  *sql.Conn
	calls int                         // Query, Exec and Begin calls executing now
	rows  map[*pinnedSQLRows]struct{} // Rows not yet closed or exhausted
	txs   map[*sql.Tx]struct{}        // transactions begun on this connection and not yet finished
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

func (p *sqlPinned) clearTx(tx *sql.Tx) {
	p.mu.Lock()
	delete(p.txs, tx)
	p.mu.Unlock()
}

// take marks the connection released after closing open Rows. It returns nil, nil when
// already released and errBusy while a call is executing.
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
	// Each open *sql.Tx holds the Conn's close lock, so Conn.Close would block forever. The native
	// contract discards an open transaction with the session: roll them all back first, outside
	// the lock because Rollback is network I/O.
	for tx := range txs {
		_ = tx.Rollback()
	}
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
			return errNotPgxConn
		}
		if err := sc.Conn().Close(ctx); err != nil {
			return err
		}
		return sqldriver.ErrBadConn // database/sql drops a connection reporting ErrBadConn
	})
	closeErr := c.Close()
	if errors.Is(rawErr, errNotPgxConn) {
		return unsupportedDB("pinned connection is not a pgx stdlib connection; it was returned to the pool, not destroyed")
	}
	if rawErr != nil && !errors.Is(rawErr, sqldriver.ErrBadConn) {
		return ClassifyError(rawErr)
	}
	if closeErr != nil && !errors.Is(closeErr, sql.ErrConnDone) {
		return ClassifyError(closeErr)
	}
	return nil
}

var errNotPgxConn = errors.New("not a *stdlib.Conn")

var (
	_ momo.Discarder          = (*sqlPinned)(nil)
	_ momo.Pinner             = (*sqlDriver)(nil)
	_ momo.PoolStater         = (*sqlDriver)(nil)
	_ momo.CapabilityReporter = (*sqlDriver)(nil)
)

// Columns reports the result column names (momo.ColumnNamer).
func (r sqlRows) Columns() ([]string, error) {
	names, err := r.r.Columns()
	return names, ClassifyError(err)
}
