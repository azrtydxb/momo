// SPDX-License-Identifier: Apache-2.0

//go:build integration

package gormfixture

import (
	"os"
	"testing"

	"github.com/azrtydxb/momo/importer"
	"github.com/azrtydxb/momo/importer/gorm"
	"github.com/azrtydxb/momo/internal/compat/schemafixture"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"gorm.io/driver/postgres"
	gormdb "gorm.io/gorm"
)

func TestImportGORM(t *testing.T) {
	// 1. the source ORM's own schema, created by GORM in the per-test schema
	g, err := gormdb.Open(postgres.Open(testdb.PostgresURL(t)), &gormdb.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := g.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := g.AutoMigrate(&Org{}, &User{}, &Post{}, &Comment{}); err != nil {
		t.Fatal(err)
	}
	source := schemafixture.AsMomoWould(t, schemafixture.Inspect(t, testopen.Postgres(t)))

	// 2. Momo models read from the source project
	models, rep, err := gorm.Read(".")
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Has(importer.ZeroValueHandling) || !rep.Has(importer.ImplicitSave) {
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
	if len(changes) != 1 || changes[0].Kind != migrate.DropIndex || changes[0].Name != "idx_comment_kind_body" {
		t.Fatalf("imported models differ from the GORM schema beyond the reported composite index: %+v", changes)
	}
}
