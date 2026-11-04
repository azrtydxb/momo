// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/azrtydxb/momo/dialect"
)

// Querier is implemented by *DB and *Tx. Query builders take a Querier so
// the same code runs inside or outside a transaction. It is sealed.
type Querier interface {
	Dialect() *dialect.Dialect
	conn() *execer
}

// QueryEvent describes one statement for observability hooks.
type QueryEvent struct {
	Op    string // "select", "insert", "update", "delete", "exec", "query", "begin", "commit", "rollback", "savepoint"
	Table string // model table when known
	// Relation is set while eager-loading a relation ("documents.author").
	Relation string
	SQL      string
	// Args is nil unless the DB was opened with WithArgLogging. Treat
	// parameter values as sensitive.
	Args  []any
	Start time.Time
	// Duration is filled for AfterQuery. For reads it covers execution
	// and iteration until the rows are closed.
	Duration time.Duration
	// Rows is the number of rows read (queries) or affected (exec);
	// -1 when unknown.
	Rows int64
	Err  error
}

// Hook observes statements. BeforeQuery may return a derived context (for
// tracing spans); AfterQuery receives that context. Hooks run synchronously
// on the calling goroutine and must be safe for concurrent use.
type Hook interface {
	BeforeQuery(ctx context.Context, e *QueryEvent) context.Context
	AfterQuery(ctx context.Context, e *QueryEvent)
}

// Option configures a DB.
type Option func(*DB)

// WithHook registers an observability hook. Hooks run in registration order.
func WithHook(h Hook) Option { return func(db *DB) { db.hooks = append(db.hooks, h) } }

// WithArgLogging includes bind-parameter values in QueryEvent.Args. It is
// off by default because parameters often contain personal data.
func WithArgLogging() Option { return func(db *DB) { db.logArgs = true } }

// DB is a Momo handle over a driver. It is safe for concurrent use and holds
// no state besides configuration: no background goroutines, no implicit
// migrations, no caches.
type DB struct {
	ex      execer
	hooks   []Hook
	logArgs bool
}

// New wraps a driver. The caller keeps ownership of the underlying pool and
// closes it.
func New(drv Driver, opts ...Option) *DB {
	db := &DB{}
	for _, o := range opts {
		o(db)
	}
	db.ex = execer{db: db, q: drv, d: drv.Dialect()}
	if c, ok := drv.(ErrorClassifier); ok {
		db.ex.classify = c.ClassifyError
	}
	return db
}

func (db *DB) Dialect() *dialect.Dialect { return db.ex.d }
func (db *DB) conn() *execer             { return &db.ex }

// Driver returns the underlying driver, e.g. to reach native features.
func (db *DB) Driver() Driver { return db.ex.q.(Driver) }

// Tx is an explicit transaction handle. It is not safe for concurrent use:
// a transaction is bound to one connection, which processes one statement
// at a time.
type Tx struct {
	ex    execer
	db    *DB
	dtx   DriverTx
	done  atomic.Bool
	depth int // savepoint nesting depth for Tx.Tx
}

func (tx *Tx) Dialect() *dialect.Dialect { return tx.ex.d }
func (tx *Tx) conn() *execer             { return &tx.ex }

type queryExecer interface {
	Query(ctx context.Context, sql string, args []any) (Rows, error)
	Exec(ctx context.Context, sql string, args []any) (int64, error)
}

// execer runs statements with hooks and error classification.
type execer struct {
	db       *DB
	q        queryExecer
	d        *dialect.Dialect
	classify func(error) error
}

func (e *execer) wrapErr(op, table string, err error) error {
	if err == nil {
		return nil
	}
	var me *Error
	if errors.As(err, &me) {
		return err
	}
	if e.classify != nil {
		err = e.classify(err)
		if errors.As(err, &me) {
			if me.Op == "" {
				me.Op = op
			}
			if me.Table == "" {
				me.Table = table
			}
		}
	}
	return err
}

func (e *execer) before(ctx context.Context, op, table, rel, sql string, args []any) (context.Context, *QueryEvent) {
	if len(e.db.hooks) == 0 {
		return ctx, nil
	}
	ev := &QueryEvent{Op: op, Table: table, Relation: rel, SQL: sql, Start: time.Now(), Rows: -1}
	if e.db.logArgs {
		ev.Args = args
	}
	for _, h := range e.db.hooks {
		ctx = h.BeforeQuery(ctx, ev)
	}
	return ctx, ev
}

func (e *execer) after(ctx context.Context, ev *QueryEvent, rows int64, err error) {
	if ev == nil {
		return
	}
	ev.Duration = time.Since(ev.Start)
	ev.Rows = rows
	ev.Err = err
	for _, h := range e.db.hooks {
		h.AfterQuery(ctx, ev)
	}
}

func (e *execer) exec(ctx context.Context, op, table string, c Compiled) (int64, error) {
	ctx, ev := e.before(ctx, op, table, "", c.SQL, c.Args)
	n, err := e.q.Exec(ctx, c.SQL, c.Args)
	err = e.wrapErr(op, table, err)
	if err != nil {
		n = -1
	}
	e.after(ctx, ev, n, err)
	return n, err
}

// query starts a read. The returned rows report to hooks when closed.
func (e *execer) query(ctx context.Context, op, table, rel string, c Compiled) (Rows, error) {
	ctx, ev := e.before(ctx, op, table, rel, c.SQL, c.Args)
	rows, err := e.q.Query(ctx, c.SQL, c.Args)
	if err != nil {
		err = e.wrapErr(op, table, err)
		e.after(ctx, ev, -1, err)
		return nil, err
	}
	if ev == nil && e.classify == nil {
		return rows, nil
	}
	return &hookedRows{Rows: rows, e: e, ctx: ctx, ev: ev, op: op, table: table}, nil
}

type hookedRows struct {
	Rows
	e      *execer
	ctx    context.Context
	ev     *QueryEvent
	op     string
	table  string
	n      int64
	closed bool
}

func (r *hookedRows) Next() bool {
	if r.Rows.Next() {
		r.n++
		return true
	}
	return false
}

func (r *hookedRows) Err() error { return r.e.wrapErr(r.op, r.table, r.Rows.Err()) }

func (r *hookedRows) Close() error {
	err := r.Rows.Close()
	if !r.closed {
		r.closed = true
		ferr := err
		if ferr == nil {
			ferr = r.Rows.Err()
		}
		r.e.after(r.ctx, r.ev, r.n, r.e.wrapErr(r.op, r.table, ferr))
	}
	return err
}

// Exec runs a raw statement. See SQL for the placeholder and safety
// contract: '?' placeholders, "??" for a literal '?'.
func Exec(ctx context.Context, q Querier, sql string, args ...any) (int64, error) {
	c, err := compile(q.Dialect(), func(b *Builder) { b.appendRaw(sql, args) })
	if err != nil {
		return 0, err
	}
	return q.conn().exec(ctx, "exec", "", c)
}

// ----------------------------------------------------------- transactions

// Begin starts an explicit transaction. The caller must Commit or Rollback;
// a deferred Rollback after Commit is a harmless no-op returning ErrTxDone.
func (db *DB) Begin(ctx context.Context, opts TxOptions) (*Tx, error) {
	drv := db.Driver()
	ctx2, ev := db.ex.before(ctx, "begin", "", "", "BEGIN", nil)
	dtx, err := drv.Begin(ctx2, opts)
	db.ex.after(ctx2, ev, -1, err)
	if err != nil {
		return nil, db.ex.wrapErr("begin", "", err)
	}
	tx := &Tx{db: db, dtx: dtx}
	tx.ex = execer{db: db, q: dtx, d: db.ex.d, classify: db.ex.classify}
	return tx, nil
}

// Commit commits the transaction.
func (tx *Tx) Commit(ctx context.Context) error {
	if tx.depth > 0 {
		return errors.New("momo: Commit called on a savepoint scope; return from the Tx callback instead")
	}
	if !tx.done.CompareAndSwap(false, true) {
		return ErrTxDone
	}
	ctx, ev := tx.ex.before(ctx, "commit", "", "", "COMMIT", nil)
	err := tx.dtx.Commit(ctx)
	tx.ex.after(ctx, ev, -1, err)
	return tx.ex.wrapErr("commit", "", err)
}

// Rollback aborts the transaction.
func (tx *Tx) Rollback(ctx context.Context) error {
	if tx.depth > 0 {
		return errors.New("momo: Rollback called on a savepoint scope; return an error from the Tx callback instead")
	}
	if !tx.done.CompareAndSwap(false, true) {
		return ErrTxDone
	}
	ctx, ev := tx.ex.before(ctx, "rollback", "", "", "ROLLBACK", nil)
	err := tx.dtx.Rollback(ctx)
	tx.ex.after(ctx, ev, -1, err)
	return tx.ex.wrapErr("rollback", "", err)
}

// Tx runs fn in a transaction. It commits if fn returns nil and rolls back
// if fn returns an error or panics. A panic is re-raised after the rollback
// completes (the rollback uses a context detached from ctx's cancellation so
// it still reaches the database). fn is called exactly once: Momo never
// retries it.
func (db *DB) Tx(ctx context.Context, opts TxOptions, fn func(tx *Tx) error) (err error) {
	tx, err := db.Begin(ctx, opts)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(context.WithoutCancel(ctx)); rbErr != nil && !errors.Is(rbErr, ErrTxDone) {
			return errors.Join(err, fmt.Errorf("momo: rollback failed: %w", rbErr))
		}
		return err
	}
	return tx.Commit(ctx)
}

// Tx runs fn inside a savepoint of this transaction: an error or panic
// rolls back to the savepoint (and the panic is re-raised); success releases
// it. The outer transaction stays open either way.
func (tx *Tx) Tx(ctx context.Context, fn func(tx *Tx) error) error {
	if !tx.ex.d.Savepoints {
		return &Error{Kind: ErrUnsupported, Op: "savepoint"}
	}
	name := "momo_sp_" + strconv.Itoa(tx.depth+1)
	if err := tx.Savepoint(ctx, name); err != nil {
		return err
	}
	inner := &Tx{db: tx.db, dtx: tx.dtx, depth: tx.depth + 1}
	inner.ex = tx.ex
	ok := false
	defer func() {
		if !ok {
			if p := recover(); p != nil {
				_ = tx.RollbackTo(context.WithoutCancel(ctx), name)
				panic(p)
			}
		}
	}()
	err := fn(inner)
	ok = true
	if err != nil {
		if rbErr := tx.RollbackTo(context.WithoutCancel(ctx), name); rbErr != nil {
			return errors.Join(err, fmt.Errorf("momo: rollback to savepoint failed: %w", rbErr))
		}
		return err
	}
	return tx.Release(ctx, name)
}

// Savepoint creates a named savepoint.
func (tx *Tx) Savepoint(ctx context.Context, name string) error {
	return tx.spExec(ctx, "SAVEPOINT ", name)
}

// RollbackTo rolls back to a named savepoint, keeping it defined.
func (tx *Tx) RollbackTo(ctx context.Context, name string) error {
	return tx.spExec(ctx, "ROLLBACK TO SAVEPOINT ", name)
}

// Release releases a named savepoint.
func (tx *Tx) Release(ctx context.Context, name string) error {
	return tx.spExec(ctx, "RELEASE SAVEPOINT ", name)
}

func (tx *Tx) spExec(ctx context.Context, stmt, name string) error {
	if tx.done.Load() {
		return ErrTxDone
	}
	c, _ := compile(tx.ex.d, func(b *Builder) { b.WriteString(stmt); b.Ident(name) })
	_, err := tx.ex.exec(ctx, "savepoint", "", c)
	return err
}
