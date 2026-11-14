// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo"
	testdb "github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
)

// queryOne runs sql on a pinned connection and releases it before returning,
// so repeated calls never hold the pool.
func queryOne(t *testing.T, db *momo.DB, sql string, dest ...any) {
	t.Helper()
	p, ok := db.Driver().(momo.Pinner)
	if !ok {
		t.Fatal("driver cannot pin")
	}
	c, err := p.Pin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Release() }()
	mustQueryRow(t, c, sql, dest...)
}

func tableExistsIn(t *testing.T, db *momo.DB, schemaName, table string) bool {
	t.Helper()
	var ok bool
	queryOne(t, db, `SELECT to_regclass('"`+schemaName+`"."`+table+`"') IS NOT NULL`, &ok)
	return ok
}

// TestCreateIndexConcurrentlyApply applies a CREATE INDEX CONCURRENTLY
// migration marked no-transaction: the statement must run outside a
// transaction (PostgreSQL rejects it inside one), leave a valid index, and
// record the version as applied in the history.
func TestCreateIndexConcurrentlyApply(t *testing.T) {
	db := concurrentIndexDB(t)
	ctx := context.Background()
	fsys := migratetest.WithSum(t, fstest.MapFS{
		"20260101000000_a.up.sql":   {Data: []byte("CREATE TABLE d (id bigint PRIMARY KEY, title text);")},
		"20260101000001_idx.up.sql": {Data: []byte("-- momo:no-transaction\nCREATE INDEX CONCURRENTLY d_title ON d (title);")},
	})
	e := engineIn(t, db, concurrentIndexSchema, fsys)
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	var valid bool
	queryOne(t, db, `SELECT indisvalid FROM pg_index WHERE indexrelid = to_regclass(quote_ident('`+concurrentIndexSchema+`')||'.d_title')`, &valid)
	if !valid {
		t.Fatal("concurrent index must be valid")
	}
	var state string
	queryOne(t, db, `SELECT status FROM `+concurrentIndexSchema+`.momo_migrations WHERE version = '20260101000001'`, &state)
	if state != "applied" {
		t.Fatalf("history status = %q, want applied", state)
	}
}

// TestPostgresApplyRecovery proves a failing transactional migration rolls its
// DDL back and is not recorded as applied.
func TestPostgresApplyRecovery(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := migratetest.WithSum(t, fstest.MapFS{
		"20260101000000_bad.up.sql": {Data: []byte("CREATE TABLE ok (x int);\nINSERT INTO missing VALUES (1);")},
	})
	e := engineOn(t, db, fsys)
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil {
		t.Fatal("apply must fail")
	}
	if tableExistsIn(t, db, testdb.Schema(t), "ok") {
		t.Fatal("transactional PostgreSQL migration must roll back DDL")
	}
	var n int
	queryOne(t, db, `SELECT count(*) FROM `+testdb.Schema(t)+`.momo_migrations WHERE status = 'applied'`, &n)
	if n != 0 {
		t.Fatalf("failed migration recorded as applied (%d rows)", n)
	}
}
