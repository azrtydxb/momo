// SPDX-License-Identifier: Apache-2.0

// Package migratetest holds helpers shared by the migrate engine tests.
package migratetest

import (
	"bytes"
	"context"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

// WithSum returns a copy of fsys with a momo.sum that matches its files.
func WithSum(t testing.TB, fsys fstest.MapFS) fs.FS {
	t.Helper()
	out := fstest.MapFS{}
	for k, v := range fsys {
		out[k] = v
	}
	delete(out, migrate.SumFileName)
	sum, err := migrate.ComputeSum(out)
	if err != nil {
		t.Fatalf("migratetest: compute sum: %v", err)
	}
	var buf bytes.Buffer
	if err := sum.Write(&buf); err != nil {
		t.Fatalf("migratetest: write sum: %v", err)
	}
	out[migrate.SumFileName] = &fstest.MapFile{Data: buf.Bytes()}
	return out
}

// NewEngine builds an engine on d, drv and fsys, failing the test on error.
func NewEngine(t testing.TB, d migrate.Dialect, drv momo.Driver, fsys fs.FS, opts ...migrate.Option) *migrate.Engine {
	t.Helper()
	e, err := migrate.NewEngine(drv, fsys, append([]migrate.Option{migrate.WithDialect(d)}, opts...)...)
	if err != nil {
		t.Fatalf("migratetest: new engine: %v", err)
	}
	return e
}

// historyDDL is the PostgreSQL momo_migrations table.
var historyDDL = []string{`CREATE TABLE IF NOT EXISTS momo_migrations (
	version TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	checksum TEXT NOT NULL,
	status TEXT NOT NULL,
	statements_applied INTEGER NOT NULL DEFAULT 0,
	error TEXT NOT NULL DEFAULT '',
	applied_at TEXT NOT NULL,
	execution_ms BIGINT NOT NULL DEFAULT 0,
	apply_id TEXT NOT NULL DEFAULT '',
	kind TEXT NOT NULL DEFAULT 'sql'
)`}

// PG is the PostgreSQL dialect for engine tests. It supplies the history DDL,
// session setup and lock that the migration-lock story (task 14) owns in
// migrate/postgres; once that lands, tests can use migrate/postgres.New()
// directly and this wrapper goes away.
type PG struct{ inner mpg.Dialect }

// Dialect describes PostgreSQL.
func (PG) Dialect() *dialect.Dialect { return dialect.For(dialect.Postgres) }

// Inspect delegates to the PostgreSQL dialect.
func (p PG) Inspect(ctx context.Context, c momo.PinnedConn, opts migrate.InspectOptions) (*schema.Schema, error) {
	return p.inner.Inspect(ctx, c, opts)
}

// Render delegates to the PostgreSQL dialect.
func (p PG) Render(plan []migrate.Change) ([]migrate.Statement, error) { return p.inner.Render(plan) }

// HistoryDDL returns the momo_migrations DDL.
func (PG) HistoryDDL() []string { return historyDDL }

// Setup applies no session options.
func (PG) Setup(context.Context, momo.PinnedConn, migrate.SessionOptions) (func() error, error) {
	return func() error { return nil }, nil
}

// Lock takes no lock.
func (PG) Lock(context.Context, momo.PinnedConn, string) (func() error, func(context.Context) (string, error), error) {
	return func() error { return nil }, func(context.Context) (string, error) { return "", nil }, nil
}

// Postgres returns the PostgreSQL test dialect.
func Postgres() migrate.Dialect { return PG{} }
