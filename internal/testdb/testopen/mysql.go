// SPDX-License-Identifier: Apache-2.0

package testopen

import (
	"database/sql"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/driver/mysqldriver"
	"github.com/azrtydxb/momo/internal/testdb"
)

// maxMySQLConns is the pool size of the MySQL openers; three, for the same
// reason as maxTestConns.
const maxMySQLConns = 3

// MySQL84 opens a fresh *momo.DB on a database private to t on MySQL 8.4,
// through mysqldriver. opts are passed to momo.New. The pool is closed by
// t.Cleanup before the database is dropped. The test is skipped when
// MOMO_TEST_MYSQL84_URL is unset, and failed when MOMO_REQUIRE_INTEGRATION=1.
func MySQL84(t *testing.T, opts ...momo.Option) *momo.DB {
	t.Helper()
	return openMySQL(t, testdb.MySQL84URL(t), dialect.FlavorMySQL, opts...)
}

// MySQL97 is MySQL84 for MySQL 9.7 (MOMO_TEST_MYSQL97_URL).
func MySQL97(t *testing.T, opts ...momo.Option) *momo.DB {
	t.Helper()
	return openMySQL(t, testdb.MySQL97URL(t), dialect.FlavorMySQL, opts...)
}

// MariaDB is MySQL84 for MariaDB 11.8 (MOMO_TEST_MARIADB_URL).
func MariaDB(t *testing.T, opts ...momo.Option) *momo.DB {
	t.Helper()
	return openMySQL(t, testdb.MariaDBURL(t), dialect.FlavorMariaDB, opts...)
}

// openMySQL opens dsn through mysqldriver.Open with a small pool that t.Cleanup
// closes.
func openMySQL(t *testing.T, dsn string, flavor dialect.Flavor, opts ...momo.Option) *momo.DB {
	t.Helper()
	sdb, drv, err := mysqldriver.Open(dsn, flavor)
	if err != nil {
		t.Fatalf("testopen: open %s: %v", flavor, err)
	}
	sdb.SetMaxOpenConns(maxMySQLConns)
	t.Cleanup(func() { closeSQL(t, sdb) })
	return momo.New(drv, opts...)
}

func closeSQL(t *testing.T, db *sql.DB) {
	t.Helper()
	if err := db.Close(); err != nil {
		t.Errorf("testopen: close pool: %v", err)
	}
}
