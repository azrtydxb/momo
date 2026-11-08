// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"strings"
	"testing"

	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
)

// TestAppRoleCannotDDL: the application role of the least-privilege split runs
// queries on migrated tables but cannot create objects. CI must run the
// integration suite with MOMO_TEST_POSTGRES_APP_URL set, so this test fails
// instead of skipping when the role is missing. The harness
// (internal/testdb.PostgresAppRoleURL) gives the role USAGE on the per-test
// schema and default privileges on the tables the administrator creates in it,
// and deliberately no CREATE; this test grants nothing itself.
func TestAppRoleCannotDDL(t *testing.T) {
	admin := testopen.Postgres(t)
	// The role's default privileges are installed when it is opened, so open
	// it before the administrator creates the table.
	app := openAppRole(t)
	mustExec(t, admin.Driver(), `CREATE TABLE app_visible (x int)`)
	mustExec(t, admin.Driver(), `INSERT INTO app_visible VALUES (1)`)

	ctx := context.Background()
	c := pinned(t, app)

	var super bool
	mustQueryRow(t, c, `SELECT rolsuper FROM pg_roles WHERE rolname = current_user`, &super)
	if super {
		t.Fatal("the application role must not be a superuser")
	}

	// DML on a migrated table works.
	var n int
	mustQueryRow(t, c, `SELECT count(*) FROM app_visible`, &n)
	if n != 1 {
		t.Fatalf("the application role must read migrated tables, count = %d", n)
	}

	// The search path starts at the per-test schema, where the role has no
	// CREATE, so an unqualified CREATE TABLE is refused.
	for _, ddl := range []string{
		`CREATE TABLE should_fail (x int)`,
		`ALTER TABLE app_visible ADD COLUMN y int`,
		`DROP TABLE app_visible`,
	} {
		_, err := c.Exec(ctx, ddl, nil)
		if err == nil || !strings.Contains(err.Error(), "permission denied") && !strings.Contains(err.Error(), "must be owner") {
			t.Errorf("the application role must not run %q (want permission denied or must be owner), got %v", ddl, err)
		}
	}
}
