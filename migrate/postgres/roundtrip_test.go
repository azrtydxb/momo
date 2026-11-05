// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	testdb "github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

// planStatements is what the engine does for a diff: Diff, OrderWithViews,
// Render. The engine (a later task) does not exist yet, so the tests drive
// these steps directly and surface every error, cycle errors included.
func planStatements(t *testing.T, from, to *schema.Schema, opts migrate.DiffOptions) []migrate.Statement {
	t.Helper()
	changes, _, err := migrate.Diff(from, to, opts)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	ordered, err := migrate.OrderWithViews(changes, from.Views, to.Views)
	if err != nil {
		t.Fatalf("order: %v", err)
	}
	stmts, err := mpg.New().Render(ordered)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return stmts
}

// applyStatements runs each statement on its own autocommit round trip, which
// is how a NoTx statement (CREATE INDEX CONCURRENTLY) has to run.
func applyStatements(t *testing.T, db *momo.DB, stmts []migrate.Statement) {
	t.Helper()
	for _, s := range stmts {
		if _, err := db.Driver().Exec(context.Background(), s.SQL, nil); err != nil {
			t.Fatalf("apply %q: %v", s.SQL, err)
		}
	}
}

func inspectNow(t *testing.T, db *momo.DB) *schema.Schema {
	t.Helper()
	got, err := inspectOnce(context.Background(), t, db, migrate.InspectOptions{Schemas: []string{testdb.Schema(t)}})
	if err != nil {
		t.Fatal(err)
	}
	// The history table is the engine's own bookkeeping, not a model table;
	// drift-time Exclude (a later task) filters it, so the tests drop it here.
	got.Tables = slices.DeleteFunc(got.Tables, func(tb schema.Table) bool { return tb.Name == "momo_migrations" })
	return got
}

// engineOn returns an engine on db's per-test schema for the migration directory fsys.
func engineOn(t *testing.T, db *momo.DB, fsys fs.FS) *migrate.Engine {
	t.Helper()
	return migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys, migrate.WithConfig(migrate.Config{Schemas: []string{testdb.Schema(t)}}))
}

// migrationFiles turns rendered statements into migration files, in order.
// Consecutive statements that agree on NoTx share a file, and a file of NoTx
// statements carries the no-transaction directive. Versions count up from
// first in the 2026-01-01 timestamp range.
func migrationFiles(stmts []migrate.Statement, first int) map[string][]byte {
	files := map[string][]byte{}
	for i := 0; i < len(stmts); {
		j := i
		for j < len(stmts) && stmts[j].NoTx == stmts[i].NoTx {
			j++
		}
		var b strings.Builder
		if stmts[i].NoTx {
			b.WriteString("-- momo:no-transaction\n")
		}
		for _, s := range stmts[i:j] {
			b.WriteString(s.SQL + ";\n")
		}
		n := first + len(files)
		files[fmt.Sprintf("202601010000%02d_step%d.up.sql", n, n)] = []byte(b.String())
		i = j
	}
	return files
}

func jsonDiff(a, b *schema.Schema) string {
	x, _ := json.MarshalIndent(schema.Normalized(a), "", " ")
	y, _ := json.MarshalIndent(schema.Normalized(b), "", " ")
	return "got:\n" + string(x) + "\nwant:\n" + string(y)
}

// roundTrip applies desired from an empty database, inspects it back, requires
// schema.Equal and then requires a second diff to find nothing to do.
func roundTrip(t *testing.T, db *momo.DB, desired *schema.Schema) *schema.Schema {
	t.Helper()
	applyStatements(t, db, planStatements(t, &schema.Schema{Dialect: "postgres"}, desired, migrate.DiffOptions{}))
	got := inspectNow(t, db)
	if !schema.Equal(got, desired) {
		t.Fatalf("round trip has a diff:\n%s", jsonDiff(got, desired))
	}
	again, _, err := migrate.Diff(got, desired, migrate.DiffOptions{})
	if err != nil || len(again) != 0 {
		t.Fatalf("second diff must be empty: %v %+v", err, again)
	}
	return got
}

func TestPostgresObjects(t *testing.T) {
	db := testopen.Postgres(t)
	desired := &schema.Schema{
		Dialect:    "postgres",
		Extensions: []schema.Extension{{Name: "vector"}},
		Enums:      []schema.Enum{{Name: "status", Values: []string{"draft", "live"}}},
		Tables: []schema.Table{{
			Name: "docs",
			Columns: []schema.Column{
				{Name: "id", Type: "bigint", Identity: "always"},
				{Name: "state", Type: "status", Enum: "status", Default: "'draft'"},
				{Name: "title", Type: "text", Nullable: true},
				{Name: "emb", Type: "vector(3)", Nullable: true},
				{Name: "deleted_at", Type: "timestamp with time zone", Nullable: true},
			},
			PrimaryKey: []string{"id"},
			Indexes: []schema.Index{
				{Name: "docs_lower", Parts: []schema.IndexPart{{Expr: "lower(title)"}}},
				{Name: "docs_live", Parts: []schema.IndexPart{{Column: "id"}}, Where: "deleted_at IS NULL"},
				{Name: "docs_emb", Method: "hnsw", Parts: []schema.IndexPart{{Column: "emb", OpClass: "vector_cosine_ops"}}, With: []schema.KV{{Key: "m", Value: "16"}}},
				{Name: "docs_state_c", Parts: []schema.IndexPart{{Column: "state"}}, Concurrently: true},
			},
		}},
	}
	stmts := planStatements(t, &schema.Schema{Dialect: "postgres"}, desired, migrate.DiffOptions{})
	var sawNoTx bool
	for _, s := range stmts {
		if strings.HasPrefix(s.SQL, "CREATE INDEX CONCURRENTLY") {
			sawNoTx = s.NoTx
		}
	}
	if !sawNoTx {
		t.Fatalf("the CONCURRENTLY index must be a NoTx statement: %+v", stmts)
	}
	// The migration is applied through the engine: the CONCURRENTLY index sits
	// in its own no-transaction file, and the history records every version.
	files := migrationFiles(stmts, 0)
	if len(files) < 2 {
		t.Fatalf("the CONCURRENTLY index must be split into its own migration file: %v", files)
	}
	e := engineOn(t, db, migratetest.FS(files))
	rep, err := e.Apply(context.Background(), migrate.ApplyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Applied) != len(files) {
		t.Fatalf("applied %d of %d files: %+v", len(rep.Applied), len(files), rep)
	}
	got := inspectNow(t, db)
	if !schema.Equal(got, desired) {
		t.Fatalf("round trip has a diff:\n%s", migratetest.Diff(got, desired))
	}
	again, _, err := migrate.Diff(got, desired, migrate.DiffOptions{})
	if err != nil || len(again) != 0 {
		t.Fatalf("second diff must be empty: %v %+v", err, again)
	}
	plan, err := e.Plan(context.Background())
	if err != nil || len(plan) != 0 {
		t.Fatalf("nothing may remain pending: %v %+v", err, plan)
	}
}

func TestPostgresRoundTripConstraintsCollateComment(t *testing.T) {
	db := testopen.Postgres(t)
	desired := &schema.Schema{
		Dialect:    "postgres",
		Extensions: []schema.Extension{{Name: "vector"}}, // the test database has it installed; inspection reports every extension
		Tables: []schema.Table{
			{
				Name:       "orgs",
				Columns:    []schema.Column{{Name: "id", Type: "bigint", Identity: "by_default"}, {Name: "name", Type: "text", Collate: "C", Comment: "it's the \"org\" name \\ ok"}},
				PrimaryKey: []string{"id"},
				Uniques:    []schema.Unique{{Name: "orgs_name_key", Columns: []string{"name"}}},
				Checks:     []schema.Check{{Name: "orgs_name_len", Expr: "length(name) > 0"}},
			},
			{
				Name: "docs",
				Columns: []schema.Column{
					{Name: "id", Type: "bigint", Identity: "always"},
					{Name: "org_id", Type: "bigint"},
					{Name: "parent_id", Type: "bigint", Nullable: true},
				},
				PrimaryKey: []string{"id"},
				ForeignKeys: []schema.ForeignKey{
					{Name: "docs_org_fk", Columns: []string{"org_id"}, RefTable: "orgs", RefColumns: []string{"id"}, OnDelete: "CASCADE", OnUpdate: "SET NULL"},
					{Name: "docs_parent_fk", Columns: []string{"parent_id"}, RefTable: "docs", RefColumns: []string{"id"}, OnDelete: "SET NULL"},
				},
			},
		},
	}
	got := roundTrip(t, db, desired)
	orgs, _ := got.Table("orgs")
	col, _ := orgs.Column("name")
	if col.Collate != "C" || col.Comment != "it's the \"org\" name \\ ok" {
		t.Fatalf("collate and comment must survive: %+v", col)
	}
}

func TestPostgresAlterColumnRoundTrip(t *testing.T) {
	db := testopen.Postgres(t)
	v1 := &schema.Schema{Dialect: "postgres", Extensions: []schema.Extension{{Name: "vector"}}, Tables: []schema.Table{{
		Name:    "t",
		Columns: []schema.Column{{Name: "id", Type: "bigint"}, {Name: "n", Type: "text", Nullable: true, Default: "'x'"}, {Name: "gone", Type: "text", Nullable: true}},
	}}}
	roundTrip(t, db, v1)
	v2 := &schema.Schema{Dialect: "postgres", Extensions: []schema.Extension{{Name: "vector"}}, Tables: []schema.Table{{
		Name: "t",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint"},
			{Name: "n", Type: "varchar(20)", Collate: "C", Comment: "now commented", Default: "'y'"},
			{Name: "added", Type: "integer", Default: "0", Comment: "new"},
		},
	}}}
	stmts := planStatements(t, v1, v2, migrate.DiffOptions{AllowDestructive: true})
	applyStatements(t, db, stmts)
	got := inspectNow(t, db)
	if !schema.Equal(got, v2) {
		t.Fatalf("alter round trip has a diff:\n%s", jsonDiff(got, v2))
	}
}

func TestPostgresConcurrentIndexNoTx(t *testing.T) {
	stmts, err := mpg.Dialect{}.Render([]migrate.Change{{Kind: "create_index", Table: "docs", Name: "i", After: &schema.Index{Name: "i", Concurrently: true, Parts: []schema.IndexPart{{Column: "x"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if !stmts[0].NoTx || !strings.HasPrefix(stmts[0].SQL, "CREATE INDEX CONCURRENTLY") {
		t.Fatalf("concurrent index must render outside a transaction: %+v", stmts[0])
	}
}
