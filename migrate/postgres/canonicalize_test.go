// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"bytes"
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

// declaredObjects is a model whose views, function body, policy and trigger
// condition are written differently from the text PostgreSQL reports.
func declaredObjects() *schema.Schema {
	return &schema.Schema{
		Dialect:    "postgres",
		Extensions: []schema.Extension{{Name: "vector"}},
		Tables: []schema.Table{{
			Name:       "docs",
			Columns:    []schema.Column{{Name: "id", Type: "bigint"}, {Name: "title", Type: "text", Nullable: true}},
			PrimaryKey: []string{"id"},
			RLS:        &schema.RLS{Enabled: true},
		}},
		Views: []schema.View{
			{Name: "v_docs", Query: "SELECT id, title FROM docs WHERE id>0", Depends: []string{"docs"}},
			{Name: "v_titles", Query: "select title from v_docs", Depends: []string{"v_docs"}},
			{Name: "m_count", Materialized: true, Query: "SELECT count(*) AS n FROM docs", Depends: []string{"docs"}},
		},
		Functions: []schema.Function{
			{Name: "double_it", Args: "x integer", Returns: "integer", Language: "sql", Volatility: "IMMUTABLE", Body: "SELECT\n    x *   2"},
			{Name: "touch", Args: "", Returns: "trigger", Language: "plpgsql", Volatility: "VOLATILE", Body: "BEGIN RETURN NEW; END;"},
		},
		Triggers: []schema.Trigger{{
			Table: "docs", Name: "docs_touch", Timing: "BEFORE", Events: []string{"INSERT"}, ForEachRow: true,
			When: "new.id>0", Function: "touch()",
		}},
		Policies: []schema.Policy{{Table: "docs", Name: "docs_pos", Command: "SELECT", Permissive: true, Using: "docs.id>0"}},
	}
}

// createDeclaredLive creates the objects of declaredObjects by hand, the way a
// database that predates the migrations holds them.
func createDeclaredLive(t *testing.T, d interface {
	Exec(context.Context, string, []any) (int64, error)
}) {
	t.Helper()
	for _, q := range []string{
		`CREATE TABLE docs (id bigint PRIMARY KEY, title text)`,
		`INSERT INTO docs VALUES (1, 'a'), (2, 'b'), (3, 'c')`,
		`CREATE VIEW v_docs AS SELECT id, title FROM docs WHERE id > 0`,
		`CREATE VIEW v_titles AS SELECT title FROM v_docs`,
		`CREATE MATERIALIZED VIEW m_count AS SELECT count(*) AS n FROM docs`,
		`CREATE FUNCTION double_it(x integer) RETURNS integer LANGUAGE sql IMMUTABLE AS $$SELECT x * 2$$`,
		`CREATE FUNCTION touch() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RETURN NEW; END;$$`,
		`CREATE TRIGGER docs_touch BEFORE INSERT ON docs FOR EACH ROW WHEN (NEW.id > 0) EXECUTE FUNCTION touch()`,
		`ALTER TABLE docs ENABLE ROW LEVEL SECURITY`,
		`CREATE POLICY docs_pos ON docs FOR SELECT USING (id > 0)`,
	} {
		if _, err := d.Exec(context.Background(), q, nil); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
}

// TestCanonicalizeRewritesDeclaredText: the declared text of a view, function,
// trigger and policy is replaced by what PostgreSQL reports, the result then
// equals an inspection of the live objects, and nothing is left behind.
func TestCanonicalizeRewritesDeclaredText(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	createDeclaredLive(t, db.Driver())
	pc := pinned(t, db)
	d := mpg.New()
	live, err := d.Inspect(ctx, pc, migrate.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	declared := declaredObjects()
	if schema.Equal(declared, live) {
		t.Fatal("precondition: the declared text must differ from the live text")
	}
	if err := d.Canonicalize(ctx, pc, declared); err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if !schema.Equal(declared, live) {
		t.Fatalf("canonicalized schema must equal the inspection:\n%s", migratetest.Diff(declared, live))
	}
	var q string
	for _, v := range declared.Views {
		if v.Name == "v_docs" {
			q = v.Query
		}
	}
	if !strings.Contains(q, "WHERE id > 0") || strings.Contains(q, "id>0") {
		t.Fatalf("view text must be the form pg_get_viewdef reports, got %q", q)
	}

	// Nothing is left behind: no scratch schema, and the live objects are intact.
	rows, err := db.Driver().Query(ctx, `SELECT count(*) FROM pg_namespace WHERE nspname LIKE 'momo\_canon%'`, nil)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if !rows.Next() {
		t.Fatal("no row")
	}
	if err := rows.Scan(&n); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	if n != 0 {
		t.Fatalf("scratch schema left behind: %d", n)
	}
}

func TestCanonicalizeNoObjectsIsNoop(t *testing.T) {
	db := testopen.Postgres(t)
	cd := migratetest.NewCountingDriver(db.Driver())
	pc, err := cd.Pin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Release() })
	s := &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{Name: "t", Columns: []schema.Column{{Name: "id", Type: "int"}}}}}
	if err := mpg.New().Canonicalize(context.Background(), pc, s); err != nil {
		t.Fatal(err)
	}
	if cd.DDL() != 0 {
		t.Fatalf("a schema without views, functions, triggers or policies needs no scratch schema, ran %d DDL", cd.DDL())
	}
}

// TestBaselineThenDiffKeepsViewsAndMatviewData: the live objects exist, the
// snapshot declares them in a different spelling, the baseline succeeds and the
// next Diff of the same model has no change, so no view is dropped and
// recreated and the materialized view keeps its rows.
func TestBaselineThenDiffKeepsViewsAndMatviewData(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	createDeclaredLive(t, db.Driver())
	declared := declaredObjects()
	var snap bytes.Buffer
	if err := migrate.WriteSnapshot(&snap, migrate.SnapshotFile{Version: "20260101000000", Schema: declared}); err != nil {
		t.Fatal(err)
	}
	fsys := migratetest.WithSum(t, fstest.MapFS{
		"20260101000000_init.up.sql":        {Data: []byte("SELECT 1;")},
		"20260101000000_init.snapshot.json": {Data: snap.Bytes()},
	})
	e := engineOn(t, db, fsys)
	if err := e.Baseline(ctx, "20260101000000", false); err != nil {
		t.Fatalf("baseline of declared views, function, trigger and policy: %v", err)
	}
	mig, err := e.Diff(ctx, declaredObjects(), "again", migrate.DiffOptions{})
	if err != nil || mig != nil {
		t.Fatalf("diff after baseline must have no changes: %+v %v", mig, err)
	}
	rep, err := e.Drift(ctx)
	if err != nil || !rep.Clean() {
		t.Fatalf("drift after baseline: %+v %v", rep, err)
	}
	rows, err := db.Driver().Query(ctx, `SELECT n FROM m_count`, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var n int
	if !rows.Next() {
		t.Fatal("materialized view is empty")
	}
	if err := rows.Scan(&n); err != nil || n != 3 {
		t.Fatalf("materialized view data must be kept: n=%d err=%v", n, err)
	}
}
