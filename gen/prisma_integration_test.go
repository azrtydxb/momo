// SPDX-License-Identifier: Apache-2.0

//go:build integration

package gen_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/gen/naming"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

// prismaDDL is what prisma migrate creates for the models in the prisma
// fixture, written by hand: quoted PascalCase tables, camelCase columns,
// TIMESTAMP(3), and the <Table>_pkey, _key, _idx and _fkey names.
var prismaDDL = []string{
	`CREATE TABLE "Account" (
		"id" TEXT NOT NULL,
		"email" TEXT NOT NULL,
		"createdAt" TIMESTAMP(3) NOT NULL,
		"updatedAt" TIMESTAMP(3) NOT NULL,
		CONSTRAINT "Account_pkey" PRIMARY KEY ("id")
	)`,
	`CREATE TABLE "KbChunk" (
		"id" TEXT NOT NULL,
		"accountId" TEXT NOT NULL,
		"body" TEXT NOT NULL,
		"createdAt" TIMESTAMP(3) NOT NULL,
		"deletedAt" TIMESTAMP(3),
		CONSTRAINT "KbChunk_pkey" PRIMARY KEY ("id")
	)`,
	`CREATE UNIQUE INDEX "Account_email_key" ON "Account"("email")`,
	`CREATE INDEX "KbChunk_accountId_idx" ON "KbChunk"("accountId")`,
	`ALTER TABLE "KbChunk" ADD CONSTRAINT "KbChunk_accountId_fkey" FOREIGN KEY ("accountId") REFERENCES "Account"("id") ON DELETE RESTRICT ON UPDATE CASCADE`,
}

func prismaModels(t *testing.T, typeDefaults map[string]string) *schema.Schema {
	t.Helper()
	dir, err := filepath.Abs("testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	cfg := gen.Config{Dir: dir, Naming: naming.Prisma{}, TypeDefaults: typeDefaults, Dialects: []dialect.Name{dialect.Postgres}}
	g, diags, err := gen.Load([]string{"./prisma"}, cfg)
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	return g.Schema
}

// TestPrismaDatabaseDiffsClean catches a preset or type default that does not
// match a database Prisma created: the live schema is inspected and diffed
// against the models, and any difference in a table, column, type, key or
// index name is a change. The second half proves the diff is sensitive, by
// showing the default timestamptz does produce changes.
func TestPrismaDatabaseDiffsClean(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	for _, stmt := range prismaDDL {
		if _, err := db.Driver().Exec(ctx, stmt, nil); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
	live, err := e.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}

	desired := prismaModels(t, map[string]string{"time.Time": "timestamp(3)"})
	// The test databases always have the vector extension; the models do not
	// manage extensions, so take the live ones to keep them out of the diff.
	desired.Extensions = live.Extensions
	changes, _, err := migrate.Diff(live, desired, migrate.DiffOptions{})
	if err != nil || len(changes) != 0 {
		t.Fatalf("a Prisma database must diff clean against the prisma preset: %v\n%v\n%s", changes, err, migratetest.Diff(live, desired))
	}

	withoutDefault := prismaModels(t, nil)
	withoutDefault.Extensions = live.Extensions
	changes, _, err = migrate.Diff(live, withoutDefault, migrate.DiffOptions{AllowDestructive: true})
	if err != nil || len(changes) == 0 {
		t.Fatalf("timestamptz models must differ from timestamp(3) columns: %v %v", changes, err)
	}
}
