// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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
	Args []any
	// NumArgs is always set: the number of bound parameters.
	NumArgs int
	// InTx reports that the statement ran on a *Tx.
	InTx bool
	// PoolWait is the summed RecordPoolWait for this statement.
	PoolWait time.Duration
	// Dialect is the dialect of the DB the statement ran on.
	Dialect *dialect.Dialect
	// Querier is the *DB or *Tx the statement ran on.
	Querier Querier
	Start   time.Time
	// Duration is filled for AfterQuery. For reads it covers execution
	// and iteration until the rows are closed.
	Duration time.Duration
	// Rows is the number of rows read (queries) or affected (exec);
	// -1 when unknown.
	Rows int64
	Err  error
	pool *poolWaitSlot
}

type poolWaitKey struct{}

// poolWaitSlot accumulates the pool wait reported for one statement.
type poolWaitSlot struct{ ns atomic.Int64 }

// RecordPoolWait adds d to the pool wait of the statement whose context is
// ctx. Drivers call it when a connection acquisition waited (pgxdriver does
// so from its pgxpool AcquireTracer). It is a no-op when ctx does not belong
// to a hooked statement, so it costs nothing when no hook is registered.
func RecordPoolWait(ctx context.Context, d time.Duration) {
	if s, ok := ctx.Value(poolWaitKey{}).(*poolWaitSlot); ok {
		s.ns.Add(int64(d))
	}
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
//
// A hook that also implements RelationHook receives relation load events.
func WithHook(h Hook) Option {
	return func(db *DB) {
		db.hooks = append(db.hooks, h)
		if rh, ok := h.(RelationHook); ok {
			db.relHooks = append(db.relHooks, rh)
		}
	}
}

// WithArgLogging includes bind-parameter values in QueryEvent.Args. It is
// off by default because parameters often contain personal data.
func WithArgLogging() Option { return func(db *DB) { db.logArgs = true } }

// DB is a Momo handle over a driver. It is safe for concurrent use and holds
// no state besides configuration: no background goroutines, no implicit
// migrations, no caches.
type DB struct {
	drv      Driver
	ex       execer
	hooks    []Hook
	relHooks []RelationHook
	logArgs  bool
	clock    func() time.Time // nil means time.Now
	pinned   bool             // set by OnConn: statements run on one pinned connection
	// fieldHooks holds the []FieldHook[T] of WithFieldHooks keyed by *Meta[T].
	fieldHooks map[any]any
	// idGens holds the id generators of WithIDGenerator by name.
	idGens map[string]func() string
	// txSetup runs after every transaction momo begins (WithTxSetup).
	txSetup []func(ctx context.Context, tx *Tx) error
	// implicitTx runs auto-commit statements in a short transaction (WithImplicitTx).
	implicitTx bool
}

// New wraps a driver. The caller keeps ownership of the underlying pool and
// closes it.
func New(drv Driver, opts ...Option) *DB {
	db := &DB{drv: drv}
	// The execer is set up before the options run so that options such as
	// WithMaxParams can adjust the dialect it carries.
	db.ex = execer{db: db, q: drv, d: drv.Dialect(), owner: db}
	if c, ok := drv.(ErrorClassifier); ok {
		db.ex.classify = c.ClassifyError
	}
	for _, o := range opts {
		o(db)
	}
	return db
}

func (db *DB) Dialect() *dialect.Dialect { return db.ex.d }
func (db *DB) conn() *execer             { return &db.ex }

// Driver returns the underlying driver, e.g. to reach native features.
func (db *DB) Driver() Driver { return db.drv }

// Tx is an explicit transaction handle. It is not safe for concurrent use:
// a transaction is bound to one connection, which processes one statement
// at a time.
type Tx struct {
	ex    execer
	dtx   DriverTx
	done  *atomic.Bool // shared with savepoint scopes of the same transaction
	depth int          // savepoint nesting depth for Tx.Tx
	// adopted marks a Tx bound to a transaction the caller began (AdoptTx):
	// Commit and Rollback refuse and leave the transaction to its owner.
	adopted bool
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
	inTx     bool
	owner    Querier
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

// RedactedArg replaces, in QueryEvent.Args, every value bound from a column
// tagged sensitive. Only the copy handed to hooks is masked.
const RedactedArg = "[redacted]"

// maskArgs returns a copy of args with the sensitive positions replaced, so a
// hook can neither read those values nor mutate the slice sent to the driver.
// Byte slices are copied too, so changing their contents in place does not reach
// the driver. It runs only when hooks are registered.
func maskArgs(args []any, sens []int) []any {
	if args == nil {
		return nil
	}
	out := slices.Clone(args)
	for i, a := range out {
		switch v := a.(type) {
		case []byte:
			out[i] = slices.Clone(v)
		case json.RawMessage:
			out[i] = slices.Clone(v)
		}
	}
	for _, i := range sens {
		if i >= 0 && i < len(out) {
			out[i] = RedactedArg
		}
	}
	return out
}

func (e *execer) before(ctx context.Context, op, table, rel, sql string, args []any, sens []int) (context.Context, *QueryEvent) {
	if len(e.db.hooks) == 0 {
		return ctx, nil
	}
	slot := &poolWaitSlot{}
	ctx = context.WithValue(ctx, poolWaitKey{}, slot)
	ev := &QueryEvent{
		Op: op, Table: table, Relation: rel, SQL: sql,
		NumArgs: len(args), InTx: e.inTx, Dialect: e.d, Querier: e.owner,
		Start: time.Now(), Rows: -1, pool: slot,
	}
	if e.db.logArgs {
		ev.Args = maskArgs(args, sens)
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
	ev.PoolWait = time.Duration(ev.pool.ns.Load())
	ev.Rows = rows
	ev.Err = err
	for _, h := range e.db.hooks {
		h.AfterQuery(ctx, ev)
	}
}

func (e *execer) exec(ctx context.Context, op, table string, c Compiled) (int64, error) {
	if e.unscopable() {
		return -1, errPinnedSetup(op, table)
	}
	if e.implicit() {
		return e.implicitExec(ctx, op, table, c)
	}
	ctx, ev := e.before(ctx, op, table, "", c.SQL, c.Args, c.sens)
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
	if e.unscopable() {
		return nil, errPinnedSetup(op, table)
	}
	if e.implicit() {
		return e.implicitQuery(ctx, op, table, rel, c)
	}
	ctx, ev := e.before(ctx, op, table, rel, c.SQL, c.Args, c.sens)
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
	// scanErr is the first Scan failure; Close reports it to AfterQuery.
	scanErr error
}

// Scan records a failure so the statement's AfterQuery carries it.
func (r *hookedRows) Scan(dest ...any) error {
	err := r.Rows.Scan(dest...)
	if err != nil && r.scanErr == nil {
		r.scanErr = err
	}
	return err
}

// abort closes the statement's hook pair with err unless it already ended.
func (r *hookedRows) abort(err error) {
	if r.closed {
		return
	}
	r.closed = true
	_ = r.Rows.Close()
	r.e.after(r.ctx, r.ev, r.n, r.e.wrapErr(r.op, r.table, err))
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
		if ferr == nil {
			ferr = r.scanErr
		}
		r.e.after(r.ctx, r.ev, r.n, r.e.wrapErr(r.op, r.table, ferr))
	}
	return err
}

// Exec runs a raw statement. See SQL for the placeholder and safety
// contract: '?' placeholders, "??" for a literal '?', and on PostgreSQL
// native $n placeholders.
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
	if db.pinned { // a handle from OnConn
		return nil, &Error{Kind: ErrUnsupported, Op: "begin", Err: errors.New("transactions are not available on a pinned connection")}
	}
	drv := db.Driver()
	ctx2, ev := db.ex.before(ctx, "begin", "", "", "BEGIN", nil, nil)
	dtx, err := drv.Begin(ctx2, opts)
	db.ex.after(ctx2, ev, -1, err)
	if err != nil {
		return nil, db.ex.wrapErr("begin", "", err)
	}
	tx := &Tx{dtx: dtx, done: new(atomic.Bool)}
	tx.ex = execer{db: db, q: dtx, d: db.ex.d, classify: db.ex.classify, inTx: true, owner: tx}
	if err := db.runTxSetup(ctx2, tx); err != nil {
		return nil, err
	}
	return tx, nil
}

// AdoptTx binds a Tx to dtx, a driver transaction the caller already began
// and keeps ownership of. Statements, hooks, error classification, the
// dialect and savepoints (Tx.Tx, Savepoint, ...) behave as on a Tx begun by
// DB.Begin, but Commit and Rollback on the returned Tx (not on a savepoint
// scope, which still releases and rolls back its savepoint) return an error
// matching ErrAdoptedTx and never touch dtx: the caller commits or rolls back
// its own transaction. Obtain a dtx from the driver package, for example
// pgxdriver.SQLTx or pgxdriver.PgxTx. Once the caller finishes the
// transaction, further statements fail with the driver's finished-transaction
// error (ErrTxDone). AdoptTx panics when dtx is nil, a programming error like
// a nil Driver in New. It does not fire begin hooks: momo did not begin it.
func (db *DB) AdoptTx(dtx DriverTx) *Tx {
	if dtx == nil {
		panic("momo: AdoptTx called with a nil DriverTx")
	}
	tx := &Tx{dtx: dtx, done: new(atomic.Bool), adopted: true}
	tx.ex = execer{db: db, q: dtx, d: db.ex.d, classify: db.ex.classify, inTx: true, owner: tx}
	return tx
}

// DriverTx returns the driver-level transaction behind tx, for driver
// packages that unwrap it (pgxdriver.UnwrapSQLTx, pgxdriver.UnwrapPgxTx). A
// savepoint scope returns the same outer transaction. Calling Commit or
// Rollback on it directly bypasses Momo's bookkeeping; prefer the unwrap
// helpers of the driver package.
func (tx *Tx) DriverTx() DriverTx { return tx.dtx }

// Commit commits the transaction. On a Tx from AdoptTx it returns an error
// matching ErrAdoptedTx and does nothing.
func (tx *Tx) Commit(ctx context.Context) error {
	if tx.depth > 0 {
		return errors.New("momo: Commit called on a savepoint scope; return from the Tx callback instead")
	}
	if tx.adopted {
		return &Error{Kind: ErrAdoptedTx, Op: "commit"}
	}
	if !tx.done.CompareAndSwap(false, true) {
		return ErrTxDone
	}
	ctx, ev := tx.ex.before(ctx, "commit", "", "", "COMMIT", nil, nil)
	err := tx.dtx.Commit(ctx)
	tx.ex.after(ctx, ev, -1, err)
	return tx.ex.wrapErr("commit", "", err)
}

// Rollback aborts the transaction. On a Tx from AdoptTx it returns an error
// matching ErrAdoptedTx and does nothing.
func (tx *Tx) Rollback(ctx context.Context) error {
	if tx.depth > 0 {
		return errors.New("momo: Rollback called on a savepoint scope; return an error from the Tx callback instead")
	}
	if tx.adopted {
		return &Error{Kind: ErrAdoptedTx, Op: "rollback"}
	}
	if !tx.done.CompareAndSwap(false, true) {
		return ErrTxDone
	}
	ctx, ev := tx.ex.before(ctx, "rollback", "", "", "ROLLBACK", nil, nil)
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

// inTx runs fn in a new transaction when q is a *DB and in a savepoint when q
// is a *Tx. A *DB pinned with OnConn cannot begin a transaction: the call fails
// with ErrUnsupported (Op "begin") before fn runs.
func inTx(ctx context.Context, q Querier, fn func(tx *Tx) error) error {
	switch qq := q.(type) {
	case *DB:
		return qq.Tx(ctx, TxOptions{}, fn)
	case *Tx:
		return qq.Tx(ctx, fn)
	}
	return &Error{Kind: ErrUnsupported, Op: "begin", Err: errors.New("unknown Querier")}
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
	inner := &Tx{dtx: tx.dtx, done: tx.done, depth: tx.depth + 1, adopted: tx.adopted}
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
	b := NewBuilder(tx.ex.d)
	b.WriteString(stmt)
	b.Ident(name)
	_, err := tx.ex.exec(ctx, "savepoint", "", Compiled{SQL: b.String()})
	return err
}
