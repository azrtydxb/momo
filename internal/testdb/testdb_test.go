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

func TestEnvVarNames(t *testing.T) {
	want := map[string]string{
		envPostgres:    "MOMO_TEST_POSTGRES_URL",
		envPostgresApp: "MOMO_TEST_POSTGRES_APP_URL",
		envPgBouncer:   "MOMO_TEST_PGBOUNCER_URL",
		envMySQL84:     "MOMO_TEST_MYSQL84_URL",
		envMySQL97:     "MOMO_TEST_MYSQL97_URL",
		envMariaDB:     "MOMO_TEST_MARIADB_URL",
	}
	for got, name := range want {
		if got != name {
			t.Errorf("variable name %q, want %q", got, name)
		}
	}
}

func TestSQLitePathIsInTempDir(t *testing.T) {
	if p := SQLitePath(t); filepath.Base(p) != "momo.db" {
		t.Fatalf("SQLitePath = %q, want a momo.db file", p)
	}
}

func TestRandomSuffixIsUniqueAndSafe(t *testing.T) {
	a, b := randomName(), randomName()
	if a == b || len(a) != 18 || !strings.HasPrefix(a, "t_") || strings.ContainsAny(a, `"'; `) {
		t.Fatalf("randomName gave %q and %q", a, b)
	}
}
