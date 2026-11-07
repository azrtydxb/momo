// SPDX-License-Identifier: Apache-2.0

//go:build integration

package migrate_test

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
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

// openAppRoleDB opens a pool as the non-superuser application role, which has
// no CREATE on the database, on the schema the admin connection of the test uses.
func openAppRoleDB(t *testing.T) *momo.DB {
	t.Helper()
	pool, err := pgxdriver.NewPool(context.Background(), testdb.PostgresAppRoleURL(t))
	if err != nil {
		t.Fatalf("open app role pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return momo.New(pgxdriver.New(pool))
}

// TestAppRoleCreateExtensionIfNotExists records the answer for NovaCRM: a role
// that is not a superuser and has no CREATE on the database can run
// CREATE EXTENSION IF NOT EXISTS for an extension that already exists, because
// PostgreSQL checks existence before privileges. For one that does not exist it
// fails, so a migration that creates a missing extension needs a privileged role.
func TestAppRoleCreateExtensionIfNotExists(t *testing.T) {
	_ = testopen.Postgres(t) // creates the schema the app role works in
	app := openAppRoleDB(t)
	if got := firstValue(t, app, `SELECT rolsuper::text FROM pg_roles WHERE rolname = current_user`); got != "false" {
		t.Fatalf("the app role must not be a superuser, rolsuper = %s", got)
	}
	if got := firstValue(t, app, `SELECT has_database_privilege(current_database(), 'CREATE')::text`); got != "false" {
		t.Fatalf("the app role must not have CREATE on the database, got %s", got)
	}
	if err := execDB(t, app, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		t.Fatalf("CREATE EXTENSION IF NOT EXISTS vector for an existing extension: %v", err)
	}
	err := execDB(t, app, `CREATE EXTENSION IF NOT EXISTS momo_no_such_extension`)
	if err == nil {
		t.Fatal("creating an extension that does not exist must fail")
	}
	t.Logf("missing extension as the app role: %v", err)
}

// ownHistoryAsApp creates the history table as the administrator and hands it
// to the app role, because the app role cannot create objects and the engine
// refuses a history table owned by an unrelated role.
func ownHistoryAsApp(t *testing.T, admin, app *momo.DB) {
	t.Helper()
	role := firstValue(t, app, `SELECT current_user`)
	withPinned(t, admin, func(c momo.PinnedConn) {
		if err := migrate.EnsureHistory(context.Background(), c, mpg.New()); err != nil {
			t.Fatal(err)
		}
	})
	mustExecDB(t, admin, `ALTER TABLE momo_migrations OWNER TO "`+role+`"`)
}

// TestBaselineAsAppRole: without text in the snapshot's tables, baseline needs
// no CREATE on the database.
func TestBaselineAsAppRole(t *testing.T) {
	admin := testopen.Postgres(t)
	app := openAppRoleDB(t)
	mustExecDB(t, admin, `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)
	ownHistoryAsApp(t, admin, app)
	e := migratetest.NewEngine(t, mpg.New(), app.Driver(), migratetestFS(t, initVersion))
	if err := e.Baseline(context.Background(), initVersion, false); err != nil {
		t.Fatalf("baseline of a snapshot without table text as the app role: %v", err)
	}
}

// TestBaselineAsAppRoleNeedsCreateForTableText: a default, check or index
// predicate in any table is enough to need CREATE on the database, and --force
// does not waive it.
func TestBaselineAsAppRoleNeedsCreateForTableText(t *testing.T) {
	admin := testopen.Postgres(t)
	app := openAppRoleDB(t)
	mustExecDB(t, admin, `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT DEFAULT '')`)
	ownHistoryAsApp(t, admin, app)
	s := usersSchema()
	s.Tables[0].Columns[1].Default = "''"
	fsys := snapshotFS(t, initVersion, s, "CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT DEFAULT '');\n")
	e := migratetest.NewEngine(t, mpg.New(), app.Driver(), fsys)
	for _, force := range []bool{false, true} {
		err := e.Baseline(context.Background(), initVersion, force)
		if !errors.Is(err, migrate.ErrCanonicalizePrivilege) {
			t.Fatalf("force=%v: want ErrCanonicalizePrivilege, got %v", force, err)
		}
	}
}

// TestBaselineAsAppRoleWithoutHistoryTable: the app role cannot create the
// history table, so the baseline fails before it records anything.
func TestBaselineAsAppRoleWithoutHistoryTable(t *testing.T) {
	admin := testopen.Postgres(t)
	app := openAppRoleDB(t)
	mustExecDB(t, admin, `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)
	e := migratetest.NewEngine(t, mpg.New(), app.Driver(), migratetestFS(t, initVersion))
	err := e.Baseline(context.Background(), initVersion, false)
	if err == nil {
		t.Fatal("baseline must fail when the role cannot create the history table")
	}
	t.Logf("baseline without a history table as the app role: %v", err)
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("want a permission error, got %v", err)
	}
}

// noVectorSnapshot is usersSchema without the vector extension, which the live
// database has.
func noVectorSnapshot() *schema.Schema {
	s := usersSchema()
	s.Extensions = nil
	return s
}

// TestExternalExtensionsHiddenFromBaselineAndDrift: an extension created outside
// momo and listed in Config.ExternalExtensions is neither a baseline mismatch
// nor drift, and without the setting it is both.
func TestExternalExtensionsHiddenFromBaselineAndDrift(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExecDB(t, db, `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)
	fsys := snapshotFS(t, initVersion, noVectorSnapshot(), "CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT);\n")

	plain := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
	if err := plain.Baseline(ctx, initVersion, false); !errors.Is(err, migrate.ErrBaselineMismatch) {
		t.Fatalf("without the setting the live vector extension is unmanaged: %v", err)
	}

	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys, migrate.WithConfig(migrate.Config{ExternalExtensions: []string{"vector"}}))
	if err := e.Baseline(ctx, initVersion, false); err != nil {
		t.Fatalf("baseline with vector external: %v", err)
	}
	rep, err := e.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Clean() {
		t.Fatalf("drift with vector external: %+v", rep)
	}
	rep, err = plain.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Clean() {
		t.Fatal("drift without the setting must report the vector extension as unmanaged")
	}
}
