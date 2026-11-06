// SPDX-License-Identifier: Apache-2.0

//go:build integration

package migrate_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

// TestSplitPairAppliesLockFree applies the head and no-transaction tail that
// diff writes for a concurrent index, an added UNIQUE and an added CHECK on an
// existing table, and proves the result matches the models: the index and the
// unique are valid and the check is validated.
func TestSplitPairAppliesLockFree(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	first := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
	m1, err := first.Diff(ctx, docsSchema(), "init", migrate.DiffOptions{})
	if err != nil || m1 == nil {
		t.Fatal(m1, err)
	}
	e1 := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(m1.Files))
	if _, err := e1.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	desired := docsSchema()
	desired.Tables[0].Indexes = append(desired.Tables[0].Indexes, schema.Index{
		Name: "docs_id_title", Concurrently: true, Parts: []schema.IndexPart{{Column: "id"}, {Column: "title"}},
	})
	desired.Tables[0].Uniques = []schema.Unique{{Name: "docs_title_key", Columns: []string{"title"}}}
	desired.Tables[0].Checks = []schema.Check{{Name: "docs_id_pos", Expr: "id > 0"}}
	m2, err := e1.Diff(ctx, desired, "harden", migrate.DiffOptions{})
	if err != nil || m2 == nil || m2.Next == nil {
		t.Fatalf("want a head and a tail: %v %v", m2, err)
	}
	files := map[string][]byte{}
	for k, v := range m1.Files {
		files[k] = v
	}
	for k, v := range m2.Files {
		files[k] = v
	}
	e2 := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(files))
	if _, err := e2.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`SELECT bool_and(indisvalid)::text FROM pg_index WHERE indrelid = to_regclass('docs') AND NOT indisprimary`,
		`SELECT convalidated::text FROM pg_constraint WHERE conname = 'docs_id_pos' AND connamespace = current_schema()::regnamespace`,
		`SELECT (contype = 'u')::text FROM pg_constraint WHERE conname = 'docs_title_key' AND connamespace = current_schema()::regnamespace`,
	} {
		if got := firstValue(t, db, q); got != "true" {
			t.Errorf("%s = %q, want true", q, got)
		}
	}
	inspected, err := e2.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !schema.Equal(inspected, desired) {
		t.Fatalf("database does not match the models\n%s", migratetest.Diff(inspected, desired))
	}
	if again, err := e2.Diff(ctx, desired, "noop", migrate.DiffOptions{}); err != nil || again != nil {
		t.Fatalf("a repeated diff must report no changes: %v %v", again, err)
	}
}

// TestPartitionedTableConstraintsAndIndexes settles what PostgreSQL accepts on
// a partitioned table: ADD CONSTRAINT ... CHECK ... NOT VALID followed by
// VALIDATE CONSTRAINT, a plain unique constraint and a plain index (the
// Concurrently() request is dropped there). The pair must apply and leave the
// check validated.
func TestPartitionedTableConstraintsAndIndexes(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	part := &schema.Partition{Strategy: "range", Key: "RANGE (id)"}
	base := docsSchema()
	base.Tables[0].Partition = part
	base.Tables[0].Indexes = nil
	first := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS())
	m1, err := first.Diff(ctx, base, "init", migrate.DiffOptions{})
	if err != nil || m1 == nil {
		t.Fatal(m1, err)
	}
	e1 := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(m1.Files))
	if _, err := e1.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}

	desired := docsSchema()
	desired.Tables[0].Partition = part
	desired.Tables[0].Indexes = []schema.Index{{Name: "docs_title", Concurrently: true, Parts: []schema.IndexPart{{Column: "title"}}}}
	desired.Tables[0].Uniques = []schema.Unique{{Name: "docs_id_title_key", Columns: []string{"id", "title"}}}
	desired.Tables[0].Checks = []schema.Check{{Name: "docs_id_pos", Expr: "id > 0"}}
	m2, err := e1.Diff(ctx, desired, "harden", migrate.DiffOptions{})
	if err != nil || m2 == nil {
		t.Fatal(m2, err)
	}
	files := map[string][]byte{}
	for k, v := range m1.Files {
		files[k] = v
	}
	for k, v := range m2.Files {
		files[k] = v
	}
	e2 := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(files))
	if _, err := e2.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`SELECT convalidated::text FROM pg_constraint WHERE conname = 'docs_id_pos' AND conrelid = to_regclass('docs')`,
		`SELECT (count(*) = 1)::text FROM pg_constraint WHERE conname = 'docs_id_title_key' AND contype = 'u' AND conrelid = to_regclass('docs')`,
		`SELECT (count(*) = 1)::text FROM pg_indexes WHERE indexname = 'docs_title' AND schemaname = current_schema()`,
	} {
		if got := firstValue(t, db, q); got != "true" {
			t.Errorf("%s = %q, want true", q, got)
		}
	}
}

// TestSplitHeadFailureLeavesNoPartialDDL breaks the transactional head after
// diff wrote it: its CREATE TABLE must roll back whole, and the tail must not
// run.
func TestSplitHeadFailureLeavesNoPartialDDL(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	d := docsSchema()
	d.Tables[0].Indexes[0].Concurrently = true
	m, err := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS()).Diff(ctx, d, "init", migrate.DiffOptions{})
	if err != nil || m == nil || m.Next == nil {
		t.Fatalf("want a head and a tail: %v %v", m, err)
	}
	dir := fstest.MapFS{}
	for k, v := range m.Files {
		dir[k] = &fstest.MapFile{Data: v}
	}
	headName := m.Version + "_init.up.sql"
	dir[headName] = &fstest.MapFile{Data: []byte(strings.TrimRight(string(m.Files[headName]), "\n") + "\n\nINSERT INTO missing_table VALUES (1);\n")}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.WithSum(t, dir))
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil {
		t.Fatal("apply must fail")
	}
	if tableExists(t, db, "docs") {
		t.Fatal("the failed head must leave no table behind")
	}
	if got := firstValue(t, db, `SELECT count(*)::text FROM pg_indexes WHERE indexname = 'docs_title' AND schemaname = current_schema()`); got != "0" {
		t.Fatalf("the tail must not have run, indexes = %s", got)
	}
}
