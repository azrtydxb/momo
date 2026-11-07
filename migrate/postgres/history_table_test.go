// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	testdb "github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
)

// historySchemas creates empty schemas named after the test, drops them when the
// test ends and returns their names.
func historySchemas(t *testing.T, c momo.PinnedConn, suffixes ...string) []string {
	t.Helper()
	var out []string
	for _, s := range suffixes {
		name := testdb.Schema(t) + "_" + s
		out = append(out, name)
		if _, err := c.Exec(context.Background(), `CREATE SCHEMA `+name, nil); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = c.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+name+` CASCADE`, nil) })
	}
	t.Cleanup(func() { _, _ = c.Exec(context.Background(), `RESET search_path`, nil) })
	return out
}

func setPath(t *testing.T, c momo.PinnedConn, path string) {
	t.Helper()
	if _, err := c.Exec(context.Background(), `SET search_path = `+path, nil); err != nil {
		t.Fatal(err)
	}
}

func relationCount(t *testing.T, c momo.PinnedConn, schemaName string) int {
	t.Helper()
	var n int
	mustQueryRow(t, c, `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = '`+schemaName+`' AND c.relname = 'momo_migrations'`, &n)
	return n
}

// A database written by an earlier momo holds an unqualified table in whatever
// schema was first on search_path. It must be found again, wherever that is, and
// no second table may appear in the current schema.
func TestHistoryTableFindsExistingUnqualifiedInstall(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	c := pinned(t, db)
	s := historySchemas(t, c, "old", "cur")
	old, cur := s[0], s[1]

	setPath(t, c, old)
	for _, ddl := range mpg.New().HistoryDDL() { // the unqualified DDL earlier versions ran
		if _, err := c.Exec(ctx, ddl, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.Exec(ctx, `INSERT INTO momo_migrations (version, name, checksum, status, applied_at) VALUES ('20250101000000', 'init', 'x', 'applied', '2025-01-01T00:00:00Z')`, nil); err != nil {
		t.Fatal(err)
	}

	setPath(t, c, cur+", "+old) // current_schema() is cur, the table lives in old
	ref, err := mpg.New().HistoryTable(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if want := `"` + old + `"."momo_migrations"`; ref != want {
		t.Fatalf("HistoryTable = %s, want %s", ref, want)
	}
	if err := migrate.EnsureHistory(ctx, c, mpg.New()); err != nil {
		t.Fatal(err)
	}
	if relationCount(t, c, cur) != 0 {
		t.Fatal("a second history table was created in the current schema")
	}

	p := migrate.Pending{Version: "20260101000000", Name: "next", Statements: []migrate.Statement{{SQL: "SELECT 1"}}}
	env := migrate.RunEnv{Conn: c, Dialect: mpg.New(), Clock: time.Now}
	if _, err := migrate.RunMigration(ctx, env, p, "a1"); err != nil {
		t.Fatal(err)
	}
	var n int
	mustQueryRow(t, c, `SELECT count(*) FROM "`+old+`".momo_migrations WHERE status = 'applied'`, &n)
	if n != 2 {
		t.Fatalf("want the old row and the new one in the existing table, got %d applied rows", n)
	}
	if relationCount(t, c, cur) != 0 {
		t.Fatal("the apply created a history table in the current schema")
	}
}

// With no table anywhere, the table is created in current_schema(), qualified.
func TestHistoryTableNewInstallIsQualified(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	c := pinned(t, db)
	s := historySchemas(t, c, "fresh")
	setPath(t, c, s[0])
	ref, err := mpg.New().HistoryTable(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if want := `"` + s[0] + `"."momo_migrations"`; ref != want {
		t.Fatalf("HistoryTable = %s, want %s", ref, want)
	}
	ddl := strings.Join(mpg.New().HistoryDDLFor(ref), "\n")
	if !strings.Contains(ddl, "CREATE TABLE IF NOT EXISTS "+ref+" (") {
		t.Fatalf("DDL is not qualified: %s", ddl)
	}
	if err := migrate.EnsureHistory(ctx, c, mpg.New()); err != nil {
		t.Fatal(err)
	}
	if relationCount(t, c, s[0]) != 1 {
		t.Fatal("the history table is not in the current schema")
	}
}

// A view, or a temporary table, named momo_migrations earlier on search_path must
// not be used, even when the real table sits in a later schema.
func TestHistoryTableRefusesShadowingObject(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	c := pinned(t, db)
	s := historySchemas(t, c, "shadow", "real")
	shadow, live := s[0], s[1]

	setPath(t, c, live)
	if err := migrate.EnsureHistory(ctx, c, mpg.New()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Exec(ctx, `CREATE VIEW "`+shadow+`".momo_migrations AS SELECT 'x'::text AS version, 'x'::text AS name, 'x'::text AS checksum, 'applied'::text AS status`, nil); err != nil {
		t.Fatal(err)
	}
	setPath(t, c, shadow+", "+live)
	if _, err := mpg.New().HistoryTable(ctx, c); err == nil || !strings.Contains(err.Error(), "view") {
		t.Fatalf("a view on the search_path must be refused, got %v", err)
	}
	if err := migrate.EnsureHistory(ctx, c, mpg.New()); err == nil {
		t.Fatal("EnsureHistory must refuse a shadowing view")
	}

	// Remove the view; a temporary table now shadows the real one.
	if _, err := c.Exec(ctx, `DROP VIEW "`+shadow+`".momo_migrations`, nil); err != nil {
		t.Fatal(err)
	}
	setPath(t, c, live)
	if _, err := c.Exec(ctx, `CREATE TEMP TABLE momo_migrations (version text, name text, checksum text, status text, applied_at text)`, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = c.Exec(context.Background(), `DROP TABLE IF EXISTS pg_temp.momo_migrations`, nil) })
	if _, err := mpg.New().HistoryTable(ctx, c); err == nil || !strings.Contains(err.Error(), "temporary") {
		t.Fatalf("a temporary table must be refused, got %v", err)
	}
}
