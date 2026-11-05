// SPDX-License-Identifier: Apache-2.0

package migratetest

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// CountingDriver wraps a driver and counts the statements the engine executes
// through it, on the driver itself, on pinned connections and in transactions.
// The migration engine reaches the database through Pin, so wrapping the
// driver is enough to observe it.
type CountingDriver struct {
	inner  momo.Driver
	ddl    atomic.Int64
	writes atomic.Int64
}

// NewCountingDriver wraps inner. inner must implement momo.Pinner for the
// engine to use it.
func NewCountingDriver(inner momo.Driver) *CountingDriver { return &CountingDriver{inner: inner} }

// DDL is the number of CREATE, ALTER, DROP and TRUNCATE statements executed so
// far, not counting those on the migration history table: that table is engine
// bookkeeping, not the user's schema.
func (c *CountingDriver) DDL() int { return int(c.ddl.Load()) }

// Writes is the number of statements executed through Exec so far that are not
// session settings (SET, RESET) or advisory-lock calls. A read-only operation
// leaves it at zero.
func (c *CountingDriver) Writes() int { return int(c.writes.Load()) }

func (c *CountingDriver) count(sql string) {
	s := strings.ToUpper(strings.TrimSpace(sql))
	first, _, _ := strings.Cut(s, " ")
	switch first {
	case "SET", "RESET", "SELECT", "BEGIN", "COMMIT", "ROLLBACK":
		return
	case "CREATE", "ALTER", "DROP", "TRUNCATE":
		if !strings.Contains(s, "MOMO_MIGRATIONS") {
			c.ddl.Add(1)
		}
	}
	if !strings.Contains(s, "MOMO_MIGRATIONS") {
		c.writes.Add(1)
	}
}

// Dialect forwards to the wrapped driver.
func (c *CountingDriver) Dialect() *dialect.Dialect { return c.inner.Dialect() }

// Query forwards to the wrapped driver.
func (c *CountingDriver) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	return c.inner.Query(ctx, sql, args)
}

// Exec counts and forwards.
func (c *CountingDriver) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	c.count(sql)
	return c.inner.Exec(ctx, sql, args)
}

// Begin forwards and counts the statements of the transaction.
func (c *CountingDriver) Begin(ctx context.Context, opts momo.TxOptions) (momo.DriverTx, error) {
	tx, err := c.inner.Begin(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &countingTx{DriverTx: tx, c: c}, nil
}

// Pin pins a connection of the wrapped driver.
func (c *CountingDriver) Pin(ctx context.Context) (momo.PinnedConn, error) {
	p, ok := c.inner.(momo.Pinner)
	if !ok {
		return nil, errors.New("migratetest: the wrapped driver cannot pin a connection")
	}
	pc, err := p.Pin(ctx)
	if err != nil {
		return nil, err
	}
	return &countingConn{PinnedConn: pc, c: c}, nil
}

type countingTx struct {
	momo.DriverTx
	c *CountingDriver
}

func (t *countingTx) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	t.c.count(sql)
	return t.DriverTx.Exec(ctx, sql, args)
}

type countingConn struct {
	momo.PinnedConn
	c *CountingDriver
}

func (p *countingConn) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	p.c.count(sql)
	return p.PinnedConn.Exec(ctx, sql, args)
}

func (p *countingConn) Begin(ctx context.Context, opts momo.TxOptions) (momo.DriverTx, error) {
	tx, err := p.PinnedConn.Begin(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &countingTx{DriverTx: tx, c: p.c}, nil
}

// Discard destroys the connection when the wrapped one supports it, as the
// migration session cleanup needs.
func (p *countingConn) Discard() error {
	if d, ok := p.PinnedConn.(momo.Discarder); ok {
		return d.Discard()
	}
	return p.PinnedConn.Release()
}
