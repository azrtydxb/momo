// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
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

// history creates the history table when it is missing and reads it.
func (e *Engine) history(ctx context.Context, c momo.PinnedConn) ([]HistoryRow, error) {
	if err := EnsureHistory(ctx, c, e.opts.dialect); err != nil {
		return nil, err
	}
	return ReadHistory(ctx, c)
}

// dirtyError returns ErrDirty naming the first running or failed row.
func dirtyError(hist []HistoryRow) error {
	for _, r := range hist {
		if r.dirty() {
			return fmt.Errorf("%w: version %s is %s", ErrDirty, r.Version, r.Status)
		}
	}
	return nil
}

// pendingFrom lists the migrations without an applied history row, in version
// order. A rolled_back row does not count as done.
func (e *Engine) pendingFrom(hist []HistoryRow) ([]Pending, error) {
	done := map[string]bool{}
	for _, r := range hist {
		if r.Status != StatusRolledBack {
			done[r.Version] = true
		}
	}
	return e.loadPending(done)
}

// sumMismatches returns the names of files whose content differs from
// momo.sum. A directory without momo.sum has nothing to compare and returns nil.
func (e *Engine) sumMismatches() ([]string, error) {
	f, err := e.dir.Open(SumFileName)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("migrate: open %s: %w", SumFileName, err)
	}
	defer func() { _ = f.Close() }()
	want, err := ParseSum(f)
	if err != nil {
		return nil, err
	}
	return VerifySum(e.dir, want), nil
}

// verifyChecksums refuses with ErrChecksumMismatch, naming the file, when a
// file differs from momo.sum or from the checksum recorded when its migration
// was applied.
func (e *Engine) verifyChecksums(hist []HistoryRow) error {
	bad, err := e.sumMismatches()
	if err != nil {
		return err
	}
	if len(bad) > 0 {
		return fmt.Errorf("%w: %s differ from %s", ErrChecksumMismatch, strings.Join(bad, ", "), SumFileName)
	}
	files, err := Discover(e.dir)
	if err != nil {
		return err
	}
	sum, err := ComputeSum(e.dir)
	if err != nil {
		return err
	}
	byVersion := map[string]File{}
	for _, f := range files {
		if f.Kind == "up" {
			byVersion[f.Version] = f
		}
	}
	for _, r := range hist {
		f, ok := byVersion[r.Version]
		if r.Status == StatusApplied && ok && sum.Files[f.Path] != r.Checksum {
			return fmt.Errorf("%w: %s changed after version %s was applied", ErrChecksumMismatch, f.Path, r.Version)
		}
	}
	return nil
}

// Plan lists the migrations that Apply would run. It takes no lock and does
// not check checksums or dirty state; Status reports those.
func (e *Engine) Plan(ctx context.Context) ([]Pending, error) {
	c, err := e.pin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Release() }()
	hist, err := e.history(ctx, c)
	if err != nil {
		return nil, err
	}
	return e.pendingFrom(hist)
}

// resolveSchemas returns the managed schemas: Config.Schemas, or the
// connection's current_schema() when none are configured. On PostgreSQL the
// first one must be current_schema(), because the history table is referenced
// unqualified and lives there.
func (e *Engine) resolveSchemas(ctx context.Context, c momo.PinnedConn) ([]string, error) {
	if e.opts.dialect.Dialect().Name != dialect.Postgres {
		return e.opts.config.Schemas, nil
	}
	rows, err := c.Query(ctx, `SELECT COALESCE(current_schema(), '')`, nil)
	if err != nil {
		return nil, fmt.Errorf("migrate: read current_schema(): %w", err)
	}
	defer func() { _ = rows.Close() }()
	var cur string
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("migrate: read current_schema(): %w", err)
		}
		return nil, errors.New("migrate: current_schema() returned no row")
	}
	if err := rows.Scan(&cur); err != nil {
		return nil, fmt.Errorf("migrate: read current_schema(): %w", err)
	}
	if cur == "" {
		return nil, errors.New("migrate: the connection has no current schema; set search_path to the schema that holds momo_migrations")
	}
	schemas := e.opts.config.Schemas
	if len(schemas) == 0 {
		return []string{cur}, nil
	}
	if schemas[0] != cur {
		return nil, fmt.Errorf("migrate: configuration error: Config.Schemas[0] is %q but the connection's current_schema() is %q; the history table is created unqualified in the current schema, so set search_path to %q or list %q first", schemas[0], cur, schemas[0], cur)
	}
	return schemas, nil
}

// lockPoll is how often Apply retries a held migration lock.
const lockPoll = 200 * time.Millisecond

// acquireLock takes the migration lock, retrying every lockPoll until wait has
// passed. On timeout it returns the dialect's ErrLocked error, which names the
// holder.
func (e *Engine) acquireLock(ctx context.Context, c momo.PinnedConn, key string, wait time.Duration) (func() error, error) {
	deadline := time.Now().Add(wait)
	for {
		unlock, _, err := e.opts.dialect.Lock(ctx, c, key)
		if err == nil {
			return unlock, nil
		}
		left := time.Until(deadline)
		if !errors.Is(err, ErrLocked) || left <= 0 {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, errors.Join(err, ctx.Err())
		case <-time.After(min(lockPoll, left)):
		}
	}
}

func (e *Engine) lockWait(o ApplyOptions) time.Duration {
	switch {
	case o.LockWait > 0:
		return o.LockWait
	case e.opts.lockWaitSet:
		return e.opts.lockWait
	default:
		return defaultLockWait
	}
}

// checkTo refuses a To version that has no up file, so a typo does not
// silently apply nothing.
func (e *Engine) checkTo(to string) error {
	if to == "" {
		return nil
	}
	files, err := Discover(e.dir)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.Kind == "up" && f.Version == to {
			return nil
		}
	}
	return fmt.Errorf("migrate: --to %s: no migration with that version in the directory", to)
}

// joinErr joins cleanup into err without wrapping err when cleanup is nil.
func joinErr(err, cleanup error) error {
	if cleanup == nil {
		return err
	}
	return errors.Join(err, cleanup)
}

// Apply runs the pending migrations in version order, each with its own
// history row, and stops at the first failure.
//
// It takes the migration lock (waiting up to ApplyOptions.LockWait, then
// WithLockWait, then 30s, and returning ErrLocked naming the holder), applies
// the session timeouts, creates the history table, refuses with ErrDirty while
// a migration is running or failed and with ErrChecksumMismatch when an applied
// or listed file was edited, and then runs the pending migrations up to
// ApplyOptions.To. The session timeouts are reset and the lock released on every
// path, in that order, detached from ctx; a failure of either is joined to the
// returned error.
func (e *Engine) Apply(ctx context.Context, o ApplyOptions) (rep *Report, err error) {
	if err := e.checkTo(o.To); err != nil {
		return nil, err
	}
	c, err := e.pin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Release() }()
	schemas, err := e.resolveSchemas(ctx, c)
	if err != nil {
		return nil, err
	}
	hs := ""
	if len(schemas) > 0 {
		hs = schemas[0]
	}
	unlock, err := e.acquireLock(ctx, c, "momo:"+hs+".momo_migrations", e.lockWait(o))
	if err != nil {
		return nil, err
	}
	defer func() { err = joinErr(err, unlock()) }() // registered first, runs second
	reset, err := e.opts.dialect.Setup(ctx, c, SessionOptions{LockTimeout: e.opts.config.LockTimeout, StatementTimeout: e.opts.config.StatementTimeout})
	if err != nil {
		return nil, err
	}
	defer func() { err = joinErr(err, reset()) }() // registered second, runs first

	// The history table is created under the lock, so two first-time callers
	// cannot race on CREATE TABLE.
	hist, err := e.history(ctx, c)
	if err != nil {
		return nil, err
	}
	if err := dirtyError(hist); err != nil {
		return nil, err
	}
	if err := e.verifyChecksums(hist); err != nil {
		return nil, err
	}
	pending, err := e.pendingFrom(hist)
	if err != nil {
		return nil, err
	}
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, fmt.Errorf("migrate: apply id: %w", err)
	}
	rep = &Report{ApplyID: e.opts.clock().UTC().Format("20060102150405") + "-" + hex.EncodeToString(b[:])}
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

// Status reports the state of every migration version: applied, pending, dirty
// (running or failed), mismatch (the file differs from the recorded checksum or
// from momo.sum) or missing (a history row without a file). It takes no lock.
func (e *Engine) Status(ctx context.Context) ([]VersionStatus, error) {
	c, err := e.pin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Release() }()
	hist, err := e.history(ctx, c)
	if err != nil {
		return nil, err
	}
	files, err := Discover(e.dir)
	if err != nil {
		return nil, err
	}
	sum, err := ComputeSum(e.dir)
	if err != nil {
		return nil, err
	}
	badNames, err := e.sumMismatches()
	if err != nil {
		return nil, err
	}
	bad := map[string]bool{}
	for _, n := range badNames {
		bad[n] = true
	}
	rows := map[string]HistoryRow{}
	for _, r := range hist {
		rows[r.Version] = r
	}
	var out []VersionStatus
	seen := map[string]bool{}
	for _, f := range files {
		if f.Kind != "up" {
			continue
		}
		seen[f.Version] = true
		fileSum := sum.Files[f.Path]
		st := VersionStatus{Version: f.Version, Name: f.Name, State: "pending", Checksum: fileSum}
		if r, ok := rows[f.Version]; ok {
			st.Checksum, st.Error = r.Checksum, r.Error
			switch {
			case r.dirty():
				st.State = "dirty"
			case r.Status == StatusApplied && r.Checksum == fileSum && !bad[f.Path]:
				st.State = "applied"
			case r.Status == StatusApplied:
				st.State = "mismatch"
			}
		}
		if st.State == "pending" && bad[f.Path] {
			st.State = "mismatch"
		}
		out = append(out, st)
	}
	for _, r := range hist {
		if seen[r.Version] || r.Status == StatusRolledBack {
			continue
		}
		st := VersionStatus{Version: r.Version, Name: r.Name, State: "missing", Checksum: r.Checksum, Error: r.Error}
		if r.dirty() {
			st.State = "dirty"
		}
		out = append(out, st)
	}
	slices.SortFunc(out, func(a, b VersionStatus) int { return strings.Compare(a.Version, b.Version) })
	return out, nil
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
