// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Option configures a driver created by New.
type Option func(*driver)

// WithExecMode sets the pgx query execution mode for every statement and
// transaction this driver sends. Accepted: pgx.QueryExecModeExec (default:
// unnamed statements, server-side binding, PgBouncer-safe),
// pgx.QueryExecModeCacheStatement, pgx.QueryExecModeCacheDescribe and
// pgx.QueryExecModeDescribeExec. pgx.QueryExecModeSimpleProtocol (and any
// other mode) is rejected: simple protocol interpolates arguments client-side,
// which breaks the rule that values are always bound. A rejected mode makes
// every Query, Exec and Begin return momo.ErrUnsupported.
func WithExecMode(m pgx.QueryExecMode) Option {
	return func(d *driver) {
		switch m {
		case pgx.QueryExecModeExec,
			pgx.QueryExecModeCacheStatement,
			pgx.QueryExecModeCacheDescribe,
			pgx.QueryExecModeDescribeExec:
			d.mode = m
			d.modeErr = nil
		default:
			d.mode = m
			d.modeErr = &momo.Error{
				Kind: momo.ErrUnsupported,
				Op:   "build",
				Err:  errors.New("simple protocol interpolates arguments client-side"),
			}
		}
	}
}

type driver struct {
	pool      *pgxpool.Pool
	mode      pgx.QueryExecMode
	modeErr   error
	vecFormat VectorFormat
}

// New wraps a pgxpool. The caller owns the pool and closes it.
func New(pool *pgxpool.Pool, opts ...Option) momo.Driver {
	d := &driver{pool: pool, mode: pgx.QueryExecModeExec}
	for _, o := range opts {
		o(d)
	}
	return d
}

func (d *driver) Dialect() *dialect.Dialect { return dialect.For(dialect.Postgres) }

// Capabilities reports that this driver can execute vector SQL. The pgvector
// extension itself is checked by the vector package.
func (d *driver) Capabilities() momo.Capabilities {
	return momo.Capabilities{Vectors: true, Batch: true, Copy: true}
}

// ClassifyError implements momo.ErrorClassifier.
func (d *driver) ClassifyError(err error) error { return ClassifyError(err) }

// withMode prepends the exec mode, which pgx reads from the first argument.
func (d *driver) withMode(args []any) []any {
	out := make([]any, 0, len(args)+1)
	out = append(out, d.mode)
	return append(out, convertVectorArgs(d.vecFormat, args)...)
}

func (d *driver) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	if d.modeErr != nil {
		return nil, d.modeErr
	}
	rows, err := d.pool.Query(ctx, sql, d.withMode(args)...)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return pgxRows{rows}, nil
}

func (d *driver) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	if d.modeErr != nil {
		return 0, d.modeErr
	}
	tag, err := d.pool.Exec(ctx, sql, d.withMode(args)...)
	if err != nil {
		return 0, ClassifyError(err)
	}
	return tag.RowsAffected(), nil
}

func (d *driver) Begin(ctx context.Context, opts momo.TxOptions) (momo.DriverTx, error) {
	if d.modeErr != nil {
		return nil, d.modeErr
	}
	to, err := txOptions(opts)
	if err != nil {
		return nil, err
	}
	tx, err := d.pool.BeginTx(ctx, to)
	if err != nil {
		return nil, ClassifyError(err)
	}
	return &pgxTx{tx: tx, d: d}, nil
}

// txOptions maps momo.TxOptions onto pgx.TxOptions.
func txOptions(o momo.TxOptions) (pgx.TxOptions, error) {
	var to pgx.TxOptions
	switch o.Isolation {
	case sql.LevelDefault:
	case sql.LevelReadUncommitted:
		to.IsoLevel = pgx.ReadUncommitted
	case sql.LevelReadCommitted:
		to.IsoLevel = pgx.ReadCommitted
	case sql.LevelRepeatableRead:
		to.IsoLevel = pgx.RepeatableRead
	case sql.LevelSerializable:
		to.IsoLevel = pgx.Serializable
	default:
		return to, &momo.Error{
			Kind: momo.ErrUnsupported,
			Op:   "begin",
			Err:  errors.New("isolation level not supported by PostgreSQL"),
		}
	}
	if o.ReadOnly {
		to.AccessMode = pgx.ReadOnly
	} else {
		to.AccessMode = pgx.ReadWrite
	}
	return to, nil
}

// pgxRows adapts pgx.Rows, whose Close has no result, to momo.Rows.
type pgxRows struct{ pgx.Rows }

func (r pgxRows) Scan(dest ...any) error { return ClassifyError(r.Rows.Scan(dest...)) }
func (r pgxRows) Err() error             { return ClassifyError(r.Rows.Err()) }
func (r pgxRows) Close() error           { r.Rows.Close(); return nil }

type pgxTx struct {
	tx pgx.Tx
	d  *driver
	// done is set once Commit or Rollback has run, whatever their outcome: pgx
	// (dbTx.Commit and dbTx.Rollback) marks the transaction closed before it
	// looks at the Exec error and then closes or kills the connection, so a
	// failed Commit or Rollback leaves nothing to send a batch on. pgx.Tx.Conn
	// still returns the connection afterwards (it may already serve other
	// users of the pool), so connection-level calls such as SendBatch check
	// done first.
	done atomic.Bool
}

func (t *pgxTx) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	rows, err := t.tx.Query(ctx, sql, t.d.withMode(args)...)
	if err != nil {
		return nil, txErr("select", err)
	}
	return pgxRows{rows}, nil
}

func (t *pgxTx) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	tag, err := t.tx.Exec(ctx, sql, t.d.withMode(args)...)
	if err != nil {
		return 0, txErr("exec", err)
	}
	return tag.RowsAffected(), nil
}

func (t *pgxTx) Commit(ctx context.Context) error {
	err := t.tx.Commit(ctx)
	t.done.Store(true)
	return txErr("commit", err)
}

func (t *pgxTx) Rollback(ctx context.Context) error {
	err := t.tx.Rollback(ctx)
	t.done.Store(true)
	return txErr("rollback", err)
}

// txErr maps a finished pgx transaction to momo.ErrTxDone and classifies
// everything else.
func txErr(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrTxClosed) {
		return &momo.Error{Kind: momo.ErrTxDone, Op: op, Err: err}
	}
	return ClassifyError(err)
}
