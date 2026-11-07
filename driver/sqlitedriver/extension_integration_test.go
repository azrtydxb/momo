// SPDX-License-Identifier: Apache-2.0

//go:build integration

package sqlitedriver

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/internal/testdb"
)

// TestSQLiteNoLoadExtension is S-45: extension loading is refused on every
// connection, while sqlite-vec, registered at build time, still works.
func TestSQLiteNoLoadExtension(t *testing.T) {
	db, _, err := Open(testdb.SQLitePath(t), Config{MaxOpenConns: 2})
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
	for i, c := range []*sql.Conn{c1, c2} {
		_, err := c.ExecContext(ctx, "SELECT load_extension('momo_probe')")
		if err == nil || !strings.Contains(err.Error(), "not authorized") {
			t.Fatalf("connection %d: load_extension error = %v, want SQLite's disabled message", i, err)
		}
		var v string
		if err := c.QueryRowContext(ctx, "SELECT vec_version()").Scan(&v); err != nil {
			t.Fatalf("connection %d: sqlite-vec unavailable: %v", i, err)
		}
	}
}
