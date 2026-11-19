// SPDX-License-Identifier: Apache-2.0

//go:build integration

package pgxdriver

import (
	"context"
	"io"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// Catches: DB.Close leaving the pool of an Open driver open, closing the pool
// of a New driver, and a second Close failing.
func TestDBCloseOwnership(t *testing.T) {
	ctx := context.Background()
	url := testdb.PostgresURL(t)
	drv, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	db := momo.New(drv)
	if _, err := momo.Exec(ctx, db, "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := momo.Exec(ctx, db, "SELECT 1"); err == nil {
		t.Fatal("a statement ran after Close of an Open driver")
	}

	pool, err := NewPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	wrapped := momo.New(New(pool))
	if err := wrapped.Close(); err != nil {
		t.Fatal(err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("Close of a New driver closed the caller's pool: %v", err)
	}
}

// Catches: OpenSQL taking the exec mode from the DSN default and ignoring
// WithExecMode, and an OpenSQL driver whose Close leaves the database open.
func TestOpenSQLAppliesExecMode(t *testing.T) {
	ctx := context.Background()
	drv, err := OpenSQL(ctx, testdb.PostgresURL(t), WithExecMode(pgx.QueryExecModeExec))
	if err != nil {
		t.Fatal(err)
	}
	o := drv.(*ownedSQLDriver)
	conn, err := o.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var got pgx.QueryExecMode
	if err := conn.Raw(func(dc any) error {
		got = dc.(*stdlib.Conn).Conn().Config().DefaultQueryExecMode
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if got != pgx.QueryExecModeExec {
		t.Fatalf("exec mode = %v, want QueryExecModeExec", got)
	}
	if _, err := drv.Exec(ctx, "SELECT $1::int", []any{1}); err != nil {
		t.Fatal(err)
	}
	if err := drv.(io.Closer).Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := drv.Exec(ctx, "SELECT 1", nil); err == nil {
		t.Fatal("a statement ran after Close of an OpenSQL driver")
	}
}

// Catches: Open leaving a pool behind when the server refuses the connection.
func TestOpenFailureLeavesNothingOpen(t *testing.T) {
	ctx := context.Background()
	url := unreachableURL
	if d, err := Open(ctx, url); err == nil {
		_ = d.(io.Closer).Close()
		t.Fatal("Open succeeded against a closed port")
	} else if d != nil {
		t.Fatalf("Open returned %T with an error", d)
	}
	if d, err := OpenSQL(ctx, url); err == nil {
		_ = d.(io.Closer).Close()
		t.Fatal("OpenSQL succeeded against a closed port")
	}
}
