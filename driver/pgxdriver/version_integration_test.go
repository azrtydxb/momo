// SPDX-License-Identifier: Apache-2.0

//go:build integration

package pgxdriver

import (
	"context"
	"database/sql"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
)

func TestServerVersion(t *testing.T) {
	pool := openPool(t)
	ctx := context.Background()
	d := New(pool)
	got, err := d.(momo.ServerVersioner).ServerVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var want int
	if err := pool.QueryRow(ctx, "SELECT current_setting('server_version_num')::int").Scan(&want); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("ServerVersion = %d, want %d", got, want)
	}
	if got < 150000 {
		t.Fatalf("server %d is below PostgreSQL 15; MERGE is unavailable", got)
	}
}

func TestServerVersionSQLDB(t *testing.T) {
	db, err := sql.Open("pgx", testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	got, err := FromSQLDB(db).(momo.ServerVersioner).ServerVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var want int
	if err := db.QueryRowContext(ctx, "SELECT current_setting('server_version_num')::int").Scan(&want); err != nil {
		t.Fatal(err)
	}
	if got != want || got < 150000 {
		t.Fatalf("ServerVersion = %d, want %d (>= 150000)", got, want)
	}
}
