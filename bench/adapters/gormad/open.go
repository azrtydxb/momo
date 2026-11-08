// SPDX-License-Identifier: Apache-2.0

// Package gormad is the GORM implementation of workload.Impl.
package gormad

import (
	"database/sql"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/azrtydxb/momo/bench/adapters"
)

// OpenDB opens GORM on the pgx stdlib driver with the shared pool settings and
// no implicit transaction around single writes, as Momo does not add one. The
// returned function closes the underlying pool.
func OpenDB(url string) (*gorm.DB, func() error, error) {
	sqlDB, err := sql.Open("pgx", url)
	if err != nil {
		return nil, nil, err
	}
	adapters.ApplySQLDBSettings(sqlDB)
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		_ = sqlDB.Close()
		return nil, nil, err
	}
	return db, sqlDB.Close, nil
}

// Open is OpenDB for tests: it fails the test on error and closes the pool at
// cleanup.
func Open(tb testing.TB, url string) *gorm.DB {
	tb.Helper()
	db, closeDB, err := OpenDB(url)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		if err := closeDB(); err != nil {
			tb.Error(err)
		}
	})
	return db
}
