// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/azrtydxb/momo"
)

// RunEnv is what RunMigration needs: the pinned connection every statement and
// history write runs on, the dialect, and the clock.
type RunEnv struct {
	Conn    momo.PinnedConn
	Dialect Dialect
	Clock   func() time.Time
	// HistoryTable is the SQL reference of the history table, as returned by
	// HistoryLocator.HistoryTable. Empty means RunMigration resolves it from
	// Conn before it runs the first statement.
	HistoryTable string
	// Reconnect, when set, returns a fresh pinned connection (the caller
	// releases it through the returned value). It is used only to record a
	// failure when Conn is no longer usable, which is what a cancelled
	// context leaves behind.
	Reconnect func(ctx context.Context) (momo.PinnedConn, error)
}

// execer is the part of a connection or transaction the runner writes through.
type execer interface {
	Exec(ctx context.Context, sql string, args []any) (int64, error)
}

// RunMigration applies one migration and records it in the history table.
//
// A transactional migration runs, in one transaction, the removal of an old
// rolled_back row, a running row, the statements and the update to applied; any
// error rolls everything back and a rolled_back row carrying the error is then
// written in autocommit (rolled_back is not dirty). A migration with a NoTx
// directive or statement runs each statement autocommitted and records
// statements_applied after each one; an error leaves the row failed, which is
// dirty until resolved.
func RunMigration(ctx context.Context, env RunEnv, p Pending, applyID string) (Applied, error) {
	a, err := runMigration(ctx, env, p, applyID)
	// The history row is redacted; so is the message of the returned error.
	return a, redactErr(err)
}

func runMigration(ctx context.Context, env RunEnv, p Pending, applyID string) (Applied, error) {
	tbl := env.HistoryTable
	if tbl == "" {
		var err error
		if tbl, err = resolveHistoryTable(ctx, env.Conn, env.Dialect); err != nil {
			return Applied{}, fmt.Errorf("migrate: %s_%s: %w", p.Version, p.Name, err)
		}
	}
	h := historyWriter{d: env.Dialect, tbl: tbl}
	start := env.Clock()
	kind := string(p.Kind)
	if kind == "" {
		kind = "sql"
	}
	rec := HistoryRow{Version: p.Version, Name: p.Name, Checksum: p.Checksum, ApplyID: applyID, Kind: kind, AppliedAt: start}
	if p.NoTx || hasNoTx(p.Statements) {
		return runNonTransactional(ctx, env, h, rec, p, start)
	}
	return runTransactional(ctx, env, h, rec, p, start)
}

func hasNoTx(stmts []Statement) bool {
	for _, s := range stmts {
		if s.NoTx {
			return true
		}
	}
	return false
}

func elapsed(env RunEnv, start time.Time) (time.Duration, int64) {
	d := env.Clock().Sub(start)
	return d, d.Milliseconds()
}

func runTransactional(ctx context.Context, env RunEnv, h historyWriter, rec HistoryRow, p Pending, start time.Time) (Applied, error) {
	tx, err := env.Conn.Begin(ctx, momo.TxOptions{})
	if err != nil {
		return Applied{}, fmt.Errorf("migrate: %s: begin: %w", p.Version, err)
	}
	failed := func(cause error) (Applied, error) {
		cause = redact(cause)
		// A cancelled caller context must not stop the rollback or the
		// failure record, so both run on a context that outlives it.
		fctx, cancel := failureContext(ctx)
		defer cancel()
		if rbErr := tx.Rollback(fctx); rbErr != nil {
			cause = errors.Join(cause, fmt.Errorf("rollback: %w", rbErr))
		}
		rec.Status, rec.Error = StatusRolledBack, RedactURLs(cause.Error())
		if err := env.recordOnFailure(fctx, func(x execer) error { return h.upsert(fctx, x, rec) }); err != nil {
			cause = errors.Join(cause, fmt.Errorf("record rolled_back: %w", err))
		}
		return Applied{}, fmt.Errorf("migrate: %s_%s: %w", p.Version, p.Name, cause)
	}
	if err := h.deleteRolledBack(ctx, tx, p.Version); err != nil {
		return failed(err)
	}
	rec.Status = StatusRunning
	if err := h.insert(ctx, tx, rec); err != nil {
		return failed(err)
	}
	n, err := execAll(ctx, tx, p.Statements, nil)
	if err != nil {
		return failed(err)
	}
	d, ms := elapsed(env, start)
	if err := h.finish(ctx, tx, p.Version, StatusApplied, n, "", ms); err != nil {
		return failed(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return failed(fmt.Errorf("commit: %w", err))
	}
	return Applied{Version: p.Version, Statements: n, Duration: d}, nil
}

func runNonTransactional(ctx context.Context, env RunEnv, h historyWriter, rec HistoryRow, p Pending, start time.Time) (Applied, error) {
	if err := h.deleteRolledBack(ctx, env.Conn, p.Version); err != nil {
		return Applied{}, fmt.Errorf("migrate: %s_%s: %w", p.Version, p.Name, err)
	}
	rec.Status = StatusRunning
	if err := h.insert(ctx, env.Conn, rec); err != nil {
		return Applied{}, fmt.Errorf("migrate: %s_%s: %w", p.Version, p.Name, err)
	}
	n := 0
	for _, s := range p.Statements {
		if _, err := env.Conn.Exec(ctx, s.SQL, nil); err != nil {
			cause := fmt.Errorf("statement %d: %w", n+1, redact(err))
			_, ms := elapsed(env, start)
			// The caller's context may be the reason for the failure, so the
			// failure record is written on a context that outlives it.
			fctx, cancel := failureContext(ctx)
			defer cancel()
			if herr := env.recordOnFailure(fctx, func(x execer) error {
				return h.finish(fctx, x, p.Version, StatusFailed, n, RedactURLs(cause.Error()), ms)
			}); herr != nil {
				cause = errors.Join(cause, fmt.Errorf("record failed: %w", herr))
			}
			return Applied{Statements: n}, fmt.Errorf("migrate: %s_%s: %w", p.Version, p.Name, cause)
		}
		n++
		if err := h.progress(ctx, env.Conn, p.Version, n); err != nil {
			return Applied{Statements: n}, fmt.Errorf("migrate: %s_%s: %w", p.Version, p.Name, err)
		}
	}
	d, ms := elapsed(env, start)
	if err := h.finish(ctx, env.Conn, p.Version, StatusApplied, n, "", ms); err != nil {
		return Applied{Statements: n}, fmt.Errorf("migrate: %s_%s: %w", p.Version, p.Name, err)
	}
	return Applied{Version: p.Version, Statements: n, Duration: d}, nil
}

// execAll runs stmts in order and returns how many succeeded. after is called
// after each success when it is not nil.
func execAll(ctx context.Context, x execer, stmts []Statement, after func(n int) error) (int, error) {
	n := 0
	for _, s := range stmts {
		if _, err := x.Exec(ctx, s.SQL, nil); err != nil {
			return n, fmt.Errorf("statement %d: %w", n+1, err)
		}
		n++
		if after != nil {
			if err := after(n); err != nil {
				return n, err
			}
		}
	}
	return n, nil
}

// RunStatements runs stmts in one transaction on c and rolls it back on the
// first error. The statement hints fk-check and dependent:<type>:<name> are
// SQLite rebuild hints and do nothing on PostgreSQL. A statement marked NoTx
// cannot run inside a transaction and is refused.
func RunStatements(ctx context.Context, c momo.PinnedConn, stmts []Statement) error {
	if hasNoTx(stmts) {
		return errors.New("migrate: a NoTx statement cannot run inside a transaction")
	}
	tx, err := c.Begin(ctx, momo.TxOptions{})
	if err != nil {
		return fmt.Errorf("migrate: begin: %w", err)
	}
	if _, err := execAll(ctx, tx, stmts, nil); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			err = errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
		}
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("migrate: commit: %w", err)
	}
	return nil
}

// historyWriter writes history rows with the dialect's placeholders.
type historyWriter struct {
	d   Dialect
	tbl string // SQL reference of the history table
}

func (h historyWriter) deleteRolledBack(ctx context.Context, x execer, version string) error {
	_, err := x.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE version = %s AND status = 'rolled_back'`, h.tbl, ph(h.d, 1)), []any{version})
	if err != nil {
		return fmt.Errorf("clear rolled_back row: %w", err)
	}
	return nil
}

func (h historyWriter) insert(ctx context.Context, x execer, r HistoryRow) error {
	_, err := x.Exec(ctx, fmt.Sprintf(`INSERT INTO %s (version, name, checksum, status, statements_applied, error, applied_at, execution_ms, apply_id, kind)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)`, h.tbl, ph(h.d, 1), ph(h.d, 2), ph(h.d, 3), ph(h.d, 4), ph(h.d, 5), ph(h.d, 6), ph(h.d, 7), ph(h.d, 8), ph(h.d, 9), ph(h.d, 10)),
		h.args(r))
	if err != nil {
		return fmt.Errorf("record %s row: %w", r.Status, err)
	}
	return nil
}

// upsert records r, replacing a row for the same version unless that row is
// applied: a failure path must never turn another engine's applied row back
// into a pending version.
func (h historyWriter) upsert(ctx context.Context, x execer, r HistoryRow) error {
	_, err := x.Exec(ctx, fmt.Sprintf(`INSERT INTO %s AS h (version, name, checksum, status, statements_applied, error, applied_at, execution_ms, apply_id, kind)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
ON CONFLICT (version) DO UPDATE SET name = excluded.name, checksum = excluded.checksum, status = excluded.status,
 statements_applied = excluded.statements_applied, error = excluded.error, applied_at = excluded.applied_at,
 execution_ms = excluded.execution_ms, apply_id = excluded.apply_id, kind = excluded.kind
WHERE h.status IN ('rolled_back', 'failed', 'running')`,
		h.tbl, ph(h.d, 1), ph(h.d, 2), ph(h.d, 3), ph(h.d, 4), ph(h.d, 5), ph(h.d, 6), ph(h.d, 7), ph(h.d, 8), ph(h.d, 9), ph(h.d, 10)),
		h.args(r))
	if err != nil {
		return fmt.Errorf("record %s row: %w", r.Status, err)
	}
	return nil
}

func (historyWriter) args(r HistoryRow) []any {
	return []any{r.Version, r.Name, r.Checksum, r.Status, r.StatementsApplied, r.Error, r.AppliedAt.UTC().Format(timeLayout), r.ExecutionMS, r.ApplyID, r.Kind}
}

func (h historyWriter) progress(ctx context.Context, x execer, version string, n int) error {
	_, err := x.Exec(ctx, fmt.Sprintf(`UPDATE %s SET statements_applied = %s WHERE version = %s`, h.tbl, ph(h.d, 1), ph(h.d, 2)), []any{n, version})
	if err != nil {
		return fmt.Errorf("record progress: %w", err)
	}
	return nil
}

func (h historyWriter) finish(ctx context.Context, x execer, version, status string, n int, errText string, ms int64) error {
	_, err := x.Exec(ctx, fmt.Sprintf(`UPDATE %s SET status = %s, statements_applied = %s, error = %s, execution_ms = %s WHERE version = %s`,
		h.tbl, ph(h.d, 1), ph(h.d, 2), ph(h.d, 3), ph(h.d, 4), ph(h.d, 5)), []any{status, n, errText, ms, version})
	if err != nil {
		return fmt.Errorf("record %s row: %w", status, err)
	}
	return nil
}

// failureTimeout bounds the rollback and failure write after the caller's
// context is gone.
const failureTimeout = 5 * time.Second

func failureContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), failureTimeout)
}

// redact removes the data from errors of SQLSTATE class 22 (data exception):
// their messages quote the offending value, and the history table and logs
// must not keep it. The code is kept so the failure stays diagnosable.
func redact(err error) error {
	var s interface{ SQLState() string }
	if errors.As(err, &s) && strings.HasPrefix(s.SQLState(), "22") {
		return fmt.Errorf("data exception (SQLSTATE %s); the message is withheld because it may quote data", s.SQLState())
	}
	return err
}

// recordOnFailure runs write on Conn and, when that fails and Reconnect is set,
// once more on a fresh connection.
func (env RunEnv) recordOnFailure(ctx context.Context, write func(execer) error) error {
	err := write(env.Conn)
	if err == nil || env.Reconnect == nil {
		return err
	}
	c, cerr := env.Reconnect(ctx)
	if cerr != nil {
		return errors.Join(err, cerr)
	}
	defer func() { _ = c.Release() }()
	if err2 := write(c); err2 != nil {
		return errors.Join(err, err2)
	}
	return nil
}
