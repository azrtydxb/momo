// SPDX-License-Identifier: Apache-2.0

//go:build integration

package cli

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib" // the pgx database/sql driver

	testdb "github.com/azrtydxb/momo/internal/testdb"
)

// TestMigrateDriftFixSQL adopts a database that a migration built and that was
// then changed outside it (a column lost its NOT NULL, a stray table appeared),
// without momo history. Catches: the baseline refusal and drift not naming the
// column, --fix-sql printing a destructive drop unasked or exiting 0 while
// withholding one, and a printed script that does not make drift clean.
func TestMigrateDriftFixSQL(t *testing.T) {
	dsn := testdb.PostgresURL(t)
	root := simpleModule(t)
	writeFile(t, filepath.Join(root, "momo.yaml"),
		"version: 1\nmigrate:\n  dir:\n    postgres: migrations/postgres\n  url_env:\n    postgres: MOMO_TEST_URL\n  exclude: [vector]\n")
	chdir(t, root)
	if code, _, errOut := cliRun(t, dsn, "diff", "init"); code != 0 {
		t.Fatalf("diff: exit %d: %s", code, errOut)
	}
	if code, _, errOut := cliRun(t, dsn, "apply"); code != 0 {
		t.Fatalf("apply: exit %d: %s", code, errOut)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	for _, stmt := range []string{
		`ALTER TABLE tenants ALTER COLUMN name DROP NOT NULL`,
		`CREATE TABLE stray (id integer)`,
		`DROP TABLE momo_migrations`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	snaps, _ := filepath.Glob(filepath.Join(root, "migrations", "postgres", "*_init.snapshot.json"))
	if len(snaps) != 1 {
		t.Fatalf("snapshots: %v", snaps)
	}
	version := strings.SplitN(filepath.Base(snaps[0]), "_", 2)[0]

	// The refusal names the differing objects.
	code, _, errOut := cliRun(t, dsn, "baseline", "--version", version)
	if code != 1 || !strings.Contains(errOut, "changed: column tenants.name") || !strings.Contains(errOut, "unmanaged: table stray") {
		t.Fatalf("baseline: exit %d, want 1 naming column tenants.name and table stray:\n%s", code, errOut)
	}

	// drift without history compares with the latest snapshot.
	code, out, _ := cliRun(t, dsn, "drift")
	if code != 1 || !strings.Contains(out, "changed: column tenants.name") || !strings.Contains(out, "unmanaged: table stray") || strings.Contains(out, "unmanaged: table tenants") {
		t.Fatalf("drift: exit %d, want 1 naming column tenants.name and table stray only:\n%s", code, out)
	}

	// --fix-sql withholds the drop of the stray table: exit 1, the reason on stderr.
	code, out, errOut = cliRun(t, dsn, "drift", "--fix-sql")
	if code != 1 || !strings.Contains(out, "SET NOT NULL") || strings.Contains(strings.ToLower(out), "drop table") {
		t.Fatalf("drift --fix-sql: exit %d, want 1 and the NOT NULL repair without a drop:\n%s", code, out)
	}
	if !strings.Contains(errOut, "withheld: drop_table stray") || !strings.Contains(errOut, "--allow-destructive") {
		t.Fatalf("drift --fix-sql did not list the withheld step on stderr:\n%s", errOut)
	}

	// --allow-destructive is only valid together with --fix-sql.
	if code, _, _ = cliRun(t, dsn, "drift", "--allow-destructive"); code != 2 {
		t.Fatalf("--allow-destructive without --fix-sql: exit %d, want 2", code)
	}

	// With --allow-destructive the whole repair is printed and makes drift clean.
	code, out, errOut = cliRun(t, dsn, "drift", "--fix-sql", "--allow-destructive")
	if code != 0 || !strings.Contains(strings.ToLower(out), "drop table") {
		t.Fatalf("drift --fix-sql --allow-destructive: exit %d, want 0 with the drop:\n%s\n%s", code, out, errOut)
	}
	if _, err := db.ExecContext(ctx, out); err != nil {
		t.Fatalf("running the script: %v\n%s", err, out)
	}
	if code, out, _ = cliRun(t, dsn, "drift"); code != 0 || !strings.Contains(out, "no drift") {
		t.Fatalf("drift after the repair: exit %d:\n%s", code, out)
	}
	// A clean database prints no SQL, so standard output stays pipeable.
	if code, out, errOut = cliRun(t, dsn, "drift", "--fix-sql"); code != 0 || out != "" || !strings.Contains(errOut, "no drift") {
		t.Fatalf("drift --fix-sql on a clean database: exit %d, stdout %q, stderr %q; want 0, empty stdout, no drift on stderr", code, out, errOut)
	}
	// --version names the snapshot; stderr says which one was compared.
	if code, _, errOut = cliRun(t, dsn, "drift", "--version", version); code != 0 || !strings.Contains(errOut, "snapshot of version "+version) {
		t.Fatalf("drift --version %s: exit %d, stderr %q", version, code, errOut)
	}
	if code, _, _ = cliRun(t, dsn, "drift", "--version", "29990101000000"); code != 2 {
		t.Fatalf("drift --version of a missing snapshot: exit %d, want 2", code)
	}
	if code, _, errOut = cliRun(t, dsn, "baseline", "--version", version); code != 0 {
		t.Fatalf("baseline after the repair: exit %d: %s", code, errOut)
	}
}

// fixProject builds the tenants table by migration in a scratch database (so a
// test may create an extension), applies it and returns the URL and a handle.
func fixProject(t *testing.T) (dsn string, db *sql.DB) {
	t.Helper()
	dsn = testdb.PostgresURLFor(t, testdb.ScratchDatabase(t, "momo_cli_fix_"))
	root := simpleModule(t)
	writeFile(t, filepath.Join(root, "momo.yaml"),
		"version: 1\nmigrate:\n  dir:\n    postgres: migrations/postgres\n  url_env:\n    postgres: MOMO_TEST_URL\n  exclude: [vector]\n")
	chdir(t, root)
	if code, _, errOut := cliRun(t, dsn, "diff", "init"); code != 0 {
		t.Fatalf("diff: exit %d: %s", code, errOut)
	}
	if code, _, errOut := cliRun(t, dsn, "apply"); code != 0 {
		t.Fatalf("apply: exit %d: %s", code, errOut)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return dsn, db
}

// TestMigrateDriftFixSQLExitCodes pins when --fix-sql exits 1 although it
// printed statements, and when 0. Catches: exit 0 while a NOT NULL column is
// only staged nullable, exit 0 while a difference no statement covers remains,
// and exit 1 for a repair that is complete.
func TestMigrateDriftFixSQLExitCodes(t *testing.T) {
	ctx := context.Background()

	t.Run("staged", func(t *testing.T) {
		dsn, db := fixProject(t)
		if _, err := db.ExecContext(ctx, `ALTER TABLE tenants DROP COLUMN name`); err != nil {
			t.Fatal(err)
		}
		code, out, errOut := cliRun(t, dsn, "drift", "--fix-sql", "--allow-destructive")
		if code != 1 || !strings.Contains(strings.ToUpper(out), "ADD COLUMN") {
			t.Fatalf("exit %d, want 1 with the column added:\n%s\n%s", code, out, errOut)
		}
		if !strings.Contains(errOut, "staged: tenants.name") || !strings.Contains(errOut, "SET NOT NULL") {
			t.Fatalf("stderr does not list the staged column and the statement to run after the backfill:\n%s", errOut)
		}
	})

	t.Run("uncovered", func(t *testing.T) {
		dsn, db := fixProject(t)
		if _, err := db.ExecContext(ctx, `CREATE EXTENSION pgcrypto`); err != nil {
			t.Fatal(err)
		}
		code, _, errOut := cliRun(t, dsn, "drift", "--fix-sql", "--allow-destructive")
		if code != 1 || !strings.Contains(errOut, "not covered: unmanaged: extension pgcrypto") {
			t.Fatalf("exit %d, want 1 naming the extension no statement covers:\n%s", code, errOut)
		}
	})

	t.Run("complete", func(t *testing.T) {
		dsn, db := fixProject(t)
		if _, err := db.ExecContext(ctx, `CREATE TABLE stray (id integer)`); err != nil {
			t.Fatal(err)
		}
		if code, _, errOut := cliRun(t, dsn, "drift", "--fix-sql"); code != 1 || !strings.Contains(errOut, "withheld: drop_table stray") {
			t.Fatalf("without --allow-destructive: exit %d, want 1 listing the withheld drop:\n%s", code, errOut)
		}
		code, out, errOut := cliRun(t, dsn, "drift", "--fix-sql", "--allow-destructive")
		if code != 0 || !strings.Contains(strings.ToLower(out), "drop table") {
			t.Fatalf("with --allow-destructive: exit %d, want 0 with the drop:\n%s\n%s", code, out, errOut)
		}
	})
}
