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

// TestAppRoleExtensionIfNotExists records the answer for NovaCRM: a role that is
// not a superuser and has no CREATE on the database can run the idempotent
// extension statement a generated migration emits when the extension already
// exists, because PostgreSQL checks existence before privileges.
func TestAppRoleExtensionIfNotExists(t *testing.T) {
	_ = testopen.Postgres(t) // creates the schema the app role works in
	app := openAppRoleDB(t)
	if got := firstValue(t, app, `SELECT rolsuper::text FROM pg_roles WHERE rolname = current_user`); got != "false" {
		t.Fatalf("the app role must not be a superuser, rolsuper = %s", got)
	}
	if got := firstValue(t, app, `SELECT has_database_privilege(current_database(), 'CREATE')::text`); got != "false" {
		t.Fatalf("the app role must not have CREATE on the database, got %s", got)
	}
	if err := execDB(t, app, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		t.Fatalf("the idempotent statement for an installed extension: %v", err)
	}
}

// grantSchemaCreate lets the app role create objects in the test schema, as an
// application role that owns its schema can.
func grantSchemaCreate(t *testing.T, admin *momo.DB, app *momo.DB) {
	t.Helper()
	role := firstValue(t, app, `SELECT current_user`)
	mustExecDB(t, admin, `GRANT CREATE ON SCHEMA "`+firstValue(t, admin, `SELECT current_schema()`)+`" TO "`+role+`"`)
}

// ownHistoryAsApp creates the history table as the administrator and hands it
// to the app role.
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

// TestBaselineAsAppRole: a role with CREATE on its schema but not on the
// database baselines a snapshot without table text.
func TestBaselineAsAppRole(t *testing.T) {
	admin := testopen.Postgres(t)
	app := openAppRoleDB(t)
	mustExecDB(t, admin, `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)
	grantSchemaCreate(t, admin, app)
	e := migratetest.NewEngine(t, mpg.New(), app.Driver(), migratetestFS(t, initVersion))
	if err := e.Baseline(context.Background(), initVersion, false); err != nil {
		t.Fatalf("baseline of a snapshot without table text as the app role: %v", err)
	}
}

// TestBaselineAsAppRoleNeedsCreateForTableText: any table text PostgreSQL
// rewrites (a default, a generated column, a check, an index predicate or an
// index expression) needs CREATE on the database, and force does not waive it.
func TestBaselineAsAppRoleNeedsCreateForTableText(t *testing.T) {
	cases := []struct {
		name  string
		ddl   string // the users table in the live database
		apply func(*schema.Table)
	}{
		{"default", `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT DEFAULT '')`,
			func(tb *schema.Table) { tb.Columns[1].Default = "''" }},
		{"generated column", `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT, dbl INTEGER GENERATED ALWAYS AS (id * 2) STORED)`,
			func(tb *schema.Table) {
				tb.Columns = append(tb.Columns, schema.Column{Name: "dbl", Type: "INTEGER", Nullable: true, Generated: "id * 2"})
			}},
		{"check constraint", `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT, CONSTRAINT users_id_pos CHECK (id > 0))`,
			func(tb *schema.Table) { tb.Checks = []schema.Check{{Name: "users_id_pos", Expr: "id > 0"}} }},
		{"index predicate", `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT); CREATE INDEX users_email_part ON users (email) WHERE email IS NOT NULL`,
			func(tb *schema.Table) {
				tb.Indexes = []schema.Index{{Name: "users_email_part", Parts: []schema.IndexPart{{Column: "email"}}, Where: "email IS NOT NULL"}}
			}},
		{"index expression", `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT); CREATE INDEX users_email_lower ON users (lower(email))`,
			func(tb *schema.Table) {
				tb.Indexes = []schema.Index{{Name: "users_email_lower", Parts: []schema.IndexPart{{Expr: "lower(email)"}}}}
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			admin := testopen.Postgres(t)
			app := openAppRoleDB(t)
			for _, stmt := range strings.Split(c.ddl, "; ") {
				mustExecDB(t, admin, stmt)
			}
			grantSchemaCreate(t, admin, app)
			s := usersSchema()
			c.apply(&s.Tables[0])
			e := migratetest.NewEngine(t, mpg.New(), app.Driver(), snapshotFS(t, initVersion, s, c.ddl+";\n"))
			for _, force := range []bool{false, true} {
				if err := e.Baseline(context.Background(), initVersion, force); !errors.Is(err, migrate.ErrCanonicalizePrivilege) {
					t.Fatalf("force=%v: want ErrCanonicalizePrivilege, got %v", force, err)
				}
			}
		})
	}
}

// TestBaselineAsAppRoleNeedsCreateOnSchema: the engine runs CREATE TABLE IF NOT
// EXISTS for the history table, which PostgreSQL refuses without CREATE on the
// schema even when the table exists and the role owns it. Nothing is recorded.
func TestBaselineAsAppRoleNeedsCreateOnSchema(t *testing.T) {
	admin := testopen.Postgres(t)
	app := openAppRoleDB(t)
	mustExecDB(t, admin, `CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`)
	ownHistoryAsApp(t, admin, app)
	e := migratetest.NewEngine(t, mpg.New(), app.Driver(), migratetestFS(t, initVersion))
	err := e.Baseline(context.Background(), initVersion, false)
	if err == nil || !strings.Contains(err.Error(), "permission denied for schema") {
		t.Fatalf("want a permission denied for schema error, got %v", err)
	}
}

// TestApplyAsAppRoleNeedsCreateOnSchema: apply runs the same CREATE TABLE IF NOT
// EXISTS for the history table before the first pending migration, so it is
// refused without CREATE on the schema even when the role owns the table, and
// nothing is recorded or run. docs/migrations.md states this for apply as well
// as baseline.
func TestApplyAsAppRoleNeedsCreateOnSchema(t *testing.T) {
	admin := testopen.Postgres(t)
	app := openAppRoleDB(t)
	ownHistoryAsApp(t, admin, app)
	e := migratetest.NewEngine(t, mpg.New(), app.Driver(), migratetestFS(t, initVersion))
	_, err := e.Apply(context.Background(), migrate.ApplyOptions{})
	if err == nil || !strings.Contains(err.Error(), "permission denied for schema") {
		t.Fatalf("want a permission denied for schema error, got %v", err)
	}
	if got := firstValue(t, admin, `SELECT count(*)::text FROM momo_migrations`); got != "0" {
		t.Fatalf("a refused apply recorded %s history row(s), want 0", got)
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

// TestAutoMigrateSkipsExternalExtension: AutoMigrate creates the table but not
// an extension listed in ExternalExtensions, and creates it without the setting.
// It runs in a scratch database because it creates a database-wide extension.
func TestAutoMigrateSkipsExternalExtension(t *testing.T) {
	desired := func() *schema.Schema {
		return &schema.Schema{Dialect: "postgres", Extensions: []schema.Extension{{Name: "hstore"}}, Tables: []schema.Table{
			{Name: "things", Columns: []schema.Column{{Name: "id", Type: "INTEGER"}}, PrimaryKey: []string{"id"}},
		}}
	}
	installed := func(db *momo.DB) bool {
		return firstValue(t, db, `SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'hstore')::text`) == "true"
	}
	for _, external := range []bool{true, false} {
		db := testopen.PostgresIn(t, testdb.ScratchDatabase(t, "momo_autoext_"))
		var opts []migrate.Option
		if external {
			opts = append(opts, migrate.WithConfig(migrate.Config{ExternalExtensions: []string{"hstore"}}))
		}
		e := migratetest.NewEngine(t, mpg.New(), db.Driver(), migratetest.EmptyFS(), opts...)
		if _, err := e.AutoMigrate(context.Background(), desired()); err != nil {
			t.Fatalf("external=%v: %v", external, err)
		}
		if !tableExists(t, db, "things") {
			t.Fatalf("external=%v: the table must be created", external)
		}
		if got := installed(db); got == external {
			t.Fatalf("external=%v: hstore installed = %v", external, got)
		}
	}
}
