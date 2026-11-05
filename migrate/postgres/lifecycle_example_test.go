// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/gen"
	testdb "github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

// exampleSchema builds the schema of the shared example models the way
// momo generate does.
func exampleSchema(t *testing.T) *schema.Schema {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./internal/testmodels/example/..."}, gen.Config{Dir: root})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	s, bd := gen.BuildSchema(g.Models, gen.Config{})
	if len(bd) != 0 {
		t.Fatalf("build: %v", bd)
	}
	return s
}

// TestLifecycleExampleModelsNoPhantomDrift drives Diff, Apply and Drift with the
// example models: a freshly applied migration must not drift, and a second Diff
// must find nothing to do.
func TestLifecycleExampleModelsNoPhantomDrift(t *testing.T) {
	db := testopen.Postgres(t)
	desired := exampleSchema(t)
	ctx := context.Background()
	e := engineOn(t, db, migratetest.EmptyFS())
	m, err := e.Diff(ctx, desired, "init", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatalf("diff: %v %v", m, err)
	}
	e = engineOn(t, db, migratetest.FS(m.Files))
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	rep, err := e.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean() {
		// Before the fix: documents.documents_title_nonempty reported "expr
		// differ": declared "length(title) > 0", stored
		// "(length((title)::text) > 0)".
		t.Fatalf("phantom drift after apply: %+v", rep)
	}
	again, err := e.Diff(ctx, desired, "again", migrate.DiffOptions{})
	if err != nil || again != nil {
		t.Fatalf("second diff must be empty: %v %+v", err, again)
	}
}

// TestLifecycleTableTextNoPhantomDrift covers the other table text PostgreSQL
// rewrites on storage: IN and BETWEEN checks, column defaults, a generated
// column and index predicates and expressions.
func TestLifecycleTableTextNoPhantomDrift(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	desired := &schema.Schema{
		Dialect:    "postgres",
		Extensions: []schema.Extension{{Name: "vector"}}, // installed in the test database; inspection reports every extension
		Tables: []schema.Table{{
			Name: "items",
			Columns: []schema.Column{
				{Name: "id", Type: "bigint", Identity: "always"},
				{Name: "title", Type: "varchar(300)"},
				{Name: "status", Type: "text", Default: "'draft'"},
				{Name: "qty", Type: "integer", Default: "(1 + 2)"},
				{Name: "tags", Type: "text[]", Default: "'{}'"},
				{Name: "created_at", Type: "timestamptz", Default: "CURRENT_TIMESTAMP"},
				{Name: "label", Type: "text", Generated: "lower(title)"},
			},
			PrimaryKey: []string{"id"},
			Checks: []schema.Check{
				{Name: "items_status_in", Expr: "status IN ('draft','live')"},
				{Name: "items_qty_range", Expr: "qty BETWEEN 1 AND 5"},
				{Name: "items_title_ne", Expr: "title <> '' AND length(title) > 0"},
			},
			Indexes: []schema.Index{
				{Name: "items_lower_title", Parts: []schema.IndexPart{{Expr: "lower(title)"}}, Where: "status IN ('draft','live') AND qty > 3"},
			},
		}},
	}
	e := engineOn(t, db, migratetest.EmptyFS())
	m, err := e.Diff(ctx, desired, "init", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatalf("diff: %v %v", m, err)
	}
	e = engineOn(t, db, migratetest.FS(m.Files))
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	rep, err := e.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean() {
		t.Fatalf("phantom drift after apply: %+v", rep)
	}

	// Drift still sees a real change of a check made outside migrations.
	if _, err := db.Driver().Exec(ctx, `ALTER TABLE items DROP CONSTRAINT items_qty_range, ADD CONSTRAINT items_qty_range CHECK (qty BETWEEN 1 AND 6)`, nil); err != nil {
		t.Fatal(err)
	}
	rep, err = e.Drift(ctx)
	if err != nil || len(rep.Changed) != 1 || rep.Changed[0].Object.Name != "items.items_qty_range" {
		t.Fatalf("a changed check must still be reported: %+v %v", rep, err)
	}
}

// noPrivDialect is the PostgreSQL dialect whose Canonicalize reports a missing
// CREATE privilege on the database.
type baseDialect = migrate.Dialect

type noPrivDialect struct{ baseDialect }

func (noPrivDialect) Canonicalize(context.Context, momo.PinnedConn, *schema.Schema) error {
	return fmt.Errorf("%w: test", migrate.ErrCanonicalizePrivilege)
}

// A role that cannot create the scratch schema still gets a drift report: Drift
// falls back to the declared text instead of failing.
func TestDriftWithoutCanonicalizePrivilegeStillReports(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	desired := &schema.Schema{Dialect: "postgres", Extensions: []schema.Extension{{Name: "vector"}}, Tables: []schema.Table{{
		Name:       "plain",
		Columns:    []schema.Column{{Name: "id", Type: "bigint"}, {Name: "n", Type: "integer", Default: "0"}},
		PrimaryKey: []string{"id"},
	}}}
	m, err := engineOn(t, db, migratetest.EmptyFS()).Diff(ctx, desired, "init", migrate.DiffOptions{})
	if err != nil || m == nil {
		t.Fatalf("diff: %v %v", m, err)
	}
	files := migratetest.FS(m.Files)
	if _, err := engineOn(t, db, files).Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	e := migratetest.NewEngine(t, noPrivDialect{mpg.New()}, db.Driver(), files, migrate.WithConfig(migrate.Config{Schemas: []string{testdb.Schema(t)}}))
	rep, err := e.Drift(ctx)
	if err != nil || !rep.Clean() {
		t.Fatalf("drift must fall back to the declared text: %+v %v", rep, err)
	}
	if got := skippedNames(rep); len(got) != 1 || got[0] != "table:plain" {
		t.Fatalf("the fallback must be reported, CanonicalizeSkipped = %v", got)
	}
}
