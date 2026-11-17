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
	if code, _, errOut = cliRun(t, dsn, "baseline", "--version", version); code != 0 {
		t.Fatalf("baseline after the repair: exit %d: %s", code, errOut)
	}
}
