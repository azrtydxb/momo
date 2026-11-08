// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"testing"

	testdb "github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

// TestPostgresSchemas: models in the per-test schema and in a second schema
// migrate, with the extra schema created by the migration and its table
// qualified; drift sees the objects of both schemas and labels the unmanaged
// table with its schema.
func TestPostgresSchemas(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	cur := testdb.Schema(t)
	extra := cur + "_billing" // the harness drops only the per-test schema
	t.Cleanup(func() {
		if _, err := db.Driver().Exec(context.Background(), `DROP SCHEMA IF EXISTS "`+extra+`" CASCADE`, nil); err != nil {
			t.Errorf("drop extra schema %s: %v", extra, err)
		}
	})
	cfg := migrate.WithConfig(migrate.Config{Schemas: []string{cur, extra}})
	desired := &schema.Schema{
		Dialect: "postgres",
		Schemas: []schema.SchemaNamespace{{Name: extra}},
		Tables: []schema.Table{
			{Schema: extra, Name: "invoices", Columns: []schema.Column{{Name: "id", Type: "bigint"}}, PrimaryKey: []string{"id"}},
			{Name: "users", Columns: []schema.Column{{Name: "id", Type: "bigint"}}, PrimaryKey: []string{"id"}},
		},
	}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS(), cfg)
	mig, err := e.Diff(ctx, desired, "schemas", migrate.DiffOptions{})
	if err != nil || mig == nil {
		t.Fatalf("diff: %v %v", mig, err)
	}
	e2 := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.FS(mig.Files), cfg)
	if _, err := e2.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if !tableExistsIn(t, db, extra, "invoices") {
		t.Fatal("schema-qualified table not created in the extra schema")
	}
	if !tableExistsIn(t, db, cur, "users") {
		t.Fatal("unqualified table not created in the current schema")
	}

	// Migrated objects of both schemas round-trip with nothing left to do.
	rep, err := e2.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range append(append([]migrate.ObjectRef{}, rep.Missing...), rep.Unmanaged...) {
		if o.Name == "invoices" || o.Name == "users" {
			t.Fatalf("migrated table %s.%s reported by drift: %+v", o.Schema, o.Name, rep)
		}
	}
	for _, d := range rep.Changed {
		if d.Object.Name == "invoices" || d.Object.Name == "users" {
			t.Fatalf("migrated table %s.%s reported as changed: %+v", d.Object.Schema, d.Object.Name, d)
		}
	}

	mustExec(t, db.Driver(), `CREATE TABLE "`+extra+`".stray (id bigint)`)
	rep, err = e2.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, o := range rep.Unmanaged {
		if o.Name == "stray" {
			found = true
			if o.Schema != extra {
				t.Fatalf("drift must label the stray table with its schema %q, got %+v", extra, o)
			}
		}
	}
	if !found {
		t.Fatalf("drift must report the stray table of the extra schema: %+v", rep.Unmanaged)
	}
}
