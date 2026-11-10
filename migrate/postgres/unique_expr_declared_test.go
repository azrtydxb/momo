// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

func uniqueExprSchema(t *testing.T) *schema.Schema {
	t.Helper()
	dir, err := filepath.Abs("../../gen/testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./companionuniqexpr"}, gen.Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	s, sd := gen.BuildSchema(g.Models, gen.Config{Dialects: []dialect.Name{dialect.Postgres}})
	if len(sd) != 0 {
		t.Fatalf("schema diagnostics: %v", sd)
	}
	s.Extensions = []schema.Extension{{Name: "vector"}} // inspection reports every installed extension
	return s
}

// TestDeclaredUniqueExpressionIndexes is issue 55 on PostgreSQL: unique
// expression indexes (alone, mixed with a column, partial) render, enforce
// uniqueness, and inspect back without a diff.
func TestDeclaredUniqueExpressionIndexes(t *testing.T) {
	ctx := context.Background()
	db := testopen.Postgres(t)
	desired := uniqueExprSchema(t)

	stmts := planStatements(t, &schema.Schema{Dialect: "postgres"}, desired, migrate.DiffOptions{})
	var sql strings.Builder
	for _, s := range stmts {
		sql.WriteString(s.SQL + "\n")
	}
	for _, frag := range []string{
		`CREATE UNIQUE INDEX principal_email_key ON principal ((lower(email)))`,
		`CREATE UNIQUE INDEX template_company_name_key ON template (company_id, (lower(name)))`,
		`WHERE deleted_at IS NULL`,
	} {
		if !strings.Contains(strings.ReplaceAll(sql.String(), `"`, ""), frag) {
			t.Errorf("rendered SQL lacks %q:\n%s", frag, sql.String())
		}
	}
	applyStatements(t, db, stmts)

	exec := func(q string) error {
		_, err := db.Driver().Exec(ctx, q, nil)
		return err
	}
	if err := exec(`INSERT INTO principal (id, email) VALUES (1, 'Ann@Example.com')`); err != nil {
		t.Fatal(err)
	}
	if err := exec(`INSERT INTO principal (id, email) VALUES (2, 'ann@example.com')`); !errors.Is(err, momo.ErrUniqueViolation) {
		t.Fatalf("a case-different duplicate email must fail with ErrUniqueViolation, got %v", err)
	}

	got := inspectNow(t, db)
	want := uniqueExprSchema(t)
	if err := mpg.New().Canonicalize(ctx, pinned(t, db), want); err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if !schema.Equal(got, want) {
		t.Fatalf("round trip has a diff:\n%s", migratetest.Diff(got, want))
	}
	if again, _, err := migrate.Diff(got, want, migrate.DiffOptions{}); err != nil || len(again) != 0 {
		t.Fatalf("second diff must be empty: %v %+v", err, again)
	}
}
