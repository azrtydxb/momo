// SPDX-License-Identifier: Apache-2.0

//go:build integration && cgo && !windows

package sqlitedriver

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
)

func TestSQLiteIsolationRules(t *testing.T) {
	db, drv, err := Open(testdb.SQLitePath(t), Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if _, err := drv.Begin(ctx, momo.TxOptions{Isolation: sql.LevelReadUncommitted}); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("read uncommitted err = %v, want ErrUnsupported", err)
	}
	tx, err := drv.Begin(ctx, momo.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteReadOnlyTransaction(t *testing.T) {
	db, drv, err := Open(testdb.SQLitePath(t), Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if _, err := drv.Exec(ctx, "CREATE TABLE ro (x int)", nil); err != nil {
		t.Fatal(err)
	}
	tx, err := drv.Begin(ctx, momo.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, "INSERT INTO ro VALUES (1)", nil)
	if err == nil || !strings.Contains(err.Error(), "readonly") {
		t.Fatalf("write inside read-only transaction err = %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := drv.Exec(ctx, "INSERT INTO ro VALUES (2)", nil); err != nil {
		t.Fatalf("query_only leaked to the pooled connection: %v", err)
	}
}

func TestSQLitePinnedReadOnlyTransaction(t *testing.T) {
	db, drv, err := Open(testdb.SQLitePath(t), Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if _, err := drv.Exec(ctx, "CREATE TABLE pro (x int)", nil); err != nil {
		t.Fatal(err)
	}
	p, err := drv.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(ctx, momo.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO pro VALUES (1)", nil); err == nil {
		t.Fatal("write inside read-only transaction succeeded")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "INSERT INTO pro VALUES (2)", nil); err != nil {
		t.Fatalf("query_only leaked to the pinned connection: %v", err)
	}
	if err := p.Release(); err != nil {
		t.Fatal(err)
	}
}
