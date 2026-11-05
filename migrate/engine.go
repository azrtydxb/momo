// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/azrtydxb/momo"
)

// Engine plans and applies the migrations of one directory on one database.
type Engine struct {
	drv  momo.Driver
	dir  fs.FS
	opts options
}

// NewEngine returns an engine for the migration directory dir. The option
// WithDialect is required.
func NewEngine(drv momo.Driver, dir fs.FS, opts ...Option) (*Engine, error) {
	o := options{clock: time.Now, config: Config{LockTimeout: defaultLockTimeout}}
	for _, opt := range opts {
		opt(&o)
	}
	if o.dialect == nil {
		return nil, errors.New("migrate: NewEngine requires migrate.WithDialect(...)")
	}
	return &Engine{drv: drv, dir: dir, opts: o}, nil
}

// pin checks a connection out of the driver.
func (e *Engine) pin(ctx context.Context) (momo.PinnedConn, error) {
	p, ok := e.drv.(momo.Pinner)
	if !ok {
		return nil, errors.New("migrate: the driver cannot pin a connection (it does not implement momo.Pinner)")
	}
	c, err := p.Pin(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate: pin connection: %w", err)
	}
	return c, nil
}

// pendingOn lists the migrations without an applied history row, in version
// order, and refuses with ErrDirty when requireClean is set and a dirty row exists.
func (e *Engine) pendingOn(ctx context.Context, c momo.PinnedConn, requireClean bool) ([]Pending, error) {
	if err := EnsureHistory(ctx, c, e.opts.dialect); err != nil {
		return nil, err
	}
	hist, err := ReadHistory(ctx, c)
	if err != nil {
		return nil, err
	}
	done := map[string]bool{}
	for _, r := range hist {
		if r.dirty() && requireClean {
			return nil, fmt.Errorf("%w: version %s is %s", ErrDirty, r.Version, r.Status)
		}
		if r.Status != StatusRolledBack {
			done[r.Version] = true
		}
	}
	return e.loadPending(done)
}

func (e *Engine) loadPending(done map[string]bool) ([]Pending, error) {
	files, err := Discover(e.dir)
	if err != nil {
		return nil, err
	}
	sum, err := ComputeSum(e.dir)
	if err != nil {
		return nil, err
	}
	name := string(e.opts.dialect.Dialect().Name)
	var out []Pending
	for _, f := range files {
		if f.Kind != "up" || done[f.Version] {
			continue
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
		for i, s := range parts {
			stmts[i] = Statement{SQL: s}
		}
		out = append(out, Pending{Version: f.Version, Name: f.Name, Kind: "sql", Statements: stmts, NoTx: dirs.NoTransaction, Checksum: sum.Files[f.Path]})
	}
	return out, nil
}

// Plan lists the migrations that Apply would run.
func (e *Engine) Plan(ctx context.Context) ([]Pending, error) {
	c, err := e.pin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Release() }()
	return e.pendingOn(ctx, c, false)
}

// Apply runs the pending migrations in version order, each with its own
// history row, and stops at the first failure. It refuses with ErrDirty while
// a migration is running or failed.
func (e *Engine) Apply(ctx context.Context, o ApplyOptions) (*Report, error) {
	c, err := e.pin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Release() }()
	pending, err := e.pendingOn(ctx, c, true)
	if err != nil {
		return nil, err
	}
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, fmt.Errorf("migrate: apply id: %w", err)
	}
	rep := &Report{ApplyID: e.opts.clock().UTC().Format("20060102150405") + "-" + hex.EncodeToString(b[:])}
	env := RunEnv{Conn: c, Dialect: e.opts.dialect, Clock: e.opts.clock, Reconnect: e.pin}
	for _, p := range pending {
		if o.To != "" && p.Version > o.To {
			break
		}
		a, err := RunMigration(ctx, env, p, rep.ApplyID)
		if err != nil {
			return rep, err
		}
		rep.Applied = append(rep.Applied, a)
	}
	return rep, nil
}

// Resolve clears the dirty state of version after manual repair. With
// ResolveApplied the failed or running row becomes applied and keeps its
// statements_applied; with ResolveRolledBack the row is deleted, so the version
// is pending again. It is an error when the version has no dirty row.
func (e *Engine) Resolve(ctx context.Context, version string, state ResolveState) error {
	if state != ResolveApplied && state != ResolveRolledBack {
		return fmt.Errorf("migrate: unknown resolve state %q; use %q or %q", state, ResolveApplied, ResolveRolledBack)
	}
	c, err := e.pin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = c.Release() }()
	if err := EnsureHistory(ctx, c, e.opts.dialect); err != nil {
		return err
	}
	hist, err := ReadHistory(ctx, c)
	if err != nil {
		return err
	}
	found := false
	for _, r := range hist {
		if r.Version == version && r.dirty() {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("migrate: version %s has no running or failed history row to resolve", version)
	}
	d := e.opts.dialect
	var q string
	var args []any
	if state == ResolveApplied {
		q = fmt.Sprintf(`UPDATE momo_migrations SET status = %s, error = '' WHERE version = %s AND status IN ('running', 'failed')`, ph(d, 1), ph(d, 2))
		args = []any{StatusApplied, version}
	} else {
		q = fmt.Sprintf(`DELETE FROM momo_migrations WHERE version = %s AND status IN ('running', 'failed')`, ph(d, 1))
		args = []any{version}
	}
	if _, err := c.Exec(ctx, q, args); err != nil {
		return fmt.Errorf("migrate: resolve %s: %w", version, err)
	}
	return nil
}
