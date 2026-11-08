// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/azrtydxb/momo"
)

// GoMigration is a migration written in Go, for a data step that SQL cannot
// express (the Go-side templates an application ships, a backfill that calls
// library code). Register it with WithGoMigrations; the Engine orders it with
// the SQL files by Version and records it in the history table with kind "go".
//
// Exactly one of Up and UpNoTx is set. Up runs in the migration transaction,
// together with the running row and the final applied row, so a failure rolls
// everything back and the version is retried. UpNoTx runs outside a
// transaction on the pinned migration connection (a handle from
// momo.DB.OnConn); a failure leaves the history row failed, which is dirty
// until Engine.Resolve settles it, exactly as for a SQL file marked
// -- momo:no-transaction. A panic in either function is turned into an error.
//
// Apply holds the migration lock around every Go migration, so one runs at a
// time and only once per database. Go migrations are forward-only: they have
// no down step, and a rollback refuses a version recorded with kind "go".
//
// A Go migration has no file, so the history checksum cannot hash its code. It
// is a stable identity instead: a hash of the version, the name and the
// optional Checksum string. Change Checksum (for example "v2") on purpose to
// have Apply refuse a migration whose behavior was edited after it ran, or
// leave it empty to bind only version and name.
//
// Like a custom SQL migration, a Go migration does not change the schema: its
// snapshot file, <version>_<name>.snapshot.json, equals its parent's, and
// ValidateOffline checks that when it is given the registered migrations. The
// momo CLI cannot run Go migrations, because they exist only in the
// application's process; see docs/migrations.md.
type GoMigration struct {
	Version string // 14-digit UTC timestamp, like a SQL file; unique among SQL files too
	Name    string // lowercase, matching ^[a-z][a-z0-9_]*$
	// Up runs in the migration transaction.
	Up func(ctx context.Context, tx *momo.Tx) error
	// UpNoTx runs on the pinned connection without a transaction.
	UpNoTx func(ctx context.Context, db *momo.DB) error
	// Checksum is an optional string folded into the recorded checksum.
	Checksum string
}

var goName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// NoTx reports whether the migration runs without a transaction.
func (g GoMigration) NoTx() bool { return g.UpNoTx != nil }

// validate checks the fields of one migration.
func (g GoMigration) validate() error {
	if _, err := time.Parse(versionLayout, g.Version); err != nil || len(g.Version) != len(versionLayout) {
		return fmt.Errorf("migrate: Go migration version %q is not a UTC timestamp (YYYYMMDDHHMMSS)", g.Version)
	}
	if !goName.MatchString(g.Name) {
		return fmt.Errorf("migrate: Go migration %s name %q must match %s", g.Version, g.Name, goName)
	}
	if (g.Up == nil) == (g.UpNoTx == nil) {
		return fmt.Errorf("migrate: Go migration %s_%s must set exactly one of Up and UpNoTx", g.Version, g.Name)
	}
	return nil
}

// identity is the checksum recorded in the history row: the hex sha256 of the
// version, the name and the Checksum string.
func (g GoMigration) identity() string {
	h := sha256.Sum256([]byte("go\x00" + g.Version + "\x00" + g.Name + "\x00" + g.Checksum))
	return hex.EncodeToString(h[:])
}

// WithGoMigrations registers Go migrations on the Engine. They are ordered with
// the SQL files by version; a version shared with a SQL migration file, or
// with another Go migration, is an error that NewEngine (for the latter) or
// Plan, Apply and Status (for the former, which needs the directory) return.
func WithGoMigrations(ms ...GoMigration) Option {
	return func(o *options) { o.goMigrations = append(o.goMigrations, ms...) }
}

// WithGoMigrationDB sets the handle that Go migrations are built on. Without
// it they get a bare momo.New of the engine's driver, with none of the
// application's hooks, field hooks, clock or other options. The handle must use
// the same driver (pool) as the engine; each migration gets its transaction
// (AdoptTx) or the pinned connection (OnConn) from a copy of it.
func WithGoMigrationDB(db *momo.DB) Option {
	return func(o *options) { o.goDB = db }
}

// checkGoMigrations validates the registered migrations among themselves.
func checkGoMigrations(ms []GoMigration) error {
	seen := map[string]bool{}
	for _, g := range ms {
		if err := g.validate(); err != nil {
			return err
		}
		if seen[g.Version] {
			return fmt.Errorf("migrate: two Go migrations share version %s", g.Version)
		}
		seen[g.Version] = true
	}
	return nil
}

// goCollisions refuses a Go migration whose version is also a SQL file's.
func goCollisions(ms []GoMigration, files []File) error {
	for _, g := range ms {
		for _, f := range files {
			if f.Kind == "up" && f.Version == g.Version {
				return fmt.Errorf("migrate: Go migration %s_%s has the same version as %s", g.Version, g.Name, f.Path)
			}
		}
	}
	return nil
}

// goPending lists the Go migrations without an applied row, as Pending entries.
func (e *Engine) goPending(done map[string]bool) []Pending {
	var out []Pending
	for _, g := range e.opts.goMigrations {
		if done[g.Version] {
			continue
		}
		out = append(out, Pending{Version: g.Version, Name: g.Name, Kind: "go", NoTx: g.NoTx(), Checksum: g.identity(), Go: &g})
	}
	return out
}

// goByVersion indexes the registered Go migrations.
func (e *Engine) goByVersion() map[string]GoMigration {
	m := map[string]GoMigration{}
	for _, g := range e.opts.goMigrations {
		m[g.Version] = g
	}
	return m
}

// sortPending orders pending migrations by version.
func sortPending(p []Pending) {
	slices.SortFunc(p, func(a, b Pending) int { return strings.Compare(a.Version, b.Version) })
}

// callGo runs the function of a Go migration, turning a panic into an error.
// tx is the migration transaction, or nil for a no-transaction migration.
func callGo(ctx context.Context, env RunEnv, p Pending, tx momo.DriverTx) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	db := env.DB
	if db == nil {
		if env.Driver == nil {
			return errors.New("a Go migration needs RunEnv.Driver or RunEnv.DB")
		}
		db = momo.New(env.Driver)
	}
	if tx != nil {
		return p.Go.Up(ctx, db.AdoptTx(tx))
	}
	return p.Go.UpNoTx(ctx, db.OnConn(env.Conn))
}

// checkGoSnapshots refuses Diff while a registered Go migration has no
// snapshot file: the new migration would be compared with, and chained to, a
// snapshot that skips the Go migration's version.
func (e *Engine) checkGoSnapshots() error {
	if len(e.opts.goMigrations) == 0 {
		return nil
	}
	files, err := Discover(e.dir)
	if err != nil {
		return err
	}
	for _, g := range e.opts.goMigrations {
		if !slices.ContainsFunc(files, func(f File) bool { return f.Kind == "snapshot" && f.Version == g.Version }) {
			return fmt.Errorf("migrate: Go migration %s_%s has no snapshot file %s_%s.snapshot.json; create it (a copy of the previous snapshot) before diff", g.Version, g.Name, g.Version, g.Name)
		}
	}
	return nil
}

// RefuseGoRollback returns ErrGoMigrationNotReversible, naming the first
// version, when any of the history rows to be rolled back has kind "go". A
// rollback (Engine.Rollback, when it exists) calls it before it runs any down
// step, because a Go migration has none.
func RefuseGoRollback(rows []HistoryRow) error {
	for _, r := range rows {
		if r.Kind == "go" {
			return fmt.Errorf("%w: version %s (%s)", ErrGoMigrationNotReversible, r.Version, r.Name)
		}
	}
	return nil
}

// unregisteredGo returns the snapshot-only versions (a snapshot file and no up
// file) that are not registered Go migrations and have no history row: Go
// migrations this engine cannot run. Apply refuses to run later SQL past one.
func (e *Engine) unregisteredGo(files []File, done map[string]bool) []File {
	ups := map[string]bool{}
	for _, f := range files {
		if f.Kind == "up" {
			ups[f.Version] = true
		}
	}
	reg := e.goByVersion()
	var out []File
	for _, f := range files {
		if _, ok := reg[f.Version]; f.Kind == "snapshot" && !ups[f.Version] && !ok && !done[f.Version] {
			out = append(out, f)
		}
	}
	return out
}

// checkGoRunnable refuses with ErrGoMigrationUnregistered when a pending SQL
// migration would run past a Go migration this engine cannot run.
func (e *Engine) checkGoRunnable(pending []Pending, hist []HistoryRow, to string) error {
	files, err := Discover(e.dir)
	if err != nil {
		return err
	}
	done := map[string]bool{}
	for _, r := range hist {
		if r.Status != StatusRolledBack {
			done[r.Version] = true
		}
	}
	for _, f := range e.unregisteredGo(files, done) {
		for _, p := range pending {
			if p.Version > f.Version && (to == "" || p.Version <= to) {
				return fmt.Errorf("%w: %s has no up file and no registered Go migration, and %s_%s would run after it. The momo CLI cannot run Go migrations: apply with the application's own Engine.Apply (migrate.WithGoMigrations)", ErrGoMigrationUnregistered, f.Path, p.Version, p.Name)
			}
		}
	}
	return nil
}

// validateGo adds the problems of the registered Go migrations to a
// ValidateOffline report: a version shared with a SQL file, a missing
// snapshot, and a snapshot that changes the schema.
func validateGo(ms []GoMigration, files []File, snapshots map[string]chainEntry, add func(string, ...any)) {
	if err := checkGoMigrations(ms); err != nil {
		add("%v", err)
	}
	for _, g := range ms {
		for _, f := range files {
			if f.Kind == "up" && f.Version == g.Version {
				add("%s: has the same version as Go migration %s_%s", f.Path, g.Version, g.Name)
			}
		}
		e, ok := snapshots[g.Version]
		if !ok {
			add("Go migration %s_%s has no snapshot file %s_%s.snapshot.json", g.Version, g.Name, g.Version, g.Name)
			continue
		}
		if e.snap.ParentVersion == "" {
			continue
		}
		parent, ok := snapshots[e.snap.ParentVersion]
		if !ok {
			continue // reported as a missing parent
		}
		if !schemaEqual(e.snap.Schema, parent.snap.Schema) {
			add("%s: Go migration %s_%s must not change the schema, but its snapshot differs from its parent's %s", e.path, g.Version, g.Name, parent.path)
		}
	}
}
