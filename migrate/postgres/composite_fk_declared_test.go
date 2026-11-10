// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

func declaredCompositeFKSchema(t *testing.T) *schema.Schema {
	t.Helper()
	dir, err := filepath.Abs("../../gen/testdata/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./companionfk"}, gen.Config{Dir: dir})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	s, sd := gen.BuildSchema(g.Models, gen.Config{Dialects: []dialect.Name{dialect.Postgres}})
	if len(sd) != 0 {
		t.Fatalf("schema diagnostics: %v", sd)
	}
	return s
}

// TestDeclaredCompositeForeignKeys is issue 54 on PostgreSQL: composite keys
// declared with Config.ForeignKey (two and three columns, into unique
// constraints) render, inspect back without a diff, and a later diff alters
// and drops them.
func TestDeclaredCompositeForeignKeys(t *testing.T) {
	ctx := context.Background()
	db := testopen.Postgres(t)
	desired := declaredCompositeFKSchema(t)
	desired.Extensions = []schema.Extension{{Name: "vector"}} // inspection reports every installed extension

	stmts := planStatements(t, &schema.Schema{Dialect: "postgres"}, desired, migrate.DiffOptions{})
	var sql strings.Builder
	for _, s := range stmts {
		sql.WriteString(s.SQL + "\n")
	}
	for _, frag := range []string{
		`FOREIGN KEY (company_id, document_id) REFERENCES document (company_id, id) ON DELETE CASCADE ON UPDATE RESTRICT`,
		`FOREIGN KEY (company_id, template_id, version_no) REFERENCES document (company_id, template_id, version_no)`,
	} {
		if !strings.Contains(strings.ReplaceAll(sql.String(), `"`, ""), frag) {
			t.Errorf("rendered SQL lacks %q:\n%s", frag, sql.String())
		}
	}
	applyStatements(t, db, stmts)

	got := inspectNow(t, db)
	want := declaredCompositeFKSchema(t)
	want.Extensions = desired.Extensions
	if err := mpg.New().Canonicalize(ctx, pinned(t, db), want); err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if !schema.Equal(got, want) {
		t.Fatalf("round trip has a diff:\n%s", migratetest.Diff(got, want))
	}
	if again, _, err := migrate.Diff(got, want, migrate.DiffOptions{}); err != nil || len(again) != 0 {
		t.Fatalf("second diff must be empty: %v %+v", err, again)
	}

	// Alter: a changed action is a drop and an add of the same key.
	altered := declaredCompositeFKSchema(t)
	altered.Extensions = desired.Extensions
	note := slices.IndexFunc(altered.Tables, func(tb schema.Table) bool { return tb.Name == "note" })
	altered.Tables[note].ForeignKeys[0].OnDelete = "SET NULL"
	applyStatements(t, db, planStatements(t, got, altered, migrate.DiffOptions{}))
	got = inspectNow(t, db)
	if fk := got.Tables[slices.IndexFunc(got.Tables, func(tb schema.Table) bool { return tb.Name == "note" })].ForeignKeys; len(fk) != 1 || fk[0].OnDelete != "SET NULL" {
		t.Fatalf("alter did not change the action: %+v", fk)
	}

	// Drop: removing the declaration drops the constraint.
	dropped := declaredCompositeFKSchema(t)
	dropped.Extensions = desired.Extensions
	for i := range dropped.Tables {
		dropped.Tables[i].ForeignKeys = nil
	}
	applyStatements(t, db, planStatements(t, got, dropped, migrate.DiffOptions{}))
	got = inspectNow(t, db)
	for _, tb := range got.Tables {
		if len(tb.ForeignKeys) != 0 {
			t.Fatalf("table %s kept foreign keys after the drop: %+v", tb.Name, tb.ForeignKeys)
		}
	}
}
