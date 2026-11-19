// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// unreachableURL names a server nothing listens on. Neither Open nor OpenSQL
// dials, so it needs no server.
const unreachableURL = "postgres://u@127.0.0.1:1/db"

// Catches: Open returning a driver nothing can close, which leaks the pool it
// opened, and New returning one that closes a pool the caller owns.
func TestOpenOwnsPoolAndNewDoesNot(t *testing.T) {
	drv, err := Open(context.Background(), unreachableURL)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := drv.(io.Closer)
	if !ok {
		t.Fatalf("Open returned %T, which is not an io.Closer", drv)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, ok := New(nil).(io.Closer); ok {
		t.Fatal("New returned an io.Closer: a wrapped pool belongs to the caller")
	}
}

// Catches: Open keeping the options of the native driver out of the driver it
// builds, and a bad DSN being accepted.
func TestOpenAppliesOptionsAndRejectsBadDSN(t *testing.T) {
	drv, err := Open(context.Background(), unreachableURL, WithExecMode(pgx.QueryExecModeSimpleProtocol))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = drv.(io.Closer).Close() }()
	if _, err := drv.Exec(context.Background(), "SELECT 1", nil); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("Exec with simple protocol err = %v, want ErrUnsupported", err)
	}
	if d, err := Open(context.Background(), "://bad"); err == nil {
		t.Fatalf("Open accepted a bad DSN and returned %T", d)
	}
}

// Catches: OpenSQL not owning its database, a DSN that selects simple
// protocol being accepted, and a bad DSN being accepted.
func TestOpenSQL(t *testing.T) {
	ctx := context.Background()
	drv, err := OpenSQL(ctx, unreachableURL+"?default_query_exec_mode=exec")
	if err != nil {
		t.Fatal(err)
	}
	if err := drv.(io.Closer).Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	simple, err := OpenSQL(ctx, unreachableURL+"?default_query_exec_mode=simple_protocol")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = simple.(io.Closer).Close() }()
	if _, err := simple.Exec(ctx, "SELECT 1", nil); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("Exec with simple protocol err = %v, want ErrUnsupported", err)
	}
	if d, err := OpenSQL(ctx, "://bad"); err == nil {
		t.Fatalf("OpenSQL accepted a bad DSN and returned %T", d)
	}
}

// Catches: NewSQL taking over a database it was handed. The caller closes it.
func TestNewSQLDoesNotOwnDB(t *testing.T) {
	cfg, err := pgx.ParseConfig(unreachableURL)
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*cfg)
	defer func() { _ = db.Close() }()
	if _, ok := NewSQL(db).(io.Closer); ok {
		t.Fatal("NewSQL returned an io.Closer: a wrapped database belongs to the caller")
	}
}

// Catches: the deprecated names drifting from their replacements.
func TestDeprecatedForwarders(t *testing.T) {
	cfg, err := pgx.ParseConfig(unreachableURL)
	if err != nil {
		t.Fatal(err)
	}
	db, drv := FromConnConfig(cfg)
	defer func() { _ = db.Close() }()
	if drv == nil {
		t.Fatal("FromConnConfig returned a nil driver")
	}
	if _, ok := FromSQLDB(nil).(momo.ServerVersioner); !ok {
		t.Fatal("FromSQLDB(nil) is not the driver NewSQL(nil) returns")
	}
}
