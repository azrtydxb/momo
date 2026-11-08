// SPDX-License-Identifier: Apache-2.0

//go:build integration

package entfixture

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/azrtydxb/momo/importer"
	importent "github.com/azrtydxb/momo/importer/ent"
	entgen "github.com/azrtydxb/momo/internal/compat/entfixture/ent"
	"github.com/azrtydxb/momo/internal/compat/schemafixture"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
)

func TestImportEnt(t *testing.T) {
	// 1. the source ORM's own schema, created by Ent's migration in the
	// per-test schema
	sqlDB, err := sql.Open("pgx", testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	client := entgen.NewClient(entgen.Driver(entsql.OpenDB(dialect.Postgres, sqlDB)))
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatal(err)
	}
	source := schemafixture.AsMomoWould(t, schemafixture.Inspect(t, testopen.Postgres(t)))

	// 2. Momo models read from the Ent schema package, which is not run
	models, rep, err := importent.Read("./ent/schema")
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Has(importer.ImplicitSave) || !rep.Has(importer.GoDefault) {
		t.Fatalf("semantic changes not reported: %s", rep)
	}
	if rep.Has(importer.Unsupported) {
		t.Fatalf("the fixture must import cleanly: %s", rep)
	}
	if got := rep.Of(importer.DroppedIndex); len(got) != 1 {
		t.Fatalf("the composite index must be reported once: %s", rep)
	}

	// 3. the schema the real generator derives from the emitted models
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	desired := schemafixture.Emitted(t, dir, "imported", models, rep)

	// 4. the only difference is the composite index the importer reported
	changes, _, err := migrate.Diff(source, desired, migrate.DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != migrate.DropIndex || changes[0].Name != "user_score_active" {
		t.Fatalf("imported models differ from the Ent schema beyond the reported composite index: %+v", changes)
	}
}
