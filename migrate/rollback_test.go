// SPDX-License-Identifier: Apache-2.0

//go:build integration

package migrate_test

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
)

func TestRollbackGroup(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := migratetest.WithSum(t, fstest.MapFS{
		"20260101000000_a.up.sql":   {Data: []byte("CREATE TABLE a (x int);")},
		"20260101000000_a.down.sql": {Data: []byte("DROP TABLE a;")},
		"20260101000001_b.up.sql":   {Data: []byte("CREATE TABLE b (x int);")},
		"20260101000001_b.down.sql": {Data: []byte("DROP TABLE b;")},
	})
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	rep, err := e.Rollback(ctx, migrate.RollbackOptions{Group: true})
	if err != nil {
		t.Fatal(err)
	}
	if tableExists(t, db, "a") || tableExists(t, db, "b") {
		t.Fatal("group rollback must drop both tables")
	}
	if len(rep.Applied) != 2 || rep.Applied[0].Version != "20260101000001" || rep.Applied[1].Version != "20260101000000" {
		t.Fatalf("rollback must run in reverse version order, got %+v", rep.Applied)
	}
	for _, v := range []string{"20260101000000", "20260101000001"} {
		if got := historyRow(t, db, v).Status; got != migrate.StatusRolledBack {
			t.Fatalf("version %s status = %s, want rolled_back", v, got)
		}
	}
	// A rolled back version is pending again and applies cleanly.
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatalf("re-apply after rollback: %v", err)
	}
	if !tableExists(t, db, "a") || !tableExists(t, db, "b") {
		t.Fatal("re-apply must recreate both tables")
	}
}

func TestRollbackSingleKeepsEarlierMigrations(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := migratetest.WithSum(t, fstest.MapFS{
		"20260101000000_a.up.sql":   {Data: []byte("CREATE TABLE a (x int);")},
		"20260101000000_a.down.sql": {Data: []byte("DROP TABLE a;")},
		"20260101000001_b.up.sql":   {Data: []byte("CREATE TABLE b (x int);")},
		"20260101000001_b.down.sql": {Data: []byte("DROP TABLE b;")},
	})
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Rollback(ctx, migrate.RollbackOptions{}); err != nil {
		t.Fatal(err)
	}
	if !tableExists(t, db, "a") || tableExists(t, db, "b") {
		t.Fatal("a single rollback must drop only the last migration")
	}
}

func TestRollbackNoDown(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := migratetest.WithSum(t, fstest.MapFS{"20260101000000_a.up.sql": {Data: []byte("CREATE TABLE a (x int);")}})
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Rollback(ctx, migrate.RollbackOptions{}); !errors.Is(err, migrate.ErrNoDown) {
		t.Fatalf("want ErrNoDown, got %v", err)
	}
	if !tableExists(t, db, "a") {
		t.Fatal("nothing may run when a down file is missing")
	}
}

// A group with one missing down file runs none of its downs.
func TestRollbackGroupMissingDownRunsNothing(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := migratetest.WithSum(t, fstest.MapFS{
		"20260101000000_a.up.sql":   {Data: []byte("CREATE TABLE a (x int);")},
		"20260101000000_a.down.sql": {Data: []byte("DROP TABLE a;")},
		"20260101000001_b.up.sql":   {Data: []byte("CREATE TABLE b (x int);")},
	})
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Rollback(ctx, migrate.RollbackOptions{Group: true}); !errors.Is(err, migrate.ErrNoDown) {
		t.Fatalf("want ErrNoDown, got %v", err)
	}
	if !tableExists(t, db, "a") || !tableExists(t, db, "b") {
		t.Fatal("nothing may run when any down file of the group is missing")
	}
}

func TestRollbackNothingApplied(t *testing.T) {
	db := testopen.Postgres(t)
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.WithSum(t, fstest.MapFS{}))
	rep, err := e.Rollback(context.Background(), migrate.RollbackOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Applied) != 0 {
		t.Fatalf("report = %+v, want empty", rep)
	}
}
