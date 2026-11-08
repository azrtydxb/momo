// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

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
		{"new needs --custom", []string{"--config", cfg, "new", "x"}, 2},
		{"new --custom needs a name", []string{"--config", cfg, "new", "--custom"}, 2},
		{"new --custom needs a snapshot", []string{"--config", cfg, "new", "--custom", "x"}, 2},
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

func TestCLIRedactsURL(t *testing.T) {
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

	// A URL the driver cannot parse is echoed, password included, by the
	// driver's own error (net/url quotes the raw string). The unreachable host
	// above never reaches such an error, so only this case can observe a
	// missing redaction: the output must show the error was reported and still
	// hide the password.
	out.Reset()
	errOut.Reset()
	bad := "postgres://bob:p4ssw0rdleak@127.0.0.1:notaport/none?sslmode=disable"
	if code := runMigrate(&out, &errOut, []string{"--config", cfg, "status", "--url", bad}, noEnv); code == 0 {
		t.Fatalf("an unparsable URL must fail, got exit 0: %s", out.String())
	}
	all := out.String() + errOut.String()
	if !strings.Contains(all, "port") {
		t.Fatalf("the parse error must be reported (it names the port), got %q", all)
	}
	if strings.Contains(all, "p4ssw0rdleak") || strings.Contains(all, bad) {
		t.Fatalf("output leaks the DSN of an unparsable URL: %s", all)
	}

	// A missing URL names the environment variable to set.
	out.Reset()
	errOut.Reset()
	if code := runMigrate(&out, &errOut, []string{"--config", cfg, "status"}, noEnv); code == 0 {
		t.Fatalf("status without a URL must fail: %s", out.String())
	}
	if !strings.Contains(errOut.String(), "MOMO_DATABASE_URL") {
		t.Fatalf("missing URL must name MOMO_DATABASE_URL: %q", errOut.String())
	}
}
