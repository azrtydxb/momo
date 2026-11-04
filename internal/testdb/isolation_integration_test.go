//go:build integration

// SPDX-License-Identifier: Apache-2.0

package testdb

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func searchPathSchema(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	name, rest, ok := strings.Cut(u.Query().Get("search_path"), ",")
	if !ok || rest != "public" || !strings.HasPrefix(name, "t_") || len(name) != 18 {
		t.Fatalf("search_path = %q, want t_<16 hex>,public", u.Query().Get("search_path"))
	}
	return name
}

func TestPostgresURLIsolated(t *testing.T) {
	ctx := context.Background()
	admin := require(t, envPostgres)

	var seen []string
	for _, sub := range []string{"first", "second"} {
		t.Run(sub, func(t *testing.T) {
			u1, u2 := PostgresURL(t), PostgresURL(t)
			if u1 != u2 {
				t.Fatalf("same t returned two URLs: %q and %q", u1, u2)
			}
			name := searchPathSchema(t, u1)
			if name != Schema(t) {
				t.Fatalf("Schema(t) = %q, URL search_path schema = %q", Schema(t), name)
			}
			for _, prev := range seen {
				if prev == name {
					t.Fatalf("schema %q reused by a second test", name)
				}
			}
			seen = append(seen, name)

			conn, err := pgx.Connect(ctx, u1)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close(ctx)
			var current string
			if err := conn.QueryRow(ctx, "SELECT current_schema()").Scan(&current); err != nil || current != name {
				t.Fatalf("current_schema() = %q, %v; want %q", current, err, name)
			}
			if _, err := conn.Exec(ctx, "CREATE TABLE docs (id int PRIMARY KEY)"); err != nil {
				t.Fatalf("CREATE TABLE docs in %s: %v", name, err)
			}
		})
	}

	if len(seen) != 2 || seen[0] == seen[1] {
		t.Fatalf("subtests shared a schema: %v", seen)
	}
	t.Run("app role cannot create but can read", func(t *testing.T) {
		adminURL := PostgresURL(t)
		appURL := PostgresAppRoleURL(t)
		name := searchPathSchema(t, appURL)
		ac, err := pgx.Connect(ctx, adminURL)
		if err != nil {
			t.Fatal(err)
		}
		defer ac.Close(ctx)
		if _, err := ac.Exec(ctx, "CREATE TABLE admin_made (id int PRIMARY KEY)"); err != nil {
			t.Fatal(err)
		}
		app, err := pgx.Connect(ctx, appURL)
		if err != nil {
			t.Fatal(err)
		}
		defer app.Close(ctx)
		_, err = app.Exec(ctx, "CREATE TABLE app_made (id int)")
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Fatalf("app role CREATE TABLE in %s: err = %v, want SQLSTATE 42501", name, err)
		}
		var n int
		if err := app.QueryRow(ctx, "SELECT count(*) FROM admin_made").Scan(&n); err != nil {
			t.Fatalf("app role SELECT on an admin-created table: %v", err)
		}
	})
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	for _, name := range seen {
		var n int
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM pg_namespace WHERE nspname = $1", name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("schema %s still exists after its test finished", name)
		}
	}
}
