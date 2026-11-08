// SPDX-License-Identifier: Apache-2.0

//go:build integration

package bunfixture

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"github.com/azrtydxb/momo/importer"
	importerbun "github.com/azrtydxb/momo/importer/bun"
	"github.com/azrtydxb/momo/internal/compat/schemafixture"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
)

func TestImportBun(t *testing.T) {
	ctx := context.Background()
	// 1. the source ORM's own schema, created by Bun in the per-test schema
	sqldb, err := sql.Open("pgx", testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })
	db := bun.NewDB(sqldb, pgdialect.New())
	for _, m := range []any{(*Org)(nil), (*Account)(nil), (*Profile)(nil)} {
		if _, err := db.NewCreateTable().Model(m).WithForeignKeys().Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}
	source := schemafixture.AsMomoWould(t, schemafixture.Inspect(t, testopen.Postgres(t)))

	// 2. Momo models read from the source project
	models, rep, err := importerbun.Read(".")
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Has(importer.ZeroValueHandling) {
		t.Fatalf("nullzero not reported: %s", rep)
	}
	if rep.Has(importer.Unsupported) {
		t.Fatalf("the fixture must import cleanly: %s", rep)
	}
	if got := rep.Of(importer.DroppedIndex); len(got) != 1 {
		t.Fatalf("the composite unique constraint must be reported once: %s", rep)
	}

	// 3. the schema the real generator derives from the emitted models
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	desired := schemafixture.Emitted(t, dir, "imported", models, rep)

	// 4. the only difference is the composite unique constraint the importer reported
	changes, _, err := migrate.Diff(source, desired, migrate.DiffOptions{AllowDestructive: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Kind != migrate.DropIndex || changes[0].Name != "uq_profile_region_slug" {
		t.Fatalf("imported models differ from the Bun schema beyond the reported composite unique constraint: %+v", changes)
	}
}
