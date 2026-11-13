// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momotest_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/momotest"
)

// ownURLEnv names the variable that points momotest at a test's own database.
const ownURLEnv = "MOMO_TEST_ISSUE115_URL"

// issue115Migrations creates its objects with IF NOT EXISTS on purpose: with
// public also holding them, they must still be created in the test schema.
func issue115Migrations() fstest.MapFS {
	return fstest.MapFS{
		"20260101000000_issue115.up.sql": {Data: []byte(
			`CREATE TABLE IF NOT EXISTS issue115_items (id bigint PRIMARY KEY, name text NOT NULL);
CREATE INDEX IF NOT EXISTS issue115_items_name ON issue115_items (name);`)},
	}
}

// requirePostgres returns the administrator URL of the momotest server. When it
// is unset the test is skipped, unless MOMO_REQUIRE_INTEGRATION=1 and
// PostgreSQL is among the selected dialects, where it fails.
func requirePostgres(t *testing.T) string {
	t.Helper()
	raw := os.Getenv(momotest.DefaultEnv)
	if raw != "" {
		return raw
	}
	dialects := strings.ToLower(os.Getenv("MOMO_INTEGRATION_DIALECTS"))
	if os.Getenv(momotest.RequireEnv) == "1" && (dialects == "" || strings.Contains(dialects, "postgres")) {
		t.Fatalf("%s is not set and %s=1", momotest.DefaultEnv, momotest.RequireEnv)
	}
	t.Skip(momotest.DefaultEnv + " is not set")
	return ""
}

// ownDatabase creates a database used by this test alone, points momotest at it
// through ownURLEnv, and drops it when the test ends. It returns the URL of that
// database, with no search_path. Nothing of the shared database is touched, so
// packages running in parallel on the same server never see what a test puts
// in this database's public schema.
func ownDatabase(t *testing.T) string {
	t.Helper()
	raw := requirePostgres(t)
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	name := "momotest_115_" + hex.EncodeToString(b[:])
	quoted := pgx.Identifier{name}.Sanitize()

	ctx := context.WithoutCancel(t.Context())
	admin, err := pgx.Connect(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	// Registered before the CREATE so that a half-made database is dropped too.
	t.Cleanup(func() {
		c, err := pgx.Connect(ctx, raw)
		if err != nil {
			t.Errorf("drop database %s: %v", name, err)
			return
		}
		defer c.Close(ctx)
		if _, err := c.Exec(ctx, "DROP DATABASE IF EXISTS "+quoted+" WITH (FORCE)"); err != nil {
			t.Errorf("drop database %s: %v", name, err)
		}
	})
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		t.Fatalf("create database %s: %v", name, err)
	}

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	u.RawPath = ""
	q := u.Query()
	q.Del("search_path")
	u.RawQuery = q.Encode()
	t.Setenv(ownURLEnv, u.String())
	return u.String()
}

// connectTo opens a connection to raw that closes with the test.
func connectTo(t *testing.T, raw string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(context.WithoutCancel(t.Context())) })
	return conn
}

// migratePublic applies fsys to the public schema of the database at raw, as a
// CI job running momo migrate apply on the same database does.
func migratePublic(t *testing.T, raw string, fsys fstest.MapFS) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", "public")
	u.RawQuery = q.Encode()
	pool, err := pgxpool.New(t.Context(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	e, err := migrate.NewEngine(pgxdriver.New(pool), fsys, migrate.WithDialect(mpg.New()), migrate.WithoutSum())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Apply(t.Context(), migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
}

// TestWithMigrationsWhenPublicIsMigrated catches a momotest migrator that finds
// public.momo_migrations through the search path (test schema, public), sees
// every migration applied and builds nothing in the test schema, so tests run
// on public's tables and share and leak rows (issue #115). It also pins that
// CREATE TABLE and CREATE INDEX IF NOT EXISTS create their objects in the test
// schema although public holds objects of the same names.
func TestWithMigrationsWhenPublicIsMigrated(t *testing.T) {
	raw := ownDatabase(t)
	migratePublic(t, raw, issue115Migrations())
	admin := connectTo(t, raw)

	insert := func(t *testing.T, q momo.Querier) {
		t.Helper()
		if _, err := momo.Exec(t.Context(), q, "INSERT INTO issue115_items (id, name) VALUES (1, 'x')"); err != nil {
			t.Fatalf("insert the same primary key: %v", err)
		}
	}
	for _, name := range []string{"first", "second"} {
		t.Run("New/"+name, func(t *testing.T) {
			db := momotest.New(t, momotest.WithEnv(ownURLEnv), momotest.WithMigrations(issue115Migrations()))
			schema := scalar[string](t, db.Driver(), "SELECT current_schema()")
			insert(t, db)
			for _, rel := range []string{"issue115_items", "issue115_items_name", "momo_migrations"} {
				if !scalar[bool](t, db.Driver(), "SELECT to_regclass($1) IS NOT NULL", `"`+schema+`"."`+rel+`"`) {
					t.Errorf("%s.%s does not exist: the migrations did not run in the test schema", schema, rel)
				}
			}
		})
	}
	t.Run("NewTx", func(t *testing.T) {
		tx := momotest.NewTx(t, momotest.WithEnv(ownURLEnv), momotest.WithMigrations(issue115Migrations()))
		insert(t, tx)
	})
	var n int
	if err := admin.QueryRow(t.Context(), "SELECT count(*) FROM public.issue115_items").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("public.issue115_items holds %d rows: a test wrote to public", n)
	}
}

// guardTB records Fatalf and stops the calling goroutine, as the real one does.
type guardTB struct {
	testing.TB
	mu   sync.Mutex
	errs []string
}

func (g *guardTB) Helper() {}

func (g *guardTB) Fatalf(format string, args ...any) {
	g.mu.Lock()
	g.errs = append(g.errs, fmt.Sprintf(format, args...))
	g.mu.Unlock()
	runtime.Goexit()
}

// TestWithMigrationsFailsWhenNothingLandsInTestSchema catches a momotest that
// returns a database whose test schema holds no table after WithMigrations, so
// the test would silently run on tables outside it. A migration that qualifies
// its table with public makes the migrations resolve outside the schema
// deterministically.
func TestWithMigrationsFailsWhenNothingLandsInTestSchema(t *testing.T) {
	ownDatabase(t)
	fsys := fstest.MapFS{
		"20260101000000_guard.up.sql": {Data: []byte(`CREATE TABLE public.issue115_guard (id bigint PRIMARY KEY);`)},
	}
	g := &guardTB{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		momotest.New(g, momotest.WithEnv(ownURLEnv), momotest.WithMigrations(fsys))
	}()
	<-done
	if len(g.errs) != 1 {
		t.Fatalf("New reported %d failures, want 1: %q", len(g.errs), g.errs)
	}
	msg := g.errs[0]
	for _, want := range []string{"momotest_", "no tables", "outside"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q lacks %q", msg, want)
		}
	}
}

// TestMigrationDropIfExistsDoesNotReachPublic catches a migration pool whose
// search_path still holds public: DROP TABLE IF EXISTS legacy, where the
// migrations never created legacy in the test schema, would resolve to
// public.legacy and drop a real table. Without extensions the migrations see
// the test schema alone.
func TestMigrationDropIfExistsDoesNotReachPublic(t *testing.T) {
	raw := ownDatabase(t)
	admin := connectTo(t, raw)
	if _, err := admin.Exec(t.Context(), "CREATE TABLE public.legacy (id bigint PRIMARY KEY); INSERT INTO public.legacy VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	fsys := fstest.MapFS{
		"20260101000000_drop.up.sql": {Data: []byte(`DROP TABLE IF EXISTS legacy;
CREATE TABLE keep (id bigint PRIMARY KEY);`)},
	}
	legacyRows := func(t *testing.T, when string) {
		t.Helper()
		var n int
		if err := admin.QueryRow(t.Context(), "SELECT count(*) FROM public.legacy").Scan(&n); err != nil || n != 1 {
			t.Fatalf("after %s: public.legacy has %d rows, err %v; a migration dropped a table of public", when, n, err)
		}
	}
	momotest.New(t, momotest.WithEnv(ownURLEnv), momotest.WithMigrations(fsys))
	legacyRows(t, "New")
	momotest.NewTx(t, momotest.WithEnv(ownURLEnv), momotest.WithMigrations(fsys))
	legacyRows(t, "NewTx")
}
