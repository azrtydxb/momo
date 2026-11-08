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
	"errors"
	"path/filepath"
	"testing"

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
	sqldb, drv, err := sqlitedriver.Open(filepath.Join(tb.TempDir(), "momotest.db"), sqlitedriver.Config{})
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
