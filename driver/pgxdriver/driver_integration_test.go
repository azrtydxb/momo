// SPDX-License-Identifier: Apache-2.0

//go:build integration

package pgxdriver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func openPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closePool(t, pool) })
	return pool
}

// closePool closes pool but, unlike pool.Close, cannot hang the test binary
// when a connection was leaked: it reports the leak and gives up after 5s.
func closePool(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	acquired := pool.Stat().AcquiredConns()
	done := make(chan struct{})
	go func() { pool.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Errorf("pool.Close blocked for 5s: %d connection(s) still acquired (leaked)", acquired)
	}
}

func TestExecModeExecIsApplied(t *testing.T) {
	d := New(openPool(t), WithExecMode(pgx.QueryExecModeExec))
	rows, err := d.Query(context.Background(), "SELECT $1::text", []any{"a'b"})
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got string
	if !rows.Next() {
		t.Fatal("no row")
	}
	if err := rows.Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "a'b" {
		t.Fatalf("got %q", got)
	}
}

func TestSimpleProtocolRejected(t *testing.T) {
	d := New(openPool(t), WithExecMode(pgx.QueryExecModeSimpleProtocol))
	ctx := context.Background()
	if _, err := d.Query(ctx, "SELECT $1::int", []any{1}); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("simple protocol Query err = %v, want ErrUnsupported", err)
	}
	if _, err := d.Exec(ctx, "SELECT $1::int", []any{1}); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("simple protocol Exec err = %v, want ErrUnsupported", err)
	}
	if _, err := d.Begin(ctx, momo.TxOptions{}); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("simple protocol Begin err = %v, want ErrUnsupported", err)
	}
}

func TestDefaultExecModeNoNamedStatements(t *testing.T) {
	d := New(openPool(t))
	ctx := context.Background()
	p, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Release() }()
	rows, err := p.Query(ctx, "SELECT $1::int", []any{1})
	if err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	rows, err = p.Query(ctx, "SELECT count(*) FROM pg_prepared_statements", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	rows.Next()
	var n int
	if err := rows.Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("default mode left %d named statements; PgBouncer transaction pooling would break", n)
	}
}

// The same property as above on a connection the driver itself pins for the
// life of a transaction, so it is proven without the Pinner interface.
func TestDefaultExecModeNoNamedStatementsInTx(t *testing.T) {
	d := New(openPool(t))
	ctx := context.Background()
	tx, err := d.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, "SELECT $1::int", []any{1})
	if err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	rows, err = tx.Query(ctx, "SELECT count(*) FROM pg_prepared_statements", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var n int
	if !rows.Next() {
		t.Fatal("no row")
	}
	if err := rows.Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("default mode left %d named statements", n)
	}
}

func TestCapabilitiesAndDialect(t *testing.T) {
	d := New(openPool(t))
	if !d.(momo.CapabilityReporter).Capabilities().Vectors {
		t.Fatal("Vectors capability not reported")
	}
	if d.Dialect() == nil || d.Dialect().Name != "postgres" {
		t.Fatalf("dialect = %v", d.Dialect())
	}
}

func TestCancelledContextPropagates(t *testing.T) {
	d := New(openPool(t))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := d.Exec(ctx, "SELECT pg_sleep(10)", nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded reachable", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("cancellation did not reach the server")
	}
	cctx, ccancel := context.WithCancel(context.Background())
	ccancel()
	if _, err := d.Query(cctx, "SELECT 1", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Query with cancelled ctx err = %v", err)
	}
}

func TestRowsCloseReleasesConnection(t *testing.T) {
	pool := openPool(t)
	d := New(pool)
	ctx := context.Background()
	rows, err := d.Query(ctx, "SELECT g FROM generate_series(1,100) g", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() { // early exit: read one of 100 rows
		t.Fatal("no row")
	}
	if got := pool.Stat().AcquiredConns(); got != 1 {
		t.Fatalf("acquired while open = %d, want 1", got)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if got := pool.Stat().AcquiredConns(); got != 0 {
		t.Fatalf("acquired after Close = %d, want 0 (connection leaked)", got)
	}
	if rows.Next() {
		t.Fatal("Next after Close returned true")
	}
}

func TestErrorsAreClassifiedAndRedacted(t *testing.T) {
	d := New(openPool(t))
	ctx := context.Background()
	if _, err := d.Exec(ctx, "CREATE TABLE u (e text UNIQUE)", nil); err != nil {
		t.Fatal(err)
	}
	secret := "secret-" + "value-12345"
	n, err := d.Exec(ctx, "INSERT INTO u VALUES ($1)", []any{secret})
	if err != nil || n != 1 {
		t.Fatalf("insert n=%d err=%v", n, err)
	}
	_, err = d.Exec(ctx, "INSERT INTO u VALUES ($1)", []any{secret})
	if !errors.Is(err, momo.ErrUniqueViolation) {
		t.Fatalf("err = %v, want ErrUniqueViolation", err)
	}
	if strings.Contains(fmt.Sprint(err), secret) || strings.Contains(fmt.Sprintf("%+v", err), secret) {
		t.Fatalf("error text leaks argument value: %v", err)
	}
	// The violation of a row-returning statement surfaces from Query or from
	// Rows.Err; either way it must be classified and redacted.
	rows, err := d.Query(ctx, "INSERT INTO u VALUES ($1) RETURNING e", []any{secret})
	if err == nil {
		for rows.Next() {
		}
		err = rows.Err()
		_ = rows.Close()
	}
	if !errors.Is(err, momo.ErrUniqueViolation) {
		t.Fatalf("query err = %v, want ErrUniqueViolation", err)
	}
	if strings.Contains(fmt.Sprint(err), secret) {
		t.Fatalf("query error leaks value: %v", err)
	}
}

func TestEncodeErrorDoesNotLeakArgument(t *testing.T) {
	d := New(openPool(t))
	secret := "tok-" + "9f8e7d6c"
	rows, err := d.Query(context.Background(), "SELECT $1::int", []any{secret})
	if err == nil {
		for rows.Next() {
		}
		err = rows.Err()
		_ = rows.Close()
	}
	if err == nil {
		t.Fatal("expected an encode error")
	}
	if strings.Contains(fmt.Sprint(err), secret) {
		t.Fatalf("error leaks argument value: %v", err)
	}
}

func TestTxIsolationAndReadOnly(t *testing.T) {
	d := New(openPool(t))
	ctx := context.Background()
	show := func(tx momo.DriverTx, what string) string {
		t.Helper()
		rows, err := tx.Query(ctx, "SHOW "+what, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rows.Close() }()
		var s string
		if !rows.Next() {
			t.Fatal("no row")
		}
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	cases := []struct {
		lvl  sql.IsolationLevel
		want string
	}{
		{sql.LevelDefault, "read committed"},
		{sql.LevelReadUncommitted, "read uncommitted"},
		{sql.LevelReadCommitted, "read committed"},
		{sql.LevelRepeatableRead, "repeatable read"},
		{sql.LevelSerializable, "serializable"},
	}
	for _, c := range cases {
		tx, err := d.Begin(ctx, momo.TxOptions{Isolation: c.lvl})
		if err != nil {
			t.Fatal(err)
		}
		if got := show(tx, "transaction_isolation"); got != c.want {
			t.Errorf("level %v: got %q want %q", c.lvl, got, c.want)
		}
		if got := show(tx, "transaction_read_only"); got != "off" {
			t.Errorf("level %v: read_only = %q", c.lvl, got)
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := d.Begin(ctx, momo.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := show(tx, "transaction_read_only"); got != "on" {
		t.Errorf("read-only tx: read_only = %q", got)
	}
	if _, err := tx.Exec(ctx, "CREATE TABLE ro_denied (a int)", nil); err == nil {
		t.Error("write in read-only tx succeeded")
	}
	_ = tx.Rollback(ctx)

	if _, err := d.Begin(ctx, momo.TxOptions{Isolation: sql.LevelSnapshot}); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("snapshot isolation err = %v, want ErrUnsupported", err)
	}
}

func TestTxCommitRollbackAndDone(t *testing.T) {
	pool := openPool(t)
	d := New(pool)
	ctx := context.Background()
	if _, err := d.Exec(ctx, "CREATE TABLE c (a int)", nil); err != nil {
		t.Fatal(err)
	}
	tx, err := d.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := tx.Exec(ctx, "INSERT INTO c VALUES (1)", nil); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); !errors.Is(err, momo.ErrTxDone) {
		t.Fatalf("commit after rollback = %v, want ErrTxDone", err)
	}
	tx, err = d.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO c VALUES (2)", nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := d.Query(ctx, "SELECT a FROM c", nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []int
	for rows.Next() {
		var a int
		if err := rows.Scan(&a); err != nil {
			t.Fatal(err)
		}
		got = append(got, a)
	}
	_ = rows.Close()
	if len(got) != 1 || got[0] != 2 {
		t.Fatalf("rows = %v, want [2]", got)
	}
	if n := pool.Stat().AcquiredConns(); n != 0 {
		t.Fatalf("acquired = %d after tx done", n)
	}
}

func TestConcurrentUse(t *testing.T) {
	d := New(openPool(t))
	// Bounded so a leaked connection fails the test instead of hanging it.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rows, err := d.Query(ctx, "SELECT $1::int", []any{i})
			if err != nil {
				errs <- err
				return
			}
			defer func() { _ = rows.Close() }()
			var got int
			if !rows.Next() {
				errs <- errors.New("no row")
				return
			}
			if err := rows.Scan(&got); err != nil {
				errs <- err
				return
			}
			if got != i {
				errs <- fmt.Errorf("got %d want %d", got, i)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestScanErrorDoesNotLeakValue(t *testing.T) {
	d := New(openPool(t))
	secret := "tok-" + "scan-4711"
	rows, err := d.Query(context.Background(), "SELECT $1::text", []any{secret})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatal("no row")
	}
	var n int
	err = rows.Scan(&n)
	if err == nil {
		t.Fatal("expected a scan error")
	}
	if strings.Contains(fmt.Sprint(err), secret) || strings.Contains(fmt.Sprintf("%+v", err), secret) {
		t.Fatalf("scan error leaks value: %v", err)
	}
}

func TestRowsErrAfterContextCancel(t *testing.T) {
	d := New(openPool(t))
	ctx, cancel := context.WithCancel(context.Background())
	rows, err := d.Query(ctx, "SELECT g FROM generate_series(1,1000000) g", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatal("no row")
	}
	cancel()
	for rows.Next() {
	}
	if err := rows.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Rows.Err = %v, want context.Canceled reachable", err)
	}
}
