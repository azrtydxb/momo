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
	if _, err := conn.ExecContext(ctx, "PRAGMA query_only=ON"); err != nil {
		return nil, h.Classify("begin", err)
	}
	if _, err := conn.ExecContext(ctx, "BEGIN DEFERRED"); err != nil {
		resetQueryOnly(conn)
		return nil, h.Classify("begin", err)
	}
	return &roTx{conn: conn, h: h}, nil
}

// roTx is a read-only transaction driven with BEGIN and COMMIT statements on
// one *sql.Conn.
type roTx struct {
	conn   *sql.Conn
	h      *Hooks
	mu     sync.Mutex
	done   bool
	onDone func() // called once, after the transaction ended; may be nil
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
	return sqlRows{r: rows, h: t.h}, nil
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
	t.mu.Unlock()
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
// (COMMIT or ROLLBACK) fail. It is for tests only and is nil otherwise.
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
