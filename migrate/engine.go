// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
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
	if o.lockTimeout != nil && *o.lockTimeout < 0 || o.statementTimeout != nil && *o.statementTimeout < 0 {
		return nil, errors.New("migrate: a lock or statement timeout cannot be negative")
	}
	if err := checkGoMigrations(o.goMigrations); err != nil {
		return nil, err
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
	if err := goCollisions(e.opts.goMigrations, files); err != nil {
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
			if s, err = applyDDLDirective(s, name); err != nil {
				return nil, fmt.Errorf("migrate: %s: statement %d: %w", f.Path, i+1, err)
			}
			stmts[i] = Statement{SQL: s}
		}
		out = append(out, Pending{Version: f.Version, Name: f.Name, Kind: "sql", Statements: stmts, NoTx: dirs.NoTransaction, Checksum: sum.Files[f.Path], ForeignKeysOff: dirs.SQLiteForeignKeysOff})
	}
	out = append(out, e.goPending(done)...)
	sortPending(out)
	return out, nil
}

// undefinedTable is the SQLSTATE of a query on a table that does not exist.
const undefinedTable = "42P01"

// undefinedTableMySQL is the SQLSTATE MySQL and MariaDB report for it.
const undefinedTableMySQL = "42S02"

// readHistory reads the history table without creating it: a database that
// has none yet has an empty history. Plan and Status use it so they stay
// read-only and cannot race the first Apply on CREATE TABLE.
func (e *Engine) readHistory(ctx context.Context, c momo.PinnedConn) ([]HistoryRow, error) {
	tbl, err := resolveHistoryTable(ctx, c, e.opts.dialect)
	if errors.Is(err, ErrNoHistoryTable) {
		return nil, nil // nothing to read, as for a database without the table
	}
	if err != nil {
		return nil, err
	}
	return e.readHistoryAt(ctx, c, tbl)
}

// readHistoryAt is readHistory for a history table reference resolved earlier.
func (e *Engine) readHistoryAt(ctx context.Context, c momo.PinnedConn, tbl string) ([]HistoryRow, error) {
	hist, err := readHistoryRows(ctx, c, tbl)
	if isUndefinedTable(err) {
		return nil, nil
	}
	return hist, err
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
// momo.sum. A directory without momo.sum is an error, because without it an
// edited migration cannot be detected; WithoutSum turns the check off.
func (e *Engine) sumMismatches() ([]string, error) {
	if e.opts.noSum {
		return nil, nil
	}
	f, err := e.dir.Open(SumFileName)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("migrate: %s is missing; run momo migrate hash to create it (it records the checksum of every migration file so edits are detected)", SumFileName)
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
	if err := goCollisions(e.opts.goMigrations, files); err != nil {
		return err
	}
	goMigs := e.goByVersion()
	for _, r := range hist {
		f, ok := byVersion[r.Version]
		if r.Status == StatusApplied && ok && sum.Files[f.Path] != r.Checksum {
			return fmt.Errorf("%w: %s changed after version %s was applied", ErrChecksumMismatch, f.Path, r.Version)
		}
		if g, ok := goMigs[r.Version]; ok && r.Status == StatusApplied && g.identity() != r.Checksum {
			return fmt.Errorf("%w: Go migration %s_%s changed (name or Checksum) after version %s was applied", ErrChecksumMismatch, g.Version, g.Name, r.Version)
		}
	}
	return nil
}

// Plan lists the migrations that Apply would run. It is read-only: it takes no
// lock, creates nothing (a database without a history table has every
// migration pending) and does not check checksums or dirty state; Status
// reports those.
func (e *Engine) Plan(ctx context.Context) ([]Pending, error) {
	c, err := e.pin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Release() }()
	hist, err := e.readHistory(ctx, c)
	if err != nil {
		return nil, err
	}
	return e.pendingFrom(hist)
}

// resolveSchemas returns the managed schemas: Config.Schemas, or the
// connection's current_schema() when none are configured. On PostgreSQL the
// first one must be current_schema(), the schema a new history table is created
// in (an existing one is found through the search_path and used wherever it is).
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
		return nil, fmt.Errorf("migrate: configuration error: Config.Schemas[0] is %q but the connection's current_schema() is %q; a new history table is created in the current schema, so set search_path to %q or list %q first", schemas[0], cur, schemas[0], cur)
	}
	return schemas, nil
}

// applyIDLayout is RFC 3339 with a fixed nine-digit fraction (RFC3339Nano drops
// trailing zeros, which breaks string ordering). Report.ApplyID is this UTC time,
// a dash and 8 random hex digits, so two applies in the same instant stay
// distinct. Group rollback picks the greatest apply_id, which therefore is the
// most recent apply; nothing parses the value.
const applyIDLayout = "2006-01-02T15:04:05.000000000Z"

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
	if _, ok := e.goByVersion()[to]; ok {
		return nil
	}
	return fmt.Errorf("migrate: --to %s: no migration with that version in the directory", to)
}

// classify lets the dialect turn a driver error into a structured one (for
// example a missing extension into momo.ErrUnsupported). It returns err when the
// dialect has no classifier or does not recognize the error.
func (e *Engine) classify(err error) error {
	if c, ok := e.opts.dialect.(momo.ErrorClassifier); ok {
		return c.ClassifyError(err)
	}
	return err
}

// joinErr joins cleanup into err without wrapping err when cleanup is nil.
func joinErr(err, cleanup error) error {
	if cleanup == nil {
		return err
	}
	return errors.Join(err, cleanup)
}

// sessionOptions merges Config with the explicit WithLockTimeout and
// WithStatementTimeout values, which win and keep a zero.
func (e *Engine) sessionOptions() SessionOptions {
	s := SessionOptions{LockTimeout: e.opts.config.LockTimeout, StatementTimeout: e.opts.config.StatementTimeout}
	if d := e.opts.lockTimeout; d != nil {
		s.LockTimeout, s.LockTimeoutSet = *d, true
	}
	if d := e.opts.statementTimeout; d != nil {
		s.StatementTimeout, s.StatementTimeoutSet = *d, true
	}
	return s
}

// session resolves the managed schemas, takes the migration lock (waiting up to
// wait) and applies the session timeouts. The returned end joins its argument
// with the outcome of the cleanup: it resets the session timeouts first and
// releases the lock second, so a small statement_timeout cannot cancel the
// unlock. Both run detached from ctx; the unlock still runs when the reset or
// the Setup fails.
func (e *Engine) session(ctx context.Context, c momo.PinnedConn, wait time.Duration) (end func(error) error, err error) {
	schemas, err := e.resolveSchemas(ctx, c)
	if err != nil {
		return nil, err
	}
	hs := ""
	if len(schemas) > 0 {
		hs = schemas[0]
	}
	unlock, err := e.lockHistory(ctx, c, hs, wait)
	if err != nil {
		return nil, err
	}
	reset, err := e.opts.dialect.Setup(ctx, c, e.sessionOptions())
	if err != nil {
		return nil, joinErr(err, unlock())
	}
	return func(err error) error {
		err = joinErr(err, reset()) // runs first
		return joinErr(err, unlock())
	}, nil
}

// lockHistory takes the migration lock, keyed on the history table the
// connection resolves to: its schema-qualified name when the table exists, the
// current schema hs when it does not yet. The table is resolved before the lock
// (a read) and again after it; if another session created or moved the table
// in between so that the name differs, the lock is released and the attempt
// repeats once with the new name, then fails.
func (e *Engine) lockHistory(ctx context.Context, c momo.PinnedConn, hs string, wait time.Duration) (func() error, error) {
	d := e.opts.dialect
	var last string
	for attempt := 0; attempt < 2; attempt++ {
		tbl, err := resolveHistoryTable(ctx, c, d)
		if err != nil {
			return nil, err
		}
		key := "momo:" + hs + ".momo_migrations"
		if k, ok := d.(historyLockKeyer); ok {
			key = k.HistoryLockKey(tbl)
		}
		unlock, err := e.acquireLock(ctx, c, key, wait)
		if err != nil {
			return nil, err
		}
		again, err := resolveHistoryTable(ctx, c, d)
		if err == nil && again == tbl {
			return unlock, nil
		}
		uerr := unlock()
		if err != nil {
			return nil, joinErr(err, uerr)
		}
		last = fmt.Sprintf("%s then %s", tbl, again)
		if uerr != nil {
			return nil, uerr
		}
	}
	return nil, fmt.Errorf("migrate: the history table moved while taking the migration lock (%s); another process is installing it, retry", last)
}

// outOfOrder refuses with ErrOutOfOrder when a pending version sorts before
// the latest applied one: running it now would apply it after newer
// migrations, against a snapshot chain that no longer matches. It happens when
// two branches each add a migration and are merged.
func outOfOrder(hist []HistoryRow, pending []Pending) error {
	latest := ""
	for _, r := range hist {
		if r.Status == StatusApplied && r.Version > latest {
			latest = r.Version
		}
	}
	for _, p := range pending {
		if p.Version < latest {
			return fmt.Errorf("%w: pending version %s is older than applied version %s; rebase it after %s", ErrOutOfOrder, p.Version, latest, latest)
		}
	}
	return nil
}

// Apply runs the pending migrations in version order, each with its own
// history row, and stops at the first failure.
//
// It takes the migration lock (waiting up to ApplyOptions.LockWait, then
// WithLockWait, then 30s, and returning ErrLocked naming the holder), applies
// the session timeouts, refuses with ErrDirty while a migration is running or
// failed, with ErrChecksumMismatch when an applied or listed file was edited,
// with ErrBranchConflict (or another chain error, as Chain reports it) when the
// snapshots do not form one linear chain, and with ErrOutOfOrder when a pending
// version is older than the latest applied one. When nothing is pending it
// returns an empty report and writes nothing, not even the history table;
// otherwise it creates the history table and runs the pending migrations up to
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
	end, err := e.session(ctx, c, e.lockWait(o))
	if err != nil {
		return nil, err
	}
	defer func() { err = end(err) }()

	// Read the history without creating it: a database that has none has an
	// empty history, and an Apply with nothing to do must write nothing.
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
	if _, err := Chain(e.dir); err != nil {
		if errors.Is(err, ErrBranchConflict) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %w", ErrChainInvalid, err)
	}
	pending, err := e.pendingFrom(hist)
	if err != nil {
		return nil, err
	}
	if err := outOfOrder(hist, pending); err != nil {
		return nil, err
	}
	if err := e.checkGoRunnable(pending, hist, o.To); err != nil {
		return nil, err
	}
	if len(pending) == 0 {
		return &Report{}, nil
	}
	// The history table is created under the lock, so two first-time callers
	// cannot race on CREATE TABLE. EnsureHistory also refuses a table written
	// with a newer format.
	if err := ensureHistory(ctx, c, e.opts.dialect, tbl); err != nil {
		return nil, err
	}
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, fmt.Errorf("migrate: apply id: %w", err)
	}
	rep = &Report{ApplyID: e.opts.clock().UTC().Format(applyIDLayout) + "-" + hex.EncodeToString(b[:])}
	env := RunEnv{Driver: e.drv, DB: e.opts.goDB, Conn: c, Dialect: e.opts.dialect, Clock: e.opts.clock, Reconnect: e.pin, HistoryTable: tbl}
	for _, p := range pending {
		if o.To != "" && p.Version > o.To {
			break
		}
		restore, err := e.foreignKeysOff(ctx, c, p.ForeignKeysOff)
		if err != nil {
			return rep, err
		}
		a, err := RunMigration(ctx, env, p, rep.ApplyID)
		if err == nil {
			rep.Applied = append(rep.Applied, a)
		}
		if rerr := restore(); rerr != nil {
			err = errors.Join(err, rerr)
		}
		if err != nil {
			return rep, redactErr(e.classify(err))
		}
	}
	return rep, nil
}

// foreignKeysOff turns foreign-key enforcement off on c when off is set (the
// -- momo:sqlite-foreign-keys-off directive, which every SQLite table rebuild
// carries) and the dialect is SQLite, and returns the function that restores
// it. It runs outside any transaction, as SQLite requires. For any other
// dialect, the directive is refused with ErrValidation, as -- momo:ddl is on a
// dialect that does not support it; when off is false, it does nothing.
func (e *Engine) foreignKeysOff(ctx context.Context, c momo.PinnedConn, off bool) (restore func() error, err error) {
	if !off {
		return func() error { return nil }, nil
	}
	if d := e.opts.dialect.Dialect(); d != nil && d.Name != dialect.SQLite {
		return nil, &momo.Error{Kind: momo.ErrValidation, Op: "migrate", Err: fmt.Errorf("-- momo:sqlite-foreign-keys-off is supported on sqlite only, not %s", d.Name)}
	}
	restore, err = e.opts.dialect.Setup(ctx, c, SessionOptions{ForeignKeysOff: true})
	if err != nil {
		return nil, fmt.Errorf("migrate: turn foreign keys off: %w", err)
	}
	return restore, nil
}

// Status reports the state of every migration version: applied, pending, dirty
// (running or failed), mismatch (the file differs from the recorded checksum or
// from momo.sum) or missing (a history row without a file). It is read-only:
// it takes no lock and creates nothing.
func (e *Engine) Status(ctx context.Context) ([]VersionStatus, error) {
	c, err := e.pin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Release() }()
	hist, err := e.readHistory(ctx, c)
	if err != nil {
		return nil, err
	}
	files, err := Discover(e.dir)
	if err != nil {
		return nil, err
	}
	if err := goCollisions(e.opts.goMigrations, files); err != nil {
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
	for _, g := range e.opts.goMigrations {
		seen[g.Version] = true
		st := VersionStatus{Version: g.Version, Name: g.Name, State: "pending", Checksum: g.identity()}
		if r, ok := rows[g.Version]; ok {
			st.Checksum, st.Error = r.Checksum, r.Error
			switch {
			case r.dirty():
				st.State = "dirty"
			case r.Status == StatusApplied && r.Checksum == g.identity():
				st.State = "applied"
			case r.Status == StatusApplied:
				st.State = "mismatch"
			}
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
func (e *Engine) Resolve(ctx context.Context, version string, state ResolveState) (err error) {
	if state != ResolveApplied && state != ResolveRolledBack {
		return fmt.Errorf("migrate: unknown resolve state %q; use %q or %q", state, ResolveApplied, ResolveRolledBack)
	}
	c, err := e.pin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = c.Release() }()
	// Resolve writes, so it holds the migration lock like Apply.
	end, err := e.session(ctx, c, e.lockWait(ApplyOptions{}))
	if err != nil {
		return err
	}
	defer func() { err = end(err) }()
	tbl, err := resolveHistoryTable(ctx, c, e.opts.dialect)
	if err != nil {
		return err
	}
	if err := ensureHistory(ctx, c, e.opts.dialect, tbl); err != nil {
		return err
	}
	hist, err := readHistoryRows(ctx, c, tbl)
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
		q = fmt.Sprintf(`UPDATE %s SET status = %s, error = '' WHERE version = %s AND status IN ('running', 'failed')`, tbl, ph(d, 1), ph(d, 2))
		args = []any{StatusApplied, version}
	} else {
		q = fmt.Sprintf(`DELETE FROM %s WHERE version = %s AND status IN ('running', 'failed')`, tbl, ph(d, 1))
		args = []any{version}
	}
	if _, err := c.Exec(ctx, q, args); err != nil {
		return fmt.Errorf("migrate: resolve %s: %w", version, err)
	}
	return nil
}

// historyTable is the name of the migration history table. It is engine
// bookkeeping, not part of the application schema, so Inspect leaves it out.
const historyTable = "momo_migrations"

// Inspect reads the live database into a schema, using the configured Schemas
// (default: current_schema()). The momo_migrations history table is removed
// from the result: it is not declared by any model or snapshot, so reporting
// it would turn it into drift or a DROP TABLE.
func (e *Engine) Inspect(ctx context.Context) (*schema.Schema, error) {
	c, err := e.pin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Release() }()
	s, _, err := e.inspectOn(ctx, c)
	return s, err
}

// inspectOn is Inspect on a connection the caller holds. It also returns the
// indexes the dialect reports as invalid (a failed CONCURRENTLY build) when the
// dialect implements InvalidInspector, and applies Config.Exclude to the result
// because the dialects do not.
func (e *Engine) inspectOn(ctx context.Context, c momo.PinnedConn) (*schema.Schema, []string, error) {
	if _, err := excludeMatcher(e.opts.config.Exclude); err != nil {
		return nil, nil, err
	}
	if _, err := excludeMatcher(e.opts.config.OnlyTables); err != nil {
		return nil, nil, fmt.Errorf("migrate: Config.OnlyTables: %w", err)
	}
	schemas, err := e.resolveSchemas(ctx, c)
	if err != nil {
		return nil, nil, err
	}
	opts := InspectOptions{Schemas: schemas, Exclude: e.opts.config.Exclude, Only: e.opts.config.OnlyTables}
	var s *schema.Schema
	var invalid []string
	if ii, ok := e.opts.dialect.(InvalidInspector); ok {
		s, invalid, err = ii.InspectWithInvalid(ctx, c, opts)
	} else {
		s, err = e.opts.dialect.Inspect(ctx, c, opts)
	}
	if err != nil {
		return nil, nil, err
	}
	hs := ""
	if len(schemas) > 0 {
		hs = schemas[0]
	}
	s.Tables = slices.DeleteFunc(s.Tables, func(t schema.Table) bool {
		return t.Name == historyTable && (t.Schema == "" || t.Schema == hs)
	})
	invalid = dropExcludedInvalid(s, invalid, e.opts.config.Exclude)
	return e.hide(s), invalid, nil
}

// dropExcludedInvalid removes from invalid the indexes of a table or view that
// Config.Exclude hides. It must run on the schema before applyExclude strips
// that table, because the index is located through it.
func dropExcludedInvalid(s *schema.Schema, invalid []string, globs []string) []string {
	if len(globs) == 0 || len(invalid) == 0 {
		return invalid
	}
	match, err := excludeMatcher(globs)
	if err != nil {
		return invalid
	}
	return slices.DeleteFunc(slices.Clone(invalid), func(name string) bool {
		ref := invalidIndexRef(s, name)
		i := strings.LastIndexByte(ref.Name, '.')
		return i >= 0 && match(ref.Schema, ref.Name[:i])
	})
}

// migrationName is the form a migration name must have.
var migrationName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Diff compares desired with the schema of the latest snapshot in the
// migration directory (the empty schema when there is none) and renders the
// difference as a new migration. It returns nil, nil when nothing changed.
// Otherwise Migration.Files holds the new files only, by relative name:
// <version>_<name>.up.sql, <version>_<name>.snapshot.json (whose parent is the
// latest snapshot's version) and the updated momo.sum. The version is the
// engine clock in UTC (NextVersion), moved to the second after the latest
// existing version when the clock is not already past it, so versions only grow.
//
// When the rendered plan mixes transactional statements with statements that
// must run outside a transaction (CREATE INDEX CONCURRENTLY, the unique index
// of an added UNIQUE, the VALIDATE CONSTRAINT of an added CHECK), Diff splits
// it: the returned Migration holds the transactional part and Next is a
// trailing "<name>_no_tx" migration, one second later, that carries
// -- momo:no-transaction. Files holds the files of both. A plan of only
// such statements is the single no-transaction migration.
//
// Diff does not touch the database: it reads the directory and renders through
// the dialect. It orders the changes with OrderWithViews and returns the
// ordering error (a dependency cycle) instead of falling back to the unordered
// list. A dialect that implements SchemaRenderer (SQLite) renders from the two
// schemas rather than the changes alone, which is how a SQLite table rebuild
// becomes plain statements in the up file.
func (e *Engine) Diff(_ context.Context, desired *schema.Schema, name string, opts DiffOptions) (*Migration, error) {
	if !migrationName.MatchString(name) {
		return nil, fmt.Errorf("migrate: migration name %q must match %s", name, migrationName)
	}
	if desired == nil {
		return nil, errors.New("migrate: Diff needs a desired schema")
	}
	to := *desired
	if to.Dialect == "" {
		to.Dialect = e.opts.dialect.Dialect().Name
	}
	to = *dropExternalExtensions(&to, e.opts.config.ExternalExtensions)
	// Tables outside Config.OnlyTables are not managed: a snapshot that still
	// holds one (its model was removed) must not plan to drop it.
	to = *applyOnly(&to, e.opts.config.OnlyTables)
	if v, ok := e.opts.dialect.(SchemaValidator); ok {
		if err := v.ValidateSchema(&to); err != nil {
			return nil, err
		}
	}
	from := &schema.Schema{Dialect: to.Dialect}
	parent := ""
	snap, err := LatestSnapshot(e.dir)
	switch {
	case err == nil:
		from = applyOnly(dropExternalExtensions(snap.Schema, e.opts.config.ExternalExtensions), e.opts.config.OnlyTables)
		parent = snap.Version
	case !errors.Is(err, ErrNoSnapshot):
		return nil, err
	}
	version, err := NextVersion(e.opts.clock, e.dir)
	if err != nil {
		return nil, err
	}
	if err := e.checkGoSnapshots(); err != nil {
		return nil, err
	}
	changes, hints, err := Diff(from, &to, opts)
	if err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		return nil, nil
	}
	ordered, err := OrderWithViews(changes, from.Views, to.Views)
	if err != nil {
		return nil, fmt.Errorf("migrate: order changes: %w", err)
	}
	stmts, err := renderPlan(e.opts.dialect, from, &to, ordered)
	if err != nil {
		return nil, err
	}
	m := &Migration{Version: version, Name: name, Kind: "sql", Parent: parent, Up: stmts, Changes: ordered, Snapshot: &to, Hints: hints}
	// A staged NOT NULL column stays nullable in the snapshot; the split below
	// only moves index and constraint changes, so it starts from that schema.
	staged := stagedSnapshot(&to, from, changes, opts)
	m.Snapshot = staged
	finishMigration(m)
	// Statements that must run outside a transaction share a file with
	// transactional ones only when that is all there is: a mixed plan is split
	// so the transactional DDL stays atomic.
	if hasNoTx(stmts) && slices.ContainsFunc(stmts, func(s Statement) bool { return !s.NoTx }) {
		head, _, ok, err := e.splitNoTx(version, name, parent, ordered, staged, hints)
		if err != nil {
			return nil, err
		}
		if ok {
			m = head
		}
	}
	files := []*Migration{m}
	if m.Next != nil {
		files = append(files, m.Next)
	}
	if m.Files, err = e.migrationFiles(files...); err != nil {
		return nil, err
	}
	return m, nil
}

// migrationFiles renders the up file, the snapshot of each migration in ms and
// the extended momo.sum, keyed by relative file name.
func (e *Engine) migrationFiles(ms ...*Migration) (map[string][]byte, error) {
	added := memFS{}
	var names []string
	out := map[string][]byte{}
	for _, m := range ms {
		upName := m.Version + "_" + m.Name + ".up.sql"
		snapName := m.Version + "_" + m.Name + ".snapshot.json"
		var snapBuf bytes.Buffer
		if err := WriteSnapshot(&snapBuf, SnapshotFile{Version: m.Version, ParentVersion: m.Parent, Schema: m.Snapshot}); err != nil {
			return nil, err
		}
		added[upName], added[snapName] = renderUp(m), snapBuf.Bytes()
		names = append(names, snapName, upName)
		out[upName], out[snapName] = added[upName], added[snapName]
	}
	existing, err := e.existingSum()
	if err != nil {
		return nil, err
	}
	sum, err := AppendSum(existing, added, names)
	if err != nil {
		return nil, err
	}
	var sumBuf bytes.Buffer
	if err := sum.Write(&sumBuf); err != nil {
		return nil, err
	}
	out[SumFileName] = sumBuf.Bytes()
	return out, nil
}

// existingSum reads momo.sum from the directory; a directory without one gets
// a sum computed over its current files, as momo migrate hash would write.
func (e *Engine) existingSum() (*Sum, error) {
	f, err := e.dir.Open(SumFileName)
	if errors.Is(err, fs.ErrNotExist) {
		return ComputeSum(e.dir)
	}
	if err != nil {
		return nil, fmt.Errorf("migrate: open %s: %w", SumFileName, err)
	}
	defer func() { _ = f.Close() }()
	return ParseSum(f)
}

// renderUp writes the up file: the directive header, then the statements
// separated by a blank line, each ended by a semicolon.
func renderUp(m *Migration) []byte {
	var b bytes.Buffer
	b.WriteString("-- momo:generated\n")
	if m.Destructive {
		b.WriteString("-- momo:destructive\n")
	}
	if m.NoTx {
		b.WriteString("-- momo:no-transaction\n")
	}
	if slices.ContainsFunc(m.Up, isForeignKeyCheck) {
		// A SQLite table rebuild: Apply turns foreign-key enforcement off
		// around the migration, which PRAGMA foreign_keys cannot do inside the
		// transaction.
		b.WriteString("-- momo:sqlite-foreign-keys-off\n")
	}
	b.WriteString("\n")
	for i, s := range m.Up {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(strings.TrimRight(strings.TrimSpace(s.SQL), "; \n\t"))
		b.WriteString(";")
	}
	b.WriteString("\n")
	return b.Bytes()
}

// memFS holds new files before they are written. AppendSum only reads it
// through fs.ReadFile, so ReadFile is the one supported operation.
type memFS map[string][]byte

func (m memFS) ReadFile(name string) ([]byte, error) {
	if b, ok := m[name]; ok {
		return b, nil
	}
	return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
}

func (m memFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: errors.ErrUnsupported}
}
