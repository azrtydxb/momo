// SPDX-License-Identifier: Apache-2.0

//go:build integration

package migrate_test

import (
	"context"
	"testing"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

func TestMigrationRoundTrip(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	desired := docsSchema()
	m := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
	mig, err := m.Diff(ctx, desired, "init", migrate.DiffOptions{})
	if err != nil || mig == nil {
		t.Fatalf("diff: %v %v", mig, err)
	}
	fsys := migratetest.FS(mig.Files)
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	inspected, err := e.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !schema.Equal(inspected, desired) {
		t.Fatalf("round trip not empty\n%s", migratetest.Diff(inspected, desired))
	}
	again, err := e.Diff(ctx, desired, "noop", migrate.DiffOptions{})
	if err != nil || again != nil {
		t.Fatalf("no-change diff must write nothing: %v %v", again, err)
	}
}

func TestNoChangeWritesNothing(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	m := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
	mig, err := m.Diff(ctx, docsSchema(), "init", migrate.DiffOptions{})
	if err != nil || mig == nil {
		t.Fatalf("diff: %v %v", mig, err)
	}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(mig.Files))
	for i := 0; i < 2; i++ {
		again, err := e.Diff(ctx, docsSchema(), "noop", migrate.DiffOptions{})
		if err != nil || again != nil {
			t.Fatalf("diff %d must write nothing: %v %v", i, again, err)
		}
	}
}

// TestInspectIgnoresHistoryTable catches an engine Inspect that reports the
// momo_migrations history table, which would show up as drift or a DROP TABLE.
func TestInspectIgnoresHistoryTable(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	m := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
	mig, err := m.Diff(ctx, docsSchema(), "init", migrate.DiffOptions{})
	if err != nil || mig == nil {
		t.Fatalf("diff: %v %v", mig, err)
	}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(mig.Files))
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if !tableExists(t, db, "momo_migrations") {
		t.Fatal("setup: history table must exist")
	}
	s, err := e.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tb := range s.Tables {
		if tb.Name == "momo_migrations" {
			t.Fatalf("Inspect must not report the history table: %+v", s.Tables)
		}
	}
	// Diffing the live schema against an empty snapshot must not propose to
	// drop the history table either.
	changes, _, err := migrate.Diff(s, &schema.Schema{Dialect: "postgres"}, migrate.DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range changes {
		if c.Kind == migrate.DropTable && c.Table == "momo_migrations" {
			t.Fatalf("history table proposed for drop: %+v", c)
		}
	}
}
