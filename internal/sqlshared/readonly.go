// SPDX-License-Identifier: Apache-2.0

package sqlshared

import (
	"context"
	"database/sql"
	"sync"

	"github.com/azrtydxb/momo"
)

// beginReadOnly turns query_only on for conn and starts a deferred
// transaction on it with a plain BEGIN. database/sql cannot be used here:
// the SQLite drivers begin every sql.Tx with the DSN's _txlock (immediate by
// default), and BEGIN IMMEDIATE fails under query_only. roTx resets the pragma
// when the transaction ends.
func (h *Hooks) beginReadOnly(ctx context.Context, conn *sql.Conn) (*roTx, error) {
	if err := ctx.Err(); err != nil {
		return nil, h.Classify("begin", err)
	}
	if err := beginStmt(ctx, conn, "PRAGMA query_only=ON"); err != nil {
		// The pragma may have taken effect before the error (a context cancelled
		// while the statement ran), and the connection goes back to the pool.
		resetQueryOnly(conn)
		return nil, h.Classify("begin", err)
	}
	if err := beginStmt(ctx, conn, "BEGIN DEFERRED"); err != nil {
		resetQueryOnly(conn)
		return nil, h.Classify("begin", err)
	}
	return &roTx{conn: conn, h: h}, nil
}

// AfterBeginStmtHook lets a test make a statement of beginReadOnly report an
// error after it ran. It is for tests only and is nil otherwise; the same
// restrictions as ExecStmtHook apply.
var AfterBeginStmtHook func(stmt string) error

func beginStmt(ctx context.Context, conn *sql.Conn, stmt string) error {
	if _, err := conn.ExecContext(ctx, stmt); err != nil {
		return err
	}
	if h := AfterBeginStmtHook; h != nil {
		return h(stmt)
	}
	return nil
}

// roTx is a read-only transaction driven with BEGIN and COMMIT statements on
// one *sql.Conn.
type roTx struct {
	conn   *sql.Conn
	h      *Hooks
	mu     sync.Mutex
	done   bool
	rows   map[*roRows]struct{} // Rows not yet closed or exhausted
	onDone func()               // called once, after the transaction ended; may be nil
}

// roRows is registered on its transaction until closed or exhausted, so the
// transaction can close Rows the caller forgot: an open Rows holds the
// connection's close lock and conn.Close would block on it.
type roRows struct {
	sqlRows
	t *roTx
}

func (r *roRows) finish() {
	r.t.mu.Lock()
	delete(r.t.rows, r)
	r.t.mu.Unlock()
}

func (r *roRows) Next() bool {
	ok := r.sqlRows.Next()
	if !ok {
		r.finish()
	}
	return ok
}

func (r *roRows) Close() error {
	err := r.sqlRows.Close()
	r.finish()
	return err
}

func (t *roTx) live(op string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.done {
		return &momo.Error{Kind: momo.ErrTxDone, Op: op, Err: sql.ErrTxDone}
	}
	return nil
}

func (t *roTx) Query(ctx context.Context, q string, args []any) (momo.Rows, error) {
	if err := t.live("select"); err != nil {
		return nil, err
	}
	rows, err := t.conn.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, t.h.Classify("select", err)
	}
	rr := &roRows{sqlRows: sqlRows{r: rows, h: t.h}, t: t}
	t.mu.Lock()
	if t.done {
		// The transaction ended while the query ran.
		t.mu.Unlock()
		_ = rows.Close()
		return nil, &momo.Error{Kind: momo.ErrTxDone, Op: "select", Err: sql.ErrTxDone}
	}
	if t.rows == nil {
		t.rows = make(map[*roRows]struct{})
	}
	t.rows[rr] = struct{}{}
	t.mu.Unlock()
	return rr, nil
}

func (t *roTx) Exec(ctx context.Context, q string, args []any) (int64, error) {
	if err := t.live("exec"); err != nil {
		return 0, err
	}
	res, err := t.conn.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, t.h.Classify("exec", err)
	}
	return t.h.rowsAffected(res)
}

func (t *roTx) Commit(ctx context.Context) error   { return t.finish(ctx, "COMMIT", "commit") }
func (t *roTx) Rollback(ctx context.Context) error { return t.finish(ctx, "ROLLBACK", "rollback") }

// abort rolls the transaction back without a caller context. Release uses it.
func (t *roTx) abort() { _ = t.finish(context.Background(), "ROLLBACK", "rollback") }

func (t *roTx) finish(ctx context.Context, stmt, op string) error {
	t.mu.Lock()
	if t.done {
		t.mu.Unlock()
		return &momo.Error{Kind: momo.ErrTxDone, Op: op, Err: sql.ErrTxDone}
	}
	t.done = true
	open := t.rows
	t.rows = nil
	t.mu.Unlock()
	// Close Rows the caller left open before the statement and before the
	// connection is released, as Release does for a pinned connection.
	for r := range open {
		_ = r.sqlRows.r.Close()
	}
	// The caller's context may be cancelled already; the transaction must
	// still end, so the statement runs on a context that outlives it.
	ectx, cancel := context.WithTimeout(context.WithoutCancel(ctx), resetTimeout)
	defer cancel()
	err := t.run(ectx, stmt)
	if err != nil && stmt == "COMMIT" {
		// A failed COMMIT can leave the transaction open (SQLITE_BUSY); end it so
		// the connection returns to the pool in autocommit mode. If that fails
		// too the connection's state is unknown, so it is discarded.
		if rerr := t.run(ectx, "ROLLBACK"); rerr != nil {
			_ = t.conn.Raw(func(any) error { return errBadConn })
			if t.onDone != nil {
				t.onDone()
			}
			return t.h.Classify(op, err)
		}
	}
	resetQueryOnly(t.conn)
	if t.onDone != nil {
		t.onDone()
	}
	return t.h.Classify(op, err)
}

// ExecStmtHook lets a test make a statement that ends a read-only transaction
// (COMMIT or ROLLBACK) fail. It is for tests only and is nil otherwise. It is a
// plain package variable, read without synchronisation by every read-only
// transaction in the process, so a test that sets it must not run in parallel
// with any other test that uses a SQLite driver, and must reset it to nil.
var ExecStmtHook func(stmt string) error

func (t *roTx) run(ctx context.Context, stmt string) error {
	if h := ExecStmtHook; h != nil {
		if err := h(stmt); err != nil {
			return err
		}
	}
	_, err := t.conn.ExecContext(ctx, stmt)
	return err
}
