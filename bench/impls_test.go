// SPDX-License-Identifier: Apache-2.0

//go:build integration

package bench

import (
	"database/sql"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/bench/adapters"
	bunad "github.com/azrtydxb/momo/bench/adapters/bunad"
	ent "github.com/azrtydxb/momo/bench/adapters/ent"
	"github.com/azrtydxb/momo/bench/adapters/ent/entgen"
	"github.com/azrtydxb/momo/bench/adapters/gormad"
	momoad "github.com/azrtydxb/momo/bench/adapters/momo"
	"github.com/azrtydxb/momo/bench/adapters/pgxhand"
	"github.com/azrtydxb/momo/bench/adapters/sqlhand"
	"github.com/azrtydxb/momo/bench/workload"
	"github.com/azrtydxb/momo/driver/pgxdriver"
)

// openAllImpls returns the six adapters in this order: pgxhand (the reference),
// sqlhand, momo, gorm, ent, bun. The equivalence test and the micro benchmarks
// both use it, so every comparison runs the same constructors.
func openAllImpls(tb testing.TB, url string, pool *pgxpool.Pool) []workload.Impl {
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
	entClient := entgen.NewClient(entgen.Driver(entsql.OpenDB(dialect.Postgres, sqlDB)))
	return []workload.Impl{
		pgxhand.New(pool),
		sqlhand.New(sqlDB),
		momoad.New(momo.New(pgxdriver.New(pool))),
		gormad.New(gormad.Open(tb, url)),
		ent.New(entClient),
		bunad.New(bun.NewDB(sqlDB, pgdialect.New())),
	}
}
