// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/azrtydxb/momo"
)

// RunEnv is what RunMigration needs: the pinned connection every statement and
// history write runs on, the dialect, and the clock.
type RunEnv struct {
	Conn    momo.PinnedConn
	Dialect Dialect
	Clock   func() time.Time
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
	h := historyWriter{d: env.Dialect}
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
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			cause = errors.Join(cause, fmt.Errorf("rollback: %w", rbErr))
		}
		rec.Status, rec.Error = StatusRolledBack, cause.Error()
		if err := h.upsert(ctx, env.Conn, rec); err != nil {
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
			cause := fmt.Errorf("statement %d: %w", n+1, err)
			_, ms := elapsed(env, start)
			// The caller's context may be the reason for the failure, so the
			// failure record is written on a context that outlives it.
			if herr := h.finish(context.WithoutCancel(ctx), env.Conn, p.Version, StatusFailed, n, cause.Error(), ms); herr != nil {
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
type historyWriter struct{ d Dialect }

func (h historyWriter) deleteRolledBack(ctx context.Context, x execer, version string) error {
	_, err := x.Exec(ctx, fmt.Sprintf(`DELETE FROM momo_migrations WHERE version = %s AND status = 'rolled_back'`, ph(h.d, 1)), []any{version})
	if err != nil {
		return fmt.Errorf("clear rolled_back row: %w", err)
	}
	return nil
}

func (h historyWriter) insert(ctx context.Context, x execer, r HistoryRow) error {
	_, err := x.Exec(ctx, fmt.Sprintf(`INSERT INTO momo_migrations (version, name, checksum, status, statements_applied, error, applied_at, execution_ms, apply_id, kind)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)`, ph(h.d, 1), ph(h.d, 2), ph(h.d, 3), ph(h.d, 4), ph(h.d, 5), ph(h.d, 6), ph(h.d, 7), ph(h.d, 8), ph(h.d, 9), ph(h.d, 10)),
		h.args(r))
	if err != nil {
		return fmt.Errorf("record %s row: %w", r.Status, err)
	}
	return nil
}

// upsert records r, replacing a row for the same version.
func (h historyWriter) upsert(ctx context.Context, x execer, r HistoryRow) error {
	_, err := x.Exec(ctx, fmt.Sprintf(`INSERT INTO momo_migrations (version, name, checksum, status, statements_applied, error, applied_at, execution_ms, apply_id, kind)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
ON CONFLICT (version) DO UPDATE SET name = excluded.name, checksum = excluded.checksum, status = excluded.status,
 statements_applied = excluded.statements_applied, error = excluded.error, applied_at = excluded.applied_at,
 execution_ms = excluded.execution_ms, apply_id = excluded.apply_id, kind = excluded.kind`,
		ph(h.d, 1), ph(h.d, 2), ph(h.d, 3), ph(h.d, 4), ph(h.d, 5), ph(h.d, 6), ph(h.d, 7), ph(h.d, 8), ph(h.d, 9), ph(h.d, 10)),
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
	_, err := x.Exec(ctx, fmt.Sprintf(`UPDATE momo_migrations SET statements_applied = %s WHERE version = %s`, ph(h.d, 1), ph(h.d, 2)), []any{n, version})
	if err != nil {
		return fmt.Errorf("record progress: %w", err)
	}
	return nil
}

func (h historyWriter) finish(ctx context.Context, x execer, version, status string, n int, errText string, ms int64) error {
	_, err := x.Exec(ctx, fmt.Sprintf(`UPDATE momo_migrations SET status = %s, statements_applied = %s, error = %s, execution_ms = %s WHERE version = %s`,
		ph(h.d, 1), ph(h.d, 2), ph(h.d, 3), ph(h.d, 4), ph(h.d, 5)), []any{status, n, errText, ms, version})
	if err != nil {
		return fmt.Errorf("record %s row: %w", status, err)
	}
	return nil
}
