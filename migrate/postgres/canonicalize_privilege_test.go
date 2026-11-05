// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

// openAppRole opens a pool as the non-superuser application role, which has no
// CREATE on the database, on the schema the admin connection of db uses.
func openAppRole(t *testing.T) *momo.DB {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxdriver.NewPool(ctx, testdb.PostgresAppRoleURL(t))
	if err != nil {
		t.Fatalf("open app role pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return momo.New(pgxdriver.New(pool))
}

// TestCanonicalizeNeedsCreateOnDatabase: a role without CREATE on the database
// gets a typed error naming the privilege, not a raw "permission denied", and
// nothing is created.
func TestCanonicalizeNeedsCreateOnDatabase(t *testing.T) {
	admin := testopen.Postgres(t)
	app := openAppRole(t)
	ctx := context.Background()
	createDeclaredLive(t, admin.Driver())
	pc := pinned(t, app)
	err := mpg.New().Canonicalize(ctx, pc, declaredObjects())
	if !errors.Is(err, migrate.ErrCanonicalizePrivilege) {
		t.Fatalf("want ErrCanonicalizePrivilege, got %v", err)
	}
	for _, want := range []string{"CREATE", "database", "privilege"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error must mention %q: %v", want, err)
		}
	}
}

// TestCanonicalizeWithoutObjectsNeedsNoPrivilege: a schema with nothing to
// canonicalize runs no DDL, so the app role is not refused.
func TestCanonicalizeWithoutObjectsNeedsNoPrivilege(t *testing.T) {
	admin := testopen.Postgres(t)
	app := openAppRole(t)
	mustExec(t, admin.Driver(), `CREATE TABLE t (id int)`)
	s := &schema.Schema{Dialect: "postgres", Tables: []schema.Table{{Name: "t", Columns: []schema.Column{{Name: "id", Type: "int"}}}}}
	if err := mpg.New().Canonicalize(context.Background(), pinned(t, app), s); err != nil {
		t.Fatal(err)
	}
}
