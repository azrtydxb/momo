// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
)

func TestCopyInsertMatchesRows(t *testing.T) {
	d := New(openPool(t)).(momo.Copier)
	ctx := context.Background()
	if _, err := d.(momo.Driver).Exec(ctx, "CREATE TEMP TABLE t_copy (id bigint, name text)", nil); err != nil {
		t.Fatal(err)
	}
	n, err := d.CopyFrom(ctx, "t_copy", []string{"id", "name"}, [][]any{{int64(1), "a"}, {int64(2), "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("copied %d, want 2", n)
	}
}

func TestBatchRunsAllStatementsInOneRoundTrip(t *testing.T) {
	d := New(openPool(t)).(momo.Batcher)
	out, err := d.SendBatch(context.Background(), []momo.Compiled{
		{SQL: "SELECT 1", Args: nil},
		{SQL: "SELECT $1::int", Args: []any{2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("got %d result sets, want 2", len(out))
	}
}

// readInt scans the single int of the next row and closes r.
func readInt(t *testing.T, r momo.Rows) int {
	t.Helper()
	if !r.Next() {
		t.Fatalf("no row: %v", r.Err())
	}
	var v int
	if err := r.Scan(&v); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestBatchResultsAreReadableInAnyOrderAndNoRowStatementIsEmpty(t *testing.T) {
	pool := openPool(t)
	d := New(pool).(momo.Batcher)
	out, err := d.SendBatch(context.Background(), []momo.Compiled{
		{SQL: "SELECT 1"},
		{SQL: "SET LOCAL application_name = 'x'"},
		{SQL: "SELECT $1::int", Args: []any{7}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := readInt(t, out[2]); got != 7 {
		t.Fatalf("third result = %d, want 7", got)
	}
	if out[1].Next() {
		t.Fatal("statement without rows must yield an empty Rows")
	}
	if err := out[1].Close(); err != nil {
		t.Fatal(err)
	}
	if got := readInt(t, out[0]); got != 1 {
		t.Fatalf("first result = %d, want 1", got)
	}
	if got := pool.Stat().AcquiredConns(); got != 0 {
		t.Fatalf("%d connection(s) still acquired after the batch was read", got)
	}
}

func TestBatchFailureNamesStatementIndexWithoutArgsAndReleasesConnection(t *testing.T) {
	pool := openPool(t)
	d := New(pool).(momo.Batcher)
	secret := "s3cret-value-xyz"
	out, err := d.SendBatch(context.Background(), []momo.Compiled{
		{SQL: "SELECT 1"},
		{SQL: "SELECT $1::int", Args: []any{secret}},
		{SQL: "SELECT 3"},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if out != nil {
		t.Fatalf("a failed batch must return no Rows, got %d", len(out))
	}
	if !strings.Contains(err.Error(), "statement 1") {
		t.Fatalf("error does not name the failing statement index: %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaks an argument: %v", err)
	}
	if got := pool.Stat().AcquiredConns(); got != 0 {
		t.Fatalf("%d connection(s) leaked by the failed batch", got)
	}
}

func TestBatchEmptyIsNoop(t *testing.T) {
	d := New(openPool(t)).(momo.Batcher)
	out, err := d.SendBatch(context.Background(), nil)
	if err != nil || len(out) != 0 {
		t.Fatalf("out=%v err=%v", out, err)
	}
}

func TestCopyErrorsAreClassifiedAndHideValues(t *testing.T) {
	d := New(openPool(t))
	ctx := context.Background()
	if _, err := d.Exec(ctx, "CREATE TEMP TABLE t_copy_err (id bigint PRIMARY KEY, name text)", nil); err != nil {
		t.Fatal(err)
	}
	c := d.(momo.Copier)
	_, err := c.CopyFrom(ctx, "t_copy_err", []string{"id", "name"}, [][]any{{int64(1), "a"}, {int64(1), "b"}})
	if !errors.Is(err, momo.ErrUniqueViolation) {
		t.Fatalf("duplicate key err = %v, want ErrUniqueViolation", err)
	}
	secret := "not-a-number-s3cret"
	_, err = c.CopyFrom(ctx, "t_copy_err", []string{"id", "name"}, [][]any{{secret, "x"}})
	if err == nil {
		t.Fatal("expected an encode error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("copy error leaks a row value: %v", err)
	}
}

func TestCopyQuotesIdentifiersAndSplitsSchema(t *testing.T) {
	d := New(openPool(t))
	ctx := context.Background()
	if _, err := d.Exec(ctx, `CREATE TEMP TABLE "we ird""t" ("co l" bigint)`, nil); err != nil {
		t.Fatal(err)
	}
	c := d.(momo.Copier)
	n, err := c.CopyFrom(ctx, `we ird"t`, []string{"co l"}, [][]any{{int64(1)}})
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	// A hostile table name is an identifier, never SQL.
	if _, err := c.CopyFrom(ctx, `x; DROP TABLE we_ird`, []string{"a"}, [][]any{{int64(1)}}); err == nil {
		t.Fatal("expected an undefined-table error")
	}
	if _, err := d.Exec(ctx, "CREATE TEMP TABLE sq (id bigint)", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CopyFrom(ctx, "pg_temp.sq", []string{"id"}, [][]any{{int64(1)}}); err != nil {
		t.Fatalf("schema-qualified copy: %v", err)
	}
}

func TestCopyCancelStopsAndReleasesConnection(t *testing.T) {
	pool := openPool(t)
	d := New(pool)
	ctx0 := context.Background()
	if _, err := d.Exec(ctx0, "CREATE TABLE IF NOT EXISTS t_copy_cancel (id bigint)", nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.Exec(context.Background(), "DROP TABLE IF EXISTS t_copy_cancel", nil) })
	ctx, cancel := context.WithCancel(ctx0)
	cancel()
	rows := make([][]any, 1000)
	for i := range rows {
		rows[i] = []any{int64(i)}
	}
	_, err := d.(momo.Copier).CopyFrom(ctx, "t_copy_cancel", []string{"id"}, rows)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for pool.Stat().AcquiredConns() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := pool.Stat().AcquiredConns(); got != 0 {
		t.Fatalf("%d connection(s) still acquired after a cancelled copy", got)
	}
	rs, err := d.Query(ctx0, "SELECT count(*) FROM t_copy_cancel", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := readInt(t, rs); got != 0 {
		t.Fatalf("cancelled copy left %d rows", got)
	}
}

func TestCopyAndBatchOnPinnedConnectionUseThatSession(t *testing.T) {
	d := New(openPool(t))
	ctx := context.Background()
	p, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Release() }()
	if _, err := p.Exec(ctx, "CREATE TEMP TABLE t_pin_copy (id bigint)", nil); err != nil {
		t.Fatal(err)
	}
	c, ok := p.(momo.Copier)
	if !ok {
		t.Fatal("pinned connection is not a Copier")
	}
	if n, err := c.CopyFrom(ctx, "t_pin_copy", []string{"id"}, [][]any{{int64(1)}, {int64(2)}}); err != nil || n != 2 {
		t.Fatalf("n=%d err=%v (a TEMP table is only visible on the pinned session)", n, err)
	}
	b, ok := p.(momo.Batcher)
	if !ok {
		t.Fatal("pinned connection is not a Batcher")
	}
	out, err := b.SendBatch(ctx, []momo.Compiled{{SQL: "SELECT count(*) FROM t_pin_copy"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := readInt(t, out[0]); got != 2 {
		t.Fatalf("batch saw %d rows, want 2", got)
	}
}

func TestCapabilitiesReportCopyAndBatch(t *testing.T) {
	caps := New(openPool(t)).(momo.CapabilityReporter).Capabilities()
	if !caps.Copy || !caps.Batch {
		t.Fatalf("capabilities = %+v, want Copy and Batch", caps)
	}
}
