// SPDX-License-Identifier: Apache-2.0

// Package testopen opens the integration-test databases as *momo.DB. It imports
// the drivers, so it is the one place that turns a testdb URL into a database
// handle; testdb itself imports no driver.
package testopen

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

const connectTimeout = 30 * time.Second

// maxTestConns is the pool size testopen.Postgres gives a pool whose URL sets
// no pool_max_conns. Three, because the locking tests hold two transactions
// at once and still query through the pool; below CI's four, so a leak that
// would exhaust a CI pool exhausts this one first.
const maxTestConns = 3

// opener opens a fresh database for one dialect.
type opener struct {
	name dialect.Name
	open func(*testing.T, ...momo.Option) *momo.DB
}

// openers is the build-time list of dialects. Each driver milestone appends its
// entry: M1 registers PostgreSQL; M2 registers SQLite; M3 registers MySQL84,
// MySQL97 and MariaDB. The list is not mutable at runtime.
var openers = []opener{{dialect.Postgres, Postgres}}

// Postgres opens a fresh *momo.DB on testdb.PostgresURL(t) through pgxdriver.
// opts are passed to momo.New, so options such as momo.WithHook and
// momo.WithArgLogging reach the database. The pool is closed by t.Cleanup. It
// fails the test when the URL is unset and MOMO_REQUIRE_INTEGRATION=1, and
// skips it otherwise (testdb does the check).
func Postgres(t *testing.T, opts ...momo.Option) *momo.DB {
	t.Helper()
	// PostgresURL registers the schema drop first, so the pool's Close below
	// runs before the schema is dropped (cleanups run last-in first-out).
	url := testdb.PostgresURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("testopen: parse postgres URL: %v", err)
	}
	if !strings.Contains(url, "pool_max_conns") {
		// A small pool makes a test that holds a connection past its use (a
		// pin released only in t.Cleanup, say) block in Acquire on any machine.
		// pgxpool's default is max(4, NumCPU), so such a leak otherwise shows
		// only on a CI runner with few cores.
		cfg.MaxConns = maxTestConns
	}
	pgxdriver.InstrumentPool(cfg)
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("testopen: open postgres pool: %v", err)
	}
	t.Cleanup(func() { closePool(pool, closeTimeout, t.Errorf) })
	return momo.New(pgxdriver.New(pool), opts...)
}

// Each runs fn once per dialect whose opener is registered in this build, as a
// subtest named after the dialect, each on a freshly opened database. Each
// passes opts to every opener. Subtests do not run in parallel.
func Each(t *testing.T, fn func(t *testing.T, db *momo.DB, d dialect.Name), opts ...momo.Option) {
	t.Helper()
	for _, o := range openers {
		t.Run(string(o.name), func(t *testing.T) {
			fn(t, o.open(t, opts...), o.name)
		})
	}
}

// closeTimeout bounds how long a test waits for pool.Close.
const closeTimeout = 5 * time.Second

// closePool closes pool, but unlike pool.Close it cannot hang a test binary on
// a leaked connection: it closes in a goroutine and, when that takes longer
// than timeout, reports the acquired-connection count through report.
func closePool(pool *pgxpool.Pool, timeout time.Duration, report func(format string, args ...any)) {
	acquired := pool.Stat().AcquiredConns()
	done := make(chan struct{})
	go func() { pool.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
		report("testopen: pool.Close blocked for %v: %d connection(s) still acquired (leaked)", timeout, acquired)
	}
}
