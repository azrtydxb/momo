// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
)

func TestNonTransactionalFailureRecovery(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := migratetest.WithSum(t, fstest.MapFS{
		"20260101000000_bad.up.sql": {Data: []byte("-- momo:no-transaction\nCREATE TABLE ok1 (x int);\nCREATE TABLE ok2 (x int);\nINSERT INTO missing VALUES (1);")},
	})
	e := migratetest.NewEngine(t, migratetest.Postgres(), db.Driver(), fsys)
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil {
		t.Fatal("apply must fail")
	}
	row := historyRow(t, db, "20260101000000")
	if row.Status != migrate.StatusFailed || row.StatementsApplied != 2 || row.Error == "" {
		t.Fatalf("want failed with statements_applied=2, got %+v", row)
	}
	if row.Checksum == "" || row.Name != "bad" || row.Kind != "sql" || row.ApplyID == "" {
		t.Fatalf("history row lacks checksum, name, kind or apply id: %+v", row)
	}
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); !errors.Is(err, migrate.ErrDirty) {
		t.Fatalf("apply must refuse while dirty, got %v", err)
	}
	if err := e.Resolve(ctx, "20260101000000", migrate.ResolveRolledBack); err != nil {
		t.Fatal(err)
	}
	plan, err := e.Plan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 1 {
		t.Fatalf("after resolve the version is pending again, plan = %+v", plan)
	}
}

func TestResolveApplied(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := migratetest.WithSum(t, fstest.MapFS{
		"20260101000000_bad.up.sql": {Data: []byte("-- momo:no-transaction\nCREATE TABLE ok1 (x int);\nINSERT INTO missing VALUES (1);")},
	})
	e := migratetest.NewEngine(t, migratetest.Postgres(), db.Driver(), fsys)
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil {
		t.Fatal("apply must fail")
	}
	if err := e.Resolve(ctx, "20260101000000", migrate.ResolveApplied); err != nil {
		t.Fatal(err)
	}
	row := historyRow(t, db, "20260101000000")
	if row.Status != migrate.StatusApplied || row.StatementsApplied != 1 {
		t.Fatalf("want applied keeping statements_applied=1, got %+v", row)
	}
	if err := e.Resolve(ctx, "20260101000000", migrate.ResolveApplied); err == nil {
		t.Fatal("resolving a version without a dirty row must fail")
	}
	if plan, err := e.Plan(ctx); err != nil || len(plan) != 0 {
		t.Fatalf("nothing pending after resolve applied: %+v %v", plan, err)
	}
}

func TestTransactionalFailureRolledBack(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	bad := migratetest.WithSum(t, fstest.MapFS{
		"20260101000000_t.up.sql": {Data: []byte("CREATE TABLE a (x int);\nINSERT INTO missing VALUES (1);")},
	})
	e := migratetest.NewEngine(t, migratetest.Postgres(), db.Driver(), bad)
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil {
		t.Fatal("apply must fail")
	}
	if tableExists(t, db, "a") {
		t.Fatal("transactional failure must roll back the DDL")
	}
	row := historyRow(t, db, "20260101000000")
	if row.Status != migrate.StatusRolledBack || row.Error == "" {
		t.Fatalf("want rolled_back with the error text, got %+v", row)
	}
	fixed := migratetest.WithSum(t, fstest.MapFS{
		"20260101000000_t.up.sql": {Data: []byte("CREATE TABLE a (x int);")},
	})
	e2 := migratetest.NewEngine(t, migratetest.Postgres(), db.Driver(), fixed)
	if _, err := e2.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatalf("a rolled-back transactional migration must apply again without resolve: %v", err)
	}
	if !tableExists(t, db, "a") {
		t.Fatal("the re-applied migration did not create a")
	}
	row = historyRow(t, db, "20260101000000")
	if row.Status != migrate.StatusApplied || row.StatementsApplied != 1 || row.Checksum == "" || row.Error != "" {
		t.Fatalf("re-applied row wrong: %+v", row)
	}
}

func TestNewEngineRequiresDialect(t *testing.T) {
	_, err := migrate.NewEngine(nil, fstest.MapFS{})
	if err == nil || !strings.Contains(err.Error(), "migrate.WithDialect") {
		t.Fatalf("want an error naming migrate.WithDialect, got %v", err)
	}
}
