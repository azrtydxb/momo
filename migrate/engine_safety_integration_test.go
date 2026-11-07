// SPDX-License-Identifier: Apache-2.0

//go:build integration

package migrate_test

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"

	testdb "github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
)

// An apply with nothing pending must not create the history table or its
// metadata row; one with a pending migration creates it first.
func TestApplyNothingPendingCreatesNoHistoryTable(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if tableExists(t, db, testdb.Schema(t)+".momo_migrations") {
		t.Fatal("a no-op apply created momo_migrations")
	}
	e = migratetest.NewEngine(t, mpg.New(), db.Driver(), twoVersions())
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if !tableExists(t, db, testdb.Schema(t)+".momo_migrations") || appliedCount(t, db) != 2 {
		t.Fatal("an apply with pending migrations must create the history table and record them")
	}
}

func TestApplyRefusesOutOfOrderOnPostgres(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	newer := fstest.MapFS{"20260103000000_new.up.sql": {Data: []byte("CREATE TABLE newer (x int);")}}
	if _, err := migratetest.NewEngine(t, mpg.New(), db.Driver(), newer).Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	both := fstest.MapFS{
		"20260102000000_late.up.sql": {Data: []byte("CREATE TABLE late (x int);")},
		"20260103000000_new.up.sql":  newer["20260103000000_new.up.sql"],
	}
	_, err := migratetest.NewEngine(t, mpg.New(), db.Driver(), both).Apply(ctx, migrate.ApplyOptions{})
	if !errors.Is(err, migrate.ErrOutOfOrder) {
		t.Fatalf("want ErrOutOfOrder, got %v", err)
	}
	if tableExists(t, db, testdb.Schema(t)+".late") {
		t.Fatal("the out-of-order migration ran")
	}
}

// Rehash against the real history table: only the named version's row changes.
func TestRehashOnPostgres(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	const v1, v2 = "20260101000000", "20260101000001"
	fsys := twoVersions()
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	before1, before2 := historyRow(t, db, v1), historyRow(t, db, v2)
	fsys["20260101000000_a.up.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE a (x int); -- reformatted")}
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); !errors.Is(err, migrate.ErrChecksumMismatch) {
		t.Fatalf("want ErrChecksumMismatch, got %v", err)
	}
	dry, err := e.Rehash(ctx, v1, false)
	if err != nil || !dry.Changed || dry.Accepted {
		t.Fatalf("dry run = %+v, %v", dry, err)
	}
	if got := historyRow(t, db, v1); got.Checksum != before1.Checksum {
		t.Fatalf("a dry run changed the checksum: %+v", got)
	}
	r, err := e.Rehash(ctx, v1, true)
	if err != nil || !r.Accepted {
		t.Fatalf("rehash = %+v, %v", r, err)
	}
	got1, got2 := historyRow(t, db, v1), historyRow(t, db, v2)
	if got1.Checksum != r.Current || got1.Checksum == before1.Checksum {
		t.Fatalf("v1 checksum not updated: %+v", got1)
	}
	if got2 != before2 {
		t.Fatalf("v2 row changed: %+v -> %+v", before2, got2)
	}
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatalf("apply after rehash: %v", err)
	}
}

// An explicit 0 turns the session timeouts off on the server.
func TestSessionTimeoutZeroOnPostgres(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	check := `DO $$ BEGIN IF current_setting('statement_timeout') <> '0' OR current_setting('lock_timeout') <> '0' THEN RAISE EXCEPTION 'timeouts are %, %', current_setting('statement_timeout'), current_setting('lock_timeout'); END IF; END $$;`
	fsys := fstest.MapFS{"20260101000000_check.up.sql": {Data: []byte(check)}}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys, migrate.WithLockTimeout(0), migrate.WithStatementTimeout(0))
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
}
