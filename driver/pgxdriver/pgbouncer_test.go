// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const pgbWorkers, pgbIters = 8, 25

// pgbItems is the schema-qualified workload table. It is qualified because the
// pooler may hand a statement a server connection whose search_path is not the
// per-test schema; the tests must not depend on search_path tracking.
var pgbItems string

// openBouncerPool opens a pool on the PgBouncer (transaction mode) port.
func openBouncerPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testdb.PgBouncerURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closePool(t, pool) })
	return pool
}

// runWorkers runs onePass from pgbWorkers goroutines and returns the joined errors.
func runWorkers(ctx context.Context, d momo.Driver) error {
	errs := make(chan error, pgbWorkers*pgbIters)
	var wg sync.WaitGroup
	for w := 0; w < pgbWorkers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < pgbIters; i++ {
				if err := onePass(ctx, d, w, i); err != nil {
					errs <- fmt.Errorf("worker %d iteration %d: %w", w, i, err)
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	var all []error
	for err := range errs {
		all = append(all, err)
	}
	return errors.Join(all...)
}

func setupBouncerItems(ctx context.Context, t *testing.T, d momo.Driver) {
	t.Helper()
	pgbItems = pgx.Identifier{testdb.Schema(t), "pgb_items"}.Sanitize()
	if _, err := d.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %[1]s", pgbItems), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(ctx, fmt.Sprintf("CREATE TABLE %[1]s (id bigserial primary key, worker int not null, n int not null)", pgbItems), nil); err != nil {
		t.Fatal(err)
	}
}

func TestPgBouncerTransactionMode(t *testing.T) {
	ctx := context.Background()
	d := New(openBouncerPool(t), WithExecMode(pgx.QueryExecModeExec))
	setupBouncerItems(ctx, t, d)
	if err := runWorkers(ctx, d); err != nil {
		t.Fatal(err)
	}
	rows, err := d.Query(ctx, fmt.Sprintf("SELECT count(*) FROM %[1]s", pgbItems), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatalf("count returned no row: %v", rows.Err())
	}
	var total int
	if err := rows.Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != pgbWorkers*pgbIters {
		t.Fatalf("rows = %d, want %d", total, pgbWorkers*pgbIters)
	}
	// Phase 2 of the plan (suite.RunAll through the pooler) needs momo-05
	// Task 35, which does not exist yet.
}

// wireCounter wraps a net.Conn and parses the frontend byte stream (pgx to
// server) for extended-protocol messages that create or use a named
// prepared statement: Parse and Bind with a non-empty statement name, and
// Describe/Close of a statement ('S'). It is independent of how the pooler is
// configured, so it holds even if PgBouncer would tolerate named statements.
type wireCounter struct {
	net.Conn
	mu      sync.Mutex
	buf     []byte
	started bool
	named   *atomic.Int64
	total   *atomic.Int64
}

func (w *wireCounter) Write(p []byte) (int, error) {
	n, err := w.Conn.Write(p)
	w.mu.Lock()
	w.buf = append(w.buf, p[:n]...)
	w.drain()
	w.mu.Unlock()
	return n, err
}

// drain consumes complete messages from w.buf.
func (w *wireCounter) drain() {
	for {
		if !w.started { // startup packet: int32 length, no type byte
			if len(w.buf) < 4 {
				return
			}
			l := int(binary.BigEndian.Uint32(w.buf))
			if len(w.buf) < l {
				return
			}
			w.buf, w.started = w.buf[l:], true
			continue
		}
		if len(w.buf) < 5 {
			return
		}
		l := int(binary.BigEndian.Uint32(w.buf[1:5]))
		if len(w.buf) < 1+l {
			return
		}
		typ, body := w.buf[0], w.buf[5:1+l]
		w.buf = w.buf[1+l:]
		switch typ {
		case 'P': // Parse: statement name, query, ...
			w.total.Add(1)
			if len(body) > 0 && body[0] != 0 {
				w.named.Add(1)
			}
		case 'B': // Bind: portal name, statement name, ...
			if i := bytes.IndexByte(body, 0); i >= 0 && i+1 < len(body) && body[i+1] != 0 {
				w.named.Add(1)
			}
		case 'D', 'C': // Describe/Close: 'S' = statement, then its name
			if len(body) > 1 && body[0] == 'S' && body[1] != 0 {
				w.named.Add(1)
			}
		}
	}
}

// bouncerPoolCounting opens a pool on the PgBouncer port whose connections
// count named-statement messages on the wire.
func bouncerPoolCounting(t *testing.T) (pool *pgxpool.Pool, named, parses *atomic.Int64) {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(testdb.PgBouncerURL(t))
	if err != nil {
		t.Fatal(err)
	}
	named, parses = new(atomic.Int64), new(atomic.Int64)
	dial := cfg.ConnConfig.DialFunc
	cfg.ConnConfig.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
		c, err := dial(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		return &wireCounter{Conn: c, named: named, total: parses}, nil
	}
	pool, err = pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closePool(t, pool) })
	return pool, named, parses
}

// TestPgBouncerNoNamedPreparedStatements runs the concurrent workload
// (transactions, reads, batches) and proves from the wire that the default
// mode never creates, binds, describes or closes a named prepared statement.
func TestPgBouncerNoNamedPreparedStatements(t *testing.T) {
	ctx := context.Background()
	pool, named, parses := bouncerPoolCounting(t)
	d := New(pool, WithExecMode(pgx.QueryExecModeExec))
	setupBouncerItems(ctx, t, d)
	if err := runWorkers(ctx, d); err != nil {
		t.Fatal(err)
	}
	if parses.Load() == 0 {
		t.Fatal("wire counter saw no Parse message; the detector is blind")
	}
	if n := named.Load(); n != 0 {
		t.Fatalf("%d named prepared-statement messages on the wire; PgBouncer transaction mode would break", n)
	}
}

// TestPgBouncerWireCounterDetectsNamedStatements is the mutation check as a
// standing test: the statement-caching mode must be seen by the same counter
// (and fails through a pooler without protocol-level prepared statement
// support, see internal/testdb/pgbouncer/pgbouncer.ini).
func TestPgBouncerWireCounterDetectsNamedStatements(t *testing.T) {
	ctx := context.Background()
	setup := New(openBouncerPool(t), WithExecMode(pgx.QueryExecModeExec))
	setupBouncerItems(ctx, t, setup)
	pool, named, _ := bouncerPoolCounting(t)
	d := New(pool, WithExecMode(pgx.QueryExecModeCacheStatement))
	err := runWorkers(ctx, d)
	if named.Load() == 0 {
		t.Fatalf("QueryExecModeCacheStatement produced no named statement on the wire (workload err: %v); the counter cannot detect a regression", err)
	}
}

// TestPgBouncerBatchUsesUnnamedStatements runs batches repeatedly with the
// default mode: batches must not rely on named statements either.
func TestPgBouncerBatchUsesUnnamedStatements(t *testing.T) {
	ctx := context.Background()
	d := New(openBouncerPool(t))
	for i := 0; i < 50; i++ {
		rs, err := d.(momo.Batcher).SendBatch(ctx, []momo.Compiled{
			{SQL: "SELECT $1::int", Args: []any{i}},
			{SQL: "SELECT $1::text, $2::int", Args: []any{"x", i}},
		})
		if err != nil {
			t.Fatalf("batch %d: %v", i, err)
		}
		for _, r := range rs {
			if err := r.Close(); err != nil {
				t.Fatalf("batch %d close: %v", i, err)
			}
		}
	}
}

// TestPgBouncerTransactionScopedState: session state does not survive a
// transaction through the pooler, so the only safe forms are transaction
// scoped: set_config(..., true) and pg_advisory_xact_lock inside one
// transaction on a single backend.
func TestPgBouncerTransactionScopedState(t *testing.T) {
	ctx := context.Background()
	d := New(openBouncerPool(t))
	for i := 0; i < pgbIters; i++ {
		tx, err := d.Begin(ctx, momo.TxOptions{})
		if err != nil {
			t.Fatal(err)
		}
		want := fmt.Sprintf("v%d", i)
		if got := txScalar(ctx, t, tx, "SELECT set_config('momo.pgb', $1, true)::text", want); got != want {
			t.Fatalf("set_config returned %q, want %q", got, want)
		}
		if got := txScalar(ctx, t, tx, "SELECT current_setting('momo.pgb')::text"); got != want {
			t.Fatalf("setting inside its transaction = %q, want %q", got, want)
		}
		if got := txScalar(ctx, t, tx, "SELECT coalesce((SELECT pg_advisory_xact_lock(424242))::text, '')"); got != "" {
			t.Fatalf("pg_advisory_xact_lock returned %q", got)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		// Whichever backend serves the next transaction, the local setting is gone
		// and the advisory lock is released.
		tx2, err := d.Begin(ctx, momo.TxOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if got := txScalar(ctx, t, tx2, "SELECT coalesce(current_setting('momo.pgb', true), '')::text"); got != "" {
			t.Fatalf("transaction-local setting leaked past commit: %q", got)
		}
		if got := txScalar(ctx, t, tx2, "SELECT count(*)::text FROM pg_locks WHERE locktype = 'advisory' AND objid = 424242 AND granted"); got != "0" {
			t.Fatalf("advisory lock still held after commit: %s", got)
		}
		if err := tx2.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

// TestPgBouncerPinnedTransaction: a pinned connection works through the pooler
// when its statements run in a transaction, which is what keeps them on one backend.
func TestPgBouncerPinnedTransaction(t *testing.T) {
	ctx := context.Background()
	d := New(openBouncerPool(t))
	p, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(errors.Join(err, p.Release()))
	}
	got := txScalar(ctx, t, tx, "SELECT pg_try_advisory_xact_lock(424243)::text")
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(errors.Join(err, p.Release()))
	}
	if err := p.Release(); err != nil {
		t.Fatal(err)
	}
	if got != "true" {
		t.Fatalf("pg_try_advisory_xact_lock = %q, want true", got)
	}
}

// txScalar runs a one-text-column query in tx and returns the value.
func txScalar(ctx context.Context, t *testing.T, tx momo.DriverTx, sql string, args ...any) string {
	t.Helper()
	rows, err := tx.Query(ctx, sql, args)
	if err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	defer func() { _ = rows.Close() }()
	var s string
	if !rows.Next() {
		t.Fatalf("%s returned no row: %v", sql, rows.Err())
	}
	if err := rows.Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func onePass(ctx context.Context, d momo.Driver, w, i int) error {
	tx, err := d.Begin(ctx, momo.TxOptions{})
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf("INSERT INTO %[1]s (worker, n) VALUES ($1, $2)", pgbItems), []any{w, i}); err != nil {
		return errors.Join(err, tx.Rollback(ctx))
	}
	mine, err := countWorker(ctx, tx, w)
	if err != nil {
		return errors.Join(err, tx.Rollback(ctx))
	}
	if mine < 1 {
		return errors.Join(fmt.Errorf("worker %d sees %d rows after its insert", w, mine), tx.Rollback(ctx))
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	rs, err := d.(momo.Batcher).SendBatch(ctx, []momo.Compiled{
		{SQL: fmt.Sprintf("UPDATE %[1]s SET n = n WHERE worker = $1", pgbItems), Args: []any{w}},
		{SQL: "SELECT 1", Args: nil},
	})
	if err != nil {
		return err
	}
	var closeErrs []error
	for _, r := range rs {
		closeErrs = append(closeErrs, r.Close())
	}
	return errors.Join(closeErrs...)
}

// countWorker reads one count inside tx. The rows are closed and their Close error is returned.
func countWorker(ctx context.Context, tx momo.DriverTx, w int) (n int, err error) {
	rows, err := tx.Query(ctx, fmt.Sprintf("SELECT count(*) FROM %[1]s WHERE worker = $1", pgbItems), []any{w})
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	if !rows.Next() {
		return 0, errors.Join(errors.New("count returned no row"), rows.Err())
	}
	if err := rows.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
