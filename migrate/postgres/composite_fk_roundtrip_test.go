// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

func compositeFKSchema(t *testing.T) *schema.Schema {
	t.Helper()
	dir, err := filepath.Abs("../../gen/testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./compositefk"}, gen.Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	s, sd := gen.BuildSchema(g.Models, gen.Config{Dialects: []dialect.Name{dialect.Postgres}})
	if len(sd) != 0 {
		t.Fatalf("schema diagnostics: %v", sd)
	}
	return s
}

// TestCompositePKForeignKeyNoDiff creates the tables by hand, the way an
// existing database has them (issue 13), inspects, and requires the diff
// against the schema generated from the models to be empty.
func TestCompositePKForeignKeyNoDiff(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	for _, ddl := range []string{
		`CREATE TABLE licenses (license_id bigint PRIMARY KEY, name text NOT NULL)`,
		`CREATE TABLE exams (license_id bigint NOT NULL, exam_id bigint NOT NULL, title text NOT NULL, PRIMARY KEY (license_id, exam_id))`,
		`ALTER TABLE exams ADD CONSTRAINT exams_license_id_fkey FOREIGN KEY (license_id) REFERENCES licenses(license_id)`,
	} {
		if _, err := db.Driver().Exec(ctx, ddl, nil); err != nil {
			t.Fatalf("%s: %v", ddl, err)
		}
	}
	got := inspectNow(t, db)
	want := compositeFKSchema(t)
	if err := mpg.New().Canonicalize(ctx, pinned(t, db), want); err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if !schema.Equal(got, want) {
		t.Fatalf("live schema differs from models:\n%s", migratetest.Diff(got, want))
	}
	changes, _, err := migrate.Diff(got, want, migrate.DiffOptions{})
	if err != nil || len(changes) != 0 {
		t.Fatalf("diff must be empty: %v %+v", err, changes)
	}
}
