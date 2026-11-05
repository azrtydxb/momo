// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBatchUnderDefaultModeCreatesNoPreparedStatements(t *testing.T) {
	d := New(openPool(t))
	ctx := context.Background()
	p, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Release() }()
	out, err := p.(momo.Batcher).SendBatch(ctx, []momo.Compiled{
		{SQL: "SELECT $1::int", Args: []any{5}},
		{SQL: "SELECT $1::text, $2::int", Args: []any{"a", 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := readInt(t, out[0]); got != 5 {
		t.Fatalf("first = %d, want 5", got)
	}
	rs, err := p.Query(ctx, "SELECT count(*) FROM pg_prepared_statements", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := readInt(t, rs); got != 0 {
		t.Fatalf("%d prepared statement(s) left on the session by a batch under the default exec mode", got)
	}
}

func TestBatchThroughPgBouncerTransactionPooling(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), testdb.PgBouncerURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closePool(t, pool) })
	d := New(pool).(momo.Batcher)
	for i := 0; i < 3; i++ {
		out, err := d.SendBatch(context.Background(), []momo.Compiled{{SQL: "SELECT $1::int", Args: []any{i}}})
		if err != nil {
			t.Fatalf("batch %d through PgBouncer: %v", i, err)
		}
		if got := readInt(t, out[0]); got != i {
			t.Fatalf("batch %d = %d", i, got)
		}
	}
}

func TestBatchRejectsExecModeThatDiffersFromConnectionDefault(t *testing.T) {
	d := New(openPool(t), WithExecMode(pgx.QueryExecModeCacheDescribe)).(momo.Batcher)
	_, err := d.SendBatch(context.Background(), []momo.Compiled{{SQL: "SELECT 1"}})
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

// cancelSource streams rows and cancels the context after n of them.
type cancelSource struct {
	i, n   int
	cancel context.CancelFunc
}

func (s *cancelSource) Next() bool {
	s.i++
	if s.i == s.n {
		s.cancel()
	}
	return s.i <= 5_000_000
}
func (s *cancelSource) Values() ([]any, error) { return []any{int64(s.i)}, nil }
func (s *cancelSource) Err() error             { return nil }

func TestCopyCancelledMidStreamStopsAndReleasesConnection(t *testing.T) {
	pool := openPool(t)
	d := New(pool)
	ctx0 := context.Background()
	if _, err := d.Exec(ctx0, "CREATE TABLE IF NOT EXISTS t_copy_mid (id bigint)", nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.Exec(context.Background(), "DROP TABLE IF EXISTS t_copy_mid", nil) })
	ctx, cancel := context.WithCancel(ctx0)
	defer cancel()
	start := time.Now()
	_, err := copyFromSource(ctx, pool, "t_copy_mid", []string{"id"}, &cancelSource{n: 1000, cancel: cancel})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if time.Since(start) > 30*time.Second {
		t.Fatal("the copy did not stop on cancel")
	}
	deadline := time.Now().Add(3 * time.Second)
	for pool.Stat().AcquiredConns() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := pool.Stat().AcquiredConns(); got != 0 {
		t.Fatalf("%d connection(s) still acquired after a mid-copy cancel", got)
	}
	rs, err := d.Query(ctx0, "SELECT count(*) FROM t_copy_mid", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := readInt(t, rs); got != 0 {
		t.Fatalf("cancelled copy left %d rows", got)
	}
}
