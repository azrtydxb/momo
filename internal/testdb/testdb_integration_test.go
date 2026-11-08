// SPDX-License-Identifier: Apache-2.0

//go:build integration

package testdb

import (
	"testing"
)

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
