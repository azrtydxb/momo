// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momotest_test

import (
	"context"
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

// issue115Migrations creates its objects with IF NOT EXISTS on purpose: with
// public also holding them, they must still be created in the test schema.
func issue115Migrations() fstest.MapFS {
	return fstest.MapFS{
		"20260101000000_issue115.up.sql": {Data: []byte(
			`CREATE TABLE IF NOT EXISTS issue115_items (id bigint PRIMARY KEY, name text NOT NULL);
CREATE INDEX IF NOT EXISTS issue115_items_name ON issue115_items (name);`)},
	}
}

// publicAdmin opens an administrator connection to the momotest server, and
// skips the test when the URL is unset.
func publicAdmin(t *testing.T) (*pgx.Conn, string) {
	t.Helper()
	raw := os.Getenv(momotest.DefaultEnv)
	if raw == "" {
		t.Skip(momotest.DefaultEnv + " is not set")
	}
	conn, err := pgx.Connect(t.Context(), raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(context.WithoutCancel(t.Context())) })
	return conn, raw
}

// dropPublic removes what these tests put in public, even when a test fails.
func dropPublic(t *testing.T, raw string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.WithoutCancel(t.Context())
		conn, err := pgx.Connect(ctx, raw)
		if err != nil {
			t.Errorf("cleanup public: %v", err)
			return
		}
		defer conn.Close(ctx)
		for _, tbl := range []string{"public.issue115_items", "public.issue115_guard", "public.momo_migrations"} {
			if _, err := conn.Exec(ctx, "DROP TABLE IF EXISTS "+tbl); err != nil {
				t.Errorf("cleanup %s: %v", tbl, err)
			}
		}
	})
}

// migratePublic applies fsys to the public schema, as a CI job running
// momo migrate apply on the same database does.
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
	admin, raw := publicAdmin(t)
	var exists bool
	if err := admin.QueryRow(t.Context(), "SELECT to_regclass('public.momo_migrations') IS NOT NULL").Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Skip("public already holds momo_migrations; not touching it")
	}
	dropPublic(t, raw)
	migratePublic(t, raw, issue115Migrations())

	insert := func(t *testing.T, q momo.Querier) {
		t.Helper()
		if _, err := momo.Exec(t.Context(), q, "INSERT INTO issue115_items (id, name) VALUES (1, 'x')"); err != nil {
			t.Fatalf("insert the same primary key: %v", err)
		}
	}
	for _, name := range []string{"first", "second"} {
		t.Run("New/"+name, func(t *testing.T) {
			db := momotest.New(t, momotest.WithMigrations(issue115Migrations()))
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
		tx := momotest.NewTx(t, momotest.WithMigrations(issue115Migrations()))
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
	_, raw := publicAdmin(t)
	dropPublic(t, raw)
	fsys := fstest.MapFS{
		"20260101000000_guard.up.sql": {Data: []byte(`CREATE TABLE public.issue115_guard (id bigint PRIMARY KEY);`)},
	}
	g := &guardTB{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		momotest.New(g, momotest.WithMigrations(fsys))
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
