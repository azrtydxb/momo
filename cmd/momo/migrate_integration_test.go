// SPDX-License-Identifier: Apache-2.0

//go:build integration

package main

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib" // the pgx database/sql driver

	testdb "github.com/azrtydxb/momo/internal/testdb"
)

// cliRun runs `momo migrate` with the per-test database URL in MOMO_TEST_URL.
func cliRun(t *testing.T, dsn string, args ...string) (code int, out, errOut string) {
	t.Helper()
	var o, e bytes.Buffer
	code = runMigrate(&o, &e, args, func(k string) string {
		if k == "MOMO_TEST_URL" {
			return dsn
		}
		return ""
	})
	return code, o.String(), e.String()
}

// simpleModels is a one-table model package whose live schema round-trips
// without drift.
const simpleModels = `package simple

import "github.com/azrtydxb/momo"

// Tenant is the only model.
type Tenant struct {
	momo.Model ` + "`momo:\"table:tenants\"`" + `
	ID         int64  ` + "`momo:\"pk,identity\"`" + `
	Name       string ` + "`momo:\"size:200,notnull,unique\"`" + `
}
`

// simpleModule copies simpleModels into a temporary module and returns its root.
func simpleModule(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "models.go"), simpleModels)
	return copyModule(t, src)
}

// TestMigrateLifecycle drives diff, validate, plan, apply, status and drift
// through the CLI against PostgreSQL, in a per-test schema.
func TestMigrateLifecycle(t *testing.T) {
	dsn := testdb.PostgresURL(t)
	root := simpleModule(t)
	// The test database carries the pgvector extension, which no migration owns.
	writeFile(t, filepath.Join(root, "momo.yaml"),
		"version: 1\nmigrate:\n  dir:\n    postgres: migrations/postgres\n  url_env:\n    postgres: MOMO_TEST_URL\n  exclude: [vector]\n")
	chdir(t, root)

	// diff writes the migration files and prints their paths relative to cwd.
	code, out, errOut := cliRun(t, dsn, "diff", "init")
	if code != 0 {
		t.Fatalf("diff: exit %d: %s", code, errOut)
	}
	for _, want := range []string{"_init.up.sql", "_init.snapshot.json", "momo.sum"} {
		if !strings.Contains(out, "migrations/postgres/") || !strings.Contains(out, want) {
			t.Fatalf("diff output lacks a relative path for %s:\n%s", want, out)
		}
	}
	if strings.Contains(out, root) {
		t.Fatalf("diff printed an absolute path:\n%s", out)
	}
	// A second diff against the new snapshot finds nothing.
	if code, out, _ = cliRun(t, dsn, "diff", "again"); code != 0 || !strings.Contains(out, "no changes") {
		t.Fatalf("second diff: exit %d, out %q", code, out)
	}

	if code, _, errOut = cliRun(t, dsn, "validate"); code != 0 {
		t.Fatalf("validate: exit %d: %s", code, errOut)
	}
	if code, out, errOut = cliRun(t, dsn, "plan"); code != 0 || !strings.Contains(out, "pending") {
		t.Fatalf("plan: exit %d, out %q, err %q", code, out, errOut)
	}
	if code, out, errOut = cliRun(t, dsn, "apply"); code != 0 || !strings.Contains(out, "applied") {
		t.Fatalf("apply: exit %d, out %q, err %q", code, out, errOut)
	}
	if code, out, errOut = cliRun(t, dsn, "status"); code != 0 || !strings.Contains(out, "applied") {
		t.Fatalf("status: exit %d, out %q, err %q", code, out, errOut)
	}
	if code, out, errOut = cliRun(t, dsn, "drift"); code != 0 {
		t.Fatalf("drift after apply: exit %d, out %q, err %q", code, out, errOut)
	}
	if code, out, _ = cliRun(t, dsn, "apply", "--lock-wait", "1s"); code != 0 || !strings.Contains(out, "no pending") {
		t.Fatalf("second apply: exit %d, out %q", code, out)
	}
	if code, _, errOut = cliRun(t, dsn, "validate", "--db"); code != 0 {
		t.Fatalf("validate --db: exit %d: %s", code, errOut)
	}

	// An edited applied file is a checksum mismatch: apply exits 3 naming it,
	// validate exits 1.
	files, err := filepath.Glob(filepath.Join(root, "migrations", "postgres", "*_init.up.sql"))
	if err != nil || len(files) != 1 {
		t.Fatalf("glob: %v %v", files, err)
	}
	b, err := os.ReadFile(files[0]) //nolint:gosec // test path
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files[0], append(b, []byte("\n-- edited\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, _ = cliRun(t, dsn, "validate"); code != 1 {
		t.Fatalf("validate after edit: exit %d, want 1", code)
	}
	if code, _, errOut = cliRun(t, dsn, "apply"); code != 3 || !strings.Contains(errOut, "_init.up.sql") {
		t.Fatalf("apply after edit: exit %d, want 3 naming the file: %s", code, errOut)
	}
}

// TestMigrateBaselineAndResolve covers the exit codes of baseline and resolve.
func TestMigrateBaselineAndResolve(t *testing.T) {
	dsn := testdb.PostgresURL(t)
	root := copyModule(t, exampleSrc)
	writeFile(t, filepath.Join(root, "momo.yaml"), "version: 1\nmigrate:\n  dir:\n    postgres: m\n  url_env:\n    postgres: MOMO_TEST_URL\n")
	chdir(t, root)
	if code, _, errOut := cliRun(t, dsn, "diff", "init"); code != 0 {
		t.Fatalf("diff: exit %d: %s", code, errOut)
	}
	snaps, _ := filepath.Glob(filepath.Join(root, "m", "*_init.snapshot.json"))
	if len(snaps) != 1 {
		t.Fatalf("snapshots: %v", snaps)
	}
	version := strings.SplitN(filepath.Base(snaps[0]), "_", 2)[0]
	// The database is empty, so it does not match the snapshot: exit 1.
	if code, _, errOut := cliRun(t, dsn, "baseline", "--version", version); code != 1 {
		t.Fatalf("baseline on a mismatching database: exit %d, want 1: %s", code, errOut)
	}
	// Nothing is dirty, so resolving is a database-side refusal: exit 3.
	if code, _, _ := cliRun(t, dsn, "resolve", "--version", version, "--applied"); code != 3 {
		t.Fatalf("resolve of a clean version: exit %d, want 3", code)
	}
}

func TestCLIExitOneOnDrift(t *testing.T) {
	// testdb.PostgresURL(t) is memoized per t: the stray table and the CLI use the same per-test schema
	// (search_path=t_xxx,public), so the CLI's default managed schema current_schema() is that schema.
	dsn := testdb.PostgresURL(t)
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if _, err := sqlDB.ExecContext(context.Background(), `CREATE TABLE stray (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cfg := yamlPath(t, dir, "migrate:\n  dir:\n    postgres: "+filepath.ToSlash(dir)+"/migrations/postgres\n  url_env:\n    postgres: MOMO_TEST_URL\n")
	mkdirAll(t, dir+"/migrations/postgres")
	var out, errOut bytes.Buffer
	code := runMigrate(&out, &errOut, []string{"--config", cfg, "drift"}, func(k string) string {
		if k == "MOMO_TEST_URL" {
			return dsn
		}
		return ""
	})
	if code != 1 {
		t.Fatalf("drift with an unmanaged table must exit 1, got %d: %s", code, errOut.String())
	}
}
