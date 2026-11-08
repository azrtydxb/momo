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

// Open opens GORM on the pgx stdlib driver with the shared pool settings and
// no implicit transaction around single writes, as Momo does not add one.
func Open(tb testing.TB, url string) *gorm.DB {
	tb.Helper()
	sqlDB, err := sql.Open("pgx", url)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			tb.Error(err)
		}
	})
	adapters.ApplySQLDBSettings(sqlDB)
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		tb.Fatal(err)
	}
	return db
}
