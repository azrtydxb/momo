// SPDX-License-Identifier: Apache-2.0

// Package sqlitetest gives an application's tests an ephemeral SQLite database:
// a new file in the test's own temporary directory, opened through
// driver/sqlitedriver (mattn/go-sqlite3 with sqlite-vec), closed and deleted
// when the test ends. It needs no server and no environment variable.
//
// It is a package of its own, and not part of momotest, so that momotest and
// the tests that use it for PostgreSQL do not compile the C sources of SQLite
// and sqlite-vec: only a test binary that imports sqlitetest links them.
// SQLite needs cgo and a non-Windows target; on any other build New skips the
// test, the same way momotest skips when its server URL is unset.
package sqlitetest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/sqlitedriver"
)

// New returns a momo.DB on a new SQLite database file inside tb.TempDir(). The
// database starts empty. Every call opens its own file, so two calls never see
// each other's tables, and tests may run in parallel. The database is closed
// and its file removed when the test ends. opts go to momo.New, for example
// momo.WithHook; sqlitedriver's default configuration applies.
func New(tb testing.TB, opts ...momo.Option) *momo.DB {
	tb.Helper()
	return open(tb, filepath.Join(tb.TempDir(), "momotest.db"), opts)
}

// Template is a SQLite database built once and copied for each test, so a
// schema with many tables is created by DDL once per test binary and not once
// per test. NewTemplate describes the build; Template.New returns a copy.
type Template struct {
	build func(ctx context.Context, db *momo.DB) error
	once  sync.Once
	image []byte
	err   error
}

// NewTemplate returns a Template whose database is made by build: build gets a
// *momo.DB on an empty database and creates the tables (and any rows every test
// should start with) through it. build runs at most once, on the first call of
// Template.New, and nothing it writes to disk outlives that call. Keep the
// Template in a package variable so that every test shares it.
func NewTemplate(build func(ctx context.Context, db *momo.DB) error) *Template {
	return &Template{build: build}
}

// New returns a *momo.DB on a copy of the template database, in a file of its
// own in tb.TempDir(). Writes to the copy are visible to no other test and not
// to the template, so tests may run in parallel. The copy is closed and removed
// when the test ends. opts go to momo.New. When the template could not be built
// the test fails; when SQLite is unavailable on this build it is skipped, like
// New.
func (tp *Template) New(tb testing.TB, opts ...momo.Option) *momo.DB {
	tb.Helper()
	tp.once.Do(tp.load)
	if errors.Is(tp.err, momo.ErrUnsupported) {
		tb.Skipf("sqlitetest: SQLite needs cgo and a non-Windows build: %v", tp.err)
		return nil
	}
	if tp.err != nil {
		tb.Fatalf("sqlitetest: build template: %v", tp.err)
		return nil
	}
	path := filepath.Join(tb.TempDir(), "momotest.db")
	if err := os.WriteFile(path, tp.image, 0o600); err != nil {
		tb.Fatalf("sqlitetest: copy template: %v", err)
		return nil
	}
	return open(tb, path, opts)
}

// load builds the template in a scratch directory and keeps a self-contained
// snapshot of it in memory (VACUUM INTO writes one file with no journal), so no
// file is left behind for a process-exit hook to remove.
func (tp *Template) load() {
	dir, err := os.MkdirTemp("", "sqlitetest-template-")
	if err != nil {
		tp.err = err
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()
	sqldb, drv, err := sqlitedriver.Open(filepath.Join(dir, "build.db"), sqlitedriver.Config{})
	if err != nil {
		tp.err = err
		return
	}
	defer func() { _ = sqldb.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	db := momo.New(drv)
	if err := tp.build(ctx, db); err != nil {
		tp.err = err
		return
	}
	snapshot := filepath.Join(dir, "snapshot.db")
	if _, err := momo.Exec(ctx, db, "VACUUM INTO ?", snapshot); err != nil {
		tp.err = fmt.Errorf("snapshot: %w", err)
		return
	}
	tp.image, tp.err = os.ReadFile(snapshot) //nolint:gosec // the path is inside the directory created above
}

// open opens the database file at path for tb and closes it when tb ends.
func open(tb testing.TB, path string, opts []momo.Option) *momo.DB {
	tb.Helper()
	sqldb, drv, err := sqlitedriver.Open(path, sqlitedriver.Config{})
	if errors.Is(err, momo.ErrUnsupported) {
		// A build without cgo, or for Windows, has only the stub driver.
		tb.Skipf("sqlitetest: SQLite needs cgo and a non-Windows build: %v", err)
		return nil
	}
	if err != nil {
		tb.Fatalf("sqlitetest: open: %v", err)
		return nil
	}
	// Registered after the TempDir cleanup, so it runs first: the file is
	// closed before its directory is removed.
	tb.Cleanup(func() {
		if err := sqldb.Close(); err != nil {
			tb.Errorf("sqlitetest: close: %v", err)
		}
	})
	return momo.New(drv, opts...)
}
