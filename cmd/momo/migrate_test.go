// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	testdb "github.com/azrtydxb/momo/internal/testdb"
	_ "github.com/jackc/pgx/v5/stdlib" // the pgx database/sql driver
)

func noEnv(string) string { return "" }

// yamlPath writes a momo.yaml with body (version 1 header added) into dir.
func yamlPath(t *testing.T, dir, body string) string {
	t.Helper()
	p := filepath.Join(dir, "momo.yaml")
	writeFile(t, p, "version: 1\n"+body)
	return p
}

func TestCLIContracts(t *testing.T) {
	dir := t.TempDir()
	cfg := yamlPath(t, dir, "migrate:\n  dir:\n    sqlite: "+filepath.ToSlash(dir)+"/migrations/sqlite\n")
	var out, errOut bytes.Buffer
	code := runMigrate(&out, &errOut, []string{"--config", cfg, "validate"}, noEnv)
	if code != 0 {
		t.Fatalf("validate on an empty directory must exit 0, got %d: %s", code, errOut.String())
	}
	code = runMigrate(&out, &errOut, []string{"plan"}, noEnv)
	if code != 2 {
		t.Fatalf("plan without a database URL is a configuration error (2), got %d", code)
	}
	code = runMigrate(&out, &errOut, []string{"nosuch"}, noEnv)
	if code != 2 {
		t.Fatalf("unknown subcommand must exit 2, got %d", code)
	}
}

func TestOfflineCommandsNoDatabase(t *testing.T) {
	dir := t.TempDir()
	cfg := yamlPath(t, dir, "migrate:\n  dir:\n    sqlite: "+filepath.ToSlash(dir)+"/migrations/sqlite\n")
	asked := false
	poison := func(k string) string {
		asked = true
		if k == "MOMO_DATABASE_URL" {
			return "postgres://nobody:secret@127.0.0.1:1/none?sslmode=disable"
		}
		return ""
	}
	var out, errOut bytes.Buffer
	for _, args := range [][]string{{"validate"}, {"lint"}, {"rebase"}} {
		code := runMigrate(&out, &errOut, append([]string{"--config", cfg}, args...), poison)
		if code == 3 {
			t.Fatalf("%v opened a connection (exit 3): %s", args, errOut.String())
		}
	}
	if asked {
		t.Fatal("an offline command read the database URL environment variable")
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

// Flag parsing and exit-code unit tests that need no database.

func TestMigrateUsageErrors(t *testing.T) {
	dir := t.TempDir()
	cfg := yamlPath(t, dir, "migrate:\n  dir:\n    postgres: "+filepath.ToSlash(dir)+"/m\n")
	const u = "postgres://u:p@127.0.0.1:1/x"
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"no subcommand", nil, 2},
		{"bad flag", []string{"--config", cfg, "validate", "--nosuch"}, 2},
		{"missing config file", []string{"--config", filepath.Join(dir, "none.yaml"), "validate"}, 2},
		{"diff without name", []string{"--config", cfg, "diff"}, 2},
		{"baseline without version", []string{"--config", cfg, "baseline", "--url", u}, 2},
		{"resolve needs a state", []string{"--config", cfg, "resolve", "--version", "20260101000000", "--url", u}, 2},
		{"resolve states are exclusive", []string{"--config", cfg, "resolve", "--version", "1", "--applied", "--rolled-back", "--url", u}, 2},
		{"bad lock wait", []string{"--config", cfg, "apply", "--lock-wait", "soon", "--url", u}, 2},
		{"unknown dialect", []string{"--config", cfg, "--dialect", "oracle", "validate"}, 2},
		{"not available yet", []string{"--config", cfg, "rollback"}, 2},
	}
	for _, c := range cases {
		var out, errOut bytes.Buffer
		if got := runMigrate(&out, &errOut, c.args, noEnv); got != c.want {
			t.Errorf("%s: exit %d, want %d (%s)", c.name, got, c.want, errOut.String())
		}
	}
}

func TestMigrateValidateExitOneOnProblems(t *testing.T) {
	dir := t.TempDir()
	mdir := filepath.Join(dir, "m")
	cfg := yamlPath(t, dir, "migrate:\n  dir:\n    postgres: "+filepath.ToSlash(mdir)+"\n")
	writeFile(t, filepath.Join(mdir, "20260101000000_a.up.sql"), "CREATE TABLE a (id int);\n")
	var out, errOut bytes.Buffer
	if code := runMigrate(&out, &errOut, []string{"--config", cfg, "validate"}, noEnv); code != 1 {
		t.Fatalf("a directory without momo.sum must exit 1, got %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String()+errOut.String(), "momo.sum") {
		t.Fatalf("problem output must name momo.sum: %q %q", out.String(), errOut.String())
	}
}

func TestMigrateRedactsURL(t *testing.T) {
	dir := t.TempDir()
	cfg := yamlPath(t, dir, "migrate:\n  dir:\n    postgres: "+filepath.ToSlash(dir)+"/m\n")
	var out, errOut bytes.Buffer
	url := "postgres://alice:hunter2secret@127.0.0.1:1/none?sslmode=disable&connect_timeout=1"
	code := runMigrate(&out, &errOut, []string{"--config", cfg, "status", "--url", url}, noEnv)
	if code != 3 {
		t.Fatalf("unreachable database must exit 3, got %d: %s", code, errOut.String())
	}
	if all := out.String() + errOut.String(); strings.Contains(all, "hunter2secret") || strings.Contains(all, url) {
		t.Fatalf("output leaks the DSN: %s", all)
	}
}
