// SPDX-License-Identifier: Apache-2.0

//go:build integration

package gen_test

import (
	"context"
	"path/filepath"
	"slices"
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

// prismaDDL is hand-written Prisma-style DDL for the models in the prisma
// fixture (not captured from a real prisma migrate run): quoted PascalCase tables, camelCase columns,
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

	// Control 1: without the type default the only differences are the four
	// DateTime columns, altered from timestamp(3) to timestamptz.
	withoutDefault := prismaModels(t, nil)
	withoutDefault.Extensions = live.Extensions
	changes, _, err = migrate.Diff(live, withoutDefault, migrate.DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	wantAlters := []string{
		"alter_column Account.createdAt", "alter_column Account.updatedAt",
		"alter_column KbChunk.createdAt", "alter_column KbChunk.deletedAt",
	}
	if got := summarize(changes); !slices.Equal(got, wantAlters) {
		t.Fatalf("type-default control: changes=%v want %v", got, wantAlters)
	}

	// Control 2: a constraint or index whose name differs from the Prisma
	// name is detected, as a drop of the wrong name and a create of the right
	// one.
	for _, stmt := range []string{
		`ALTER INDEX "KbChunk_accountId_idx" RENAME TO "kb_chunk_account_id_idx"`,
		`ALTER TABLE "KbChunk" RENAME CONSTRAINT "KbChunk_accountId_fkey" TO "kb_chunk_account_id_fkey"`,
	} {
		if _, err := db.Driver().Exec(ctx, stmt, nil); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	renamed, err := e.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	desired.Extensions = renamed.Extensions
	changes, _, err = migrate.Diff(renamed, desired, migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{
		"add_fk KbChunk.KbChunk_accountId_fkey", "create_index KbChunk.KbChunk_accountId_idx",
		"drop_fk KbChunk.kb_chunk_account_id_fkey", "drop_index KbChunk.kb_chunk_account_id_idx",
	}
	if got := summarize(changes); !slices.Equal(got, wantNames) {
		t.Fatalf("name control: changes=%v want %v", got, wantNames)
	}
}

// summarize returns the sorted "kind table.name" of each change.
func summarize(changes []migrate.Change) []string {
	out := make([]string, 0, len(changes))
	for _, c := range changes {
		out = append(out, string(c.Kind)+" "+c.Table+"."+c.Name)
	}
	slices.Sort(out)
	return out
}
