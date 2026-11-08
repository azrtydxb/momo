// SPDX-License-Identifier: Apache-2.0

//go:build integration && cgo && !windows

package driver_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/sqlitedriver"
	"github.com/azrtydxb/momo/internal/testdb"
)

func TestRoundTripSQLite(t *testing.T) {
	db, d, err := sqlitedriver.Open(testdb.SQLitePath(t), sqlitedriver.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if _, err := d.Exec(ctx, "CREATE TABLE r (id integer primary key, v text)", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(ctx, "INSERT INTO r (v) VALUES (?)", []any{"x"}); err != nil {
		t.Fatal(err)
	}
	rows, err := d.Query(ctx, "SELECT v FROM r", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("no row")
	}
	var v string
	if err := rows.Scan(&v); err != nil || v != "x" {
		t.Fatalf("v=%q err=%v", v, err)
	}
	if momo.New(d) == nil {
		t.Fatal("momo.New returned nil")
	}
}
