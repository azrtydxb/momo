// SPDX-License-Identifier: Apache-2.0

package testdb

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	set := func(string) (string, bool) { return "postgres://x", true }
	unset := func(string) (string, bool) { return "", false }

	if v, err := resolve(envPostgres, set, true); err != nil || v != "postgres://x" {
		t.Fatalf("set variable: %q, %v", v, err)
	}
	if _, err := resolve(envPostgres, unset, false); err != errSkip {
		t.Fatalf("unset, not required: err=%v, want errSkip", err)
	}
	_, err := resolve(envPostgres, unset, true)
	if err == nil || err == errSkip || !strings.Contains(err.Error(), envPostgres) {
		t.Fatalf("unset, required: err=%v, want an error naming the variable", err)
	}
}

func TestDialectRequired(t *testing.T) {
	cases := []struct {
		env, list string
		want      bool
	}{
		{envPostgres, "", true},
		{envMySQL84, "", true},
		{envPostgres, "postgres", true},
		{envPgBouncer, "postgres", true},
		{envMySQL84, "postgres", false},
		{envMariaDB, "mysql84, mariadb", true},
		{envMySQL97, "mysql84,mariadb", false},
		{envPostgres, "mysql84", false},
	}
	for _, c := range cases {
		if got := dialectRequired(c.env, c.list); got != c.want {
			t.Errorf("dialectRequired(%s, %q) = %v, want %v", c.env, c.list, got, c.want)
		}
	}
}

func TestSQLitePathIsInTempDir(t *testing.T) {
	p := SQLitePath(t)
	if filepath.Base(p) != "momo.db" {
		t.Fatalf("SQLitePath = %q, want a momo.db file", p)
	}
	// t.TempDir returns a fresh numbered dir under one per-test root, so the
	// grandparent of the database file must be that root.
	got, err := filepath.EvalSymlinks(filepath.Dir(filepath.Dir(p)))
	if err != nil {
		t.Fatalf("resolve %q: %v", p, err)
	}
	want, err := filepath.EvalSymlinks(filepath.Dir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("SQLitePath = %q, not under the test temp root %q", p, want)
	}
}

func TestRandomSuffixIsUniqueAndSafe(t *testing.T) {
	a, b := randomName(), randomName()
	if a == b || len(a) != 18 || !strings.HasPrefix(a, "t_") || strings.ContainsAny(a, `"'; `) {
		t.Fatalf("randomName gave %q and %q", a, b)
	}
}

func TestSearchPathRejectsNonURL(t *testing.T) {
	got, err := searchPath("postgres://u:p@h:5432/db?sslmode=disable", "t_ab")
	if err != nil || !strings.Contains(got, "search_path=t_ab%2Cpublic") || !strings.Contains(got, "sslmode=disable") {
		t.Fatalf("valid URL: %q, %v", got, err)
	}
	if _, err := searchPath("postgresql://h/db", "t_ab"); err != nil {
		t.Fatalf("postgresql scheme: %v", err)
	}
	for _, bad := range []string{"host=localhost user=u dbname=d", "mysql://u@h/db", "%zz", ""} {
		if _, err := searchPath(bad, "t_ab"); err == nil || !strings.Contains(err.Error(), "postgres://") {
			t.Errorf("searchPath(%q) err = %v, want an error requiring a postgres:// URL", bad, err)
		}
	}
}
