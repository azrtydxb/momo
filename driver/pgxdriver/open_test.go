// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// unreachableURL names a server nothing listens on: port 1 refuses at once.
const unreachableURL = "postgres://u@127.0.0.1:1/db"

// Catches: Open or OpenSQL succeeding without a server, which hides a bad DSN
// until the first statement, and returning a driver next to the error.
func TestOpenPingsServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if d, err := Open(ctx, unreachableURL); err == nil {
		_ = d.(io.Closer).Close()
		t.Fatal("Open succeeded without a server")
	} else if d != nil {
		t.Fatalf("Open returned %T with an error", d)
	}
	if d, err := OpenSQL(ctx, unreachableURL); err == nil {
		_ = d.(io.Closer).Close()
		t.Fatal("OpenSQL succeeded without a server")
	} else if d != nil {
		t.Fatalf("OpenSQL returned %T with an error", d)
	}
	if d, err := Open(ctx, "://bad"); err == nil {
		t.Fatalf("Open accepted a bad DSN and returned %T", d)
	}
	if d, err := OpenSQL(ctx, "://bad"); err == nil {
		t.Fatalf("OpenSQL accepted a bad DSN and returned %T", d)
	}
}

// Catches: New returning a driver that closes a pool the caller owns.
func TestNewDoesNotOwnPool(t *testing.T) {
	if _, ok := New(nil).(io.Closer); ok {
		t.Fatal("New returned an io.Closer: a wrapped pool belongs to the caller")
	}
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

// Catches: OpenSQL dropping WithExecMode, so a program behind PgBouncer keeps
// pgx's statement-caching default, and options of the native driver that a
// database/sql pool cannot take being silently ignored.
func TestSQLOptions(t *testing.T) {
	mode, set, err := sqlOptions([]Option{WithExecMode(pgx.QueryExecModeExec)})
	if err != nil || !set || mode != pgx.QueryExecModeExec {
		t.Fatalf("sqlOptions(WithExecMode(Exec)) = %v, %v, %v", mode, set, err)
	}
	if _, set, err := sqlOptions(nil); err != nil || set {
		t.Fatalf("sqlOptions() = set %v, err %v, want neither", set, err)
	}
	if _, _, err := sqlOptions([]Option{WithExecMode(pgx.QueryExecModeSimpleProtocol)}); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("simple protocol err = %v, want ErrUnsupported", err)
	}
	if _, _, err := sqlOptions([]Option{WithVectorFormat(VectorBinary)}); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("WithVectorFormat err = %v, want ErrValidation", err)
	}
	ctx := context.Background()
	if _, err := OpenSQL(ctx, unreachableURL, WithVectorFormat(VectorBinary)); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("OpenSQL(WithVectorFormat) err = %v, want ErrValidation", err)
	}
	if _, err := OpenSQL(ctx, unreachableURL, WithExecMode(pgx.QueryExecModeSimpleProtocol)); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("OpenSQL(simple protocol) err = %v, want ErrUnsupported", err)
	}
	if _, err := OpenSQL(ctx, unreachableURL+"?default_query_exec_mode=simple_protocol"); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("OpenSQL(simple protocol DSN) err = %v, want ErrUnsupported", err)
	}
	cfg, err := pgx.ParseConfig(unreachableURL)
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*cfg)
	defer func() { _ = db.Close() }()
	for name, d := range map[string]momo.Driver{
		"WithExecMode":     NewSQL(db, WithExecMode(pgx.QueryExecModeExec)),
		"WithVectorFormat": NewSQL(db, WithVectorFormat(VectorBinary)),
	} {
		if _, err := d.Exec(ctx, "SELECT 1", nil); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("NewSQL(%s) Exec err = %v, want ErrValidation", name, err)
		}
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
