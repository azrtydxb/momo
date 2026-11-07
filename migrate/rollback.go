// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/azrtydxb/momo"
)

// RollbackOptions controls Rollback.
type RollbackOptions struct {
	// Group rolls back every migration of the last Apply (the applied rows
	// that share the greatest apply_id) instead of the last migration only.
	Group bool
	// LockWait bounds the wait for the migration lock; zero uses the engine
	// default, as for ApplyOptions.
	LockWait time.Duration
}

// Rollback runs the authored down migrations of the last applied migration, or
// with Group of the whole last applied group, in descending version order.
//
// It selects the applied row with the greatest applied_at (Group: all applied
// rows with the greatest apply_id), checks that every selected version has a
// .down.sql file before anything runs, and returns ErrNoDown naming the first
// version without one. A transactional down runs with its history update in
// one transaction; the row becomes rolled_back, which makes the version
// pending again. A down with the no-transaction directive runs statement by
// statement and, if one fails, leaves the row failed (dirty until resolved).
// Like Apply it takes the migration lock, applies the session timeouts, and
// refuses with ErrDirty or ErrChecksumMismatch. Nothing applied returns an
// empty report.
func (e *Engine) Rollback(ctx context.Context, o RollbackOptions) (rep *Report, err error) {
	c, err := e.pin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Release() }()
	end, err := e.session(ctx, c, e.lockWait(ApplyOptions{LockWait: o.LockWait}))
	if err != nil {
		return nil, err
	}
	defer func() { err = end(err) }()

	tbl, err := resolveHistoryTable(ctx, c, e.opts.dialect)
	if err != nil {
		return nil, err
	}
	hist, err := e.readHistoryAt(ctx, c, tbl)
	if err != nil {
		return nil, err
	}
	if err := checkHistoryFormat(ctx, c, tbl); err != nil {
		return nil, err
	}
	if err := dirtyError(hist); err != nil {
		return nil, err
	}
	if err := e.verifyChecksums(hist); err != nil {
		return nil, err
	}
	selected := selectRollback(hist, o.Group)
	if len(selected) == 0 {
		return &Report{}, nil
	}
	downs, err := e.loadDowns(selected)
	if err != nil {
		return nil, err
	}
	env := RunEnv{Conn: c, Dialect: e.opts.dialect, Clock: e.opts.clock, Reconnect: e.pin, HistoryTable: tbl}
	rep = &Report{}
	for i, r := range selected {
		a, err := runRollback(ctx, env, r, downs[i])
		if err != nil {
			return rep, redactErr(e.classify(err))
		}
		rep.Applied = append(rep.Applied, a)
	}
	return rep, nil
}

// selectRollback returns the applied rows to roll back, in descending version
// order.
func selectRollback(hist []HistoryRow, group bool) []HistoryRow {
	var applied []HistoryRow
	for _, r := range hist {
		if r.Status == StatusApplied {
			applied = append(applied, r)
		}
	}
	if len(applied) == 0 {
		return nil
	}
	last := applied[0]
	for _, r := range applied[1:] {
		if r.AppliedAt.After(last.AppliedAt) || r.AppliedAt.Equal(last.AppliedAt) && r.Version > last.Version {
			last = r
		}
	}
	out := []HistoryRow{last}
	if group && last.ApplyID != "" {
		out = out[:0]
		for _, r := range applied {
			if r.ApplyID == last.ApplyID {
				out = append(out, r)
			}
		}
	}
	slices.SortFunc(out, func(a, b HistoryRow) int { return strings.Compare(b.Version, a.Version) })
	return out
}

// loadDowns reads the down file of every selected row, in the same order. It
// returns ErrNoDown for the first version that has none.
func (e *Engine) loadDowns(rows []HistoryRow) ([]Pending, error) {
	files, err := Discover(e.dir)
	if err != nil {
		return nil, err
	}
	byVersion := map[string]File{}
	for _, f := range files {
		if f.Kind == "down" {
			byVersion[f.Version] = f
		}
	}
	name := string(e.opts.dialect.Dialect().Name)
	out := make([]Pending, len(rows))
	for i, r := range rows {
		f, ok := byVersion[r.Version]
		if !ok {
			return nil, fmt.Errorf("%w: version %s (%s)", ErrNoDown, r.Version, r.Name)
		}
		dirs, err := ParseDirectives(f.Body)
		if err != nil {
			return nil, fmt.Errorf("migrate: %s: %w", f.Path, err)
		}
		parts, err := SplitStatements(f.Body, name)
		if err != nil {
			return nil, fmt.Errorf("migrate: %s: %w", f.Path, err)
		}
		stmts := make([]Statement, len(parts))
		for j, s := range parts {
			stmts[j] = Statement{SQL: s}
		}
		out[i] = Pending{Version: r.Version, Name: r.Name, Kind: "sql", Statements: stmts, NoTx: dirs.NoTransaction, Checksum: r.Checksum}
	}
	return out, nil
}

// runRollback runs one down migration and marks the history row rolled_back.
func runRollback(ctx context.Context, env RunEnv, r HistoryRow, p Pending) (Applied, error) {
	tbl := env.HistoryTable
	if tbl == "" {
		var err error
		if tbl, err = resolveHistoryTable(ctx, env.Conn, env.Dialect); err != nil {
			return Applied{}, fmt.Errorf("migrate: %s_%s: %w", p.Version, p.Name, err)
		}
	}
	h := historyWriter{d: env.Dialect, tbl: tbl}
	start := env.Clock()
	wrap := func(err error) error {
		return fmt.Errorf("migrate: down %s_%s: %w", p.Version, p.Name, err)
	}
	if p.NoTx || hasNoTx(p.Statements) {
		n, err := execAll(ctx, env.Conn, p.Statements, nil)
		if err != nil {
			cause := redact(err)
			fctx, cancel := failureContext(ctx)
			defer cancel()
			_, ms := elapsed(env, start)
			if herr := env.recordOnFailure(fctx, func(x execer) error {
				return h.finish(fctx, x, p.Version, StatusFailed, n, RedactURLs("down: "+cause.Error()), ms)
			}); herr != nil {
				cause = errors.Join(cause, fmt.Errorf("record failed: %w", herr))
			}
			return Applied{Statements: n}, wrap(cause)
		}
		d, ms := elapsed(env, start)
		if err := h.finish(ctx, env.Conn, p.Version, StatusRolledBack, 0, "", ms); err != nil {
			return Applied{Statements: n}, wrap(err)
		}
		return Applied{Version: p.Version, Statements: n, Duration: d}, nil
	}
	tx, err := env.Conn.Begin(ctx, momo.TxOptions{})
	if err != nil {
		return Applied{}, wrap(fmt.Errorf("begin: %w", err))
	}
	failed := func(cause error) (Applied, error) {
		cause = redact(cause)
		fctx, cancel := failureContext(ctx)
		defer cancel()
		if rbErr := tx.Rollback(fctx); rbErr != nil {
			cause = errors.Join(cause, fmt.Errorf("rollback: %w", rbErr))
		}
		return Applied{}, wrap(cause)
	}
	n, err := execAll(ctx, tx, p.Statements, nil)
	if err != nil {
		return failed(err)
	}
	d, ms := elapsed(env, start)
	if err := h.finish(ctx, tx, p.Version, StatusRolledBack, 0, "", ms); err != nil {
		return failed(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return failed(fmt.Errorf("commit: %w", err))
	}
	return Applied{Version: p.Version, Statements: n, Duration: d}, nil
}
