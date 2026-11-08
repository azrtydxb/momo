// SPDX-License-Identifier: Apache-2.0

//go:build integration && cgo && !windows

package sqlitedriver

import (
	"context"
	"testing"
)

// TestSharedCacheMemoryAcrossConnections: with cache=shared, the connections
// of one pool see the same in-memory database. The name is unique per test, so
// parallel packages cannot meet in it.
func TestSharedCacheMemoryAcrossConnections(t *testing.T) {
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, _, err := Open(dsn, Config{MaxOpenConns: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	c1, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer c1.Close()
	c2, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	if _, err := c1.ExecContext(ctx, "CREATE TABLE shared_t (x int)"); err != nil {
		t.Fatal(err)
	}
	if _, err := c1.ExecContext(ctx, "INSERT INTO shared_t VALUES (7)"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := c2.QueryRowContext(ctx, "SELECT x FROM shared_t").Scan(&n); err != nil || n != 7 {
		t.Fatalf("second connection reads %d, %v; it does not share the database", n, err)
	}
}
