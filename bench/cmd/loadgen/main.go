// SPDX-License-Identifier: Apache-2.0

// Command loadgen runs the mixed workload against one adapter and prints one
// JSON line with throughput and latency percentiles.
//
//	BENCH_PG_URL=postgres://... go run ./cmd/loadgen -impl momo -concurrency 16
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	entdialect "entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/bench/adapters"
	"github.com/azrtydxb/momo/bench/adapters/bunad"
	entad "github.com/azrtydxb/momo/bench/adapters/ent"
	"github.com/azrtydxb/momo/bench/adapters/ent/entgen"
	"github.com/azrtydxb/momo/bench/adapters/gormad"
	momoad "github.com/azrtydxb/momo/bench/adapters/momo"
	"github.com/azrtydxb/momo/bench/adapters/pgxhand"
	"github.com/azrtydxb/momo/bench/adapters/sqlhand"
	"github.com/azrtydxb/momo/bench/loadgen"
	"github.com/azrtydxb/momo/bench/workload"
	"github.com/azrtydxb/momo/driver/pgxdriver"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "loadgen:", err)
		os.Exit(1)
	}
}

func run() error {
	implName := flag.String("impl", "pgx", "adapter: pgx, sql, momo, gorm, ent or bun")
	concurrency := flag.Int("concurrency", 8, "worker goroutines")
	ops := flag.Int("ops", 20000, "total operations")
	readRatio := flag.Float64("read-ratio", 0.8, "fraction of read operations")
	seed := flag.Int64("seed", 42, "random seed")
	out := flag.String("out", "", "append the JSON line to this file instead of stdout")
	flag.Parse()

	ctx := context.Background()
	url := os.Getenv("BENCH_PG_URL")
	pool, err := adapters.NewPool(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := workload.Seed(ctx, pool, 200, 5); err != nil {
		return err
	}
	sqlDB, err := sql.Open("pgx", url)
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()
	adapters.ApplySQLDBSettings(sqlDB)

	var impl workload.Impl
	cfg := loadgen.Config{Concurrency: *concurrency, Ops: *ops, ReadRatio: *readRatio, Seed: *seed}
	switch *implName {
	case "pgx":
		impl = pgxhand.New(pool)
		cfg.PoolWaitNS = func() int64 { return pool.Stat().EmptyAcquireWaitTime().Nanoseconds() }
	case "sql":
		impl = sqlhand.New(sqlDB)
	case "momo":
		impl = momoad.New(momo.New(pgxdriver.New(pool)))
	case "gorm":
		db, closeDB, err := gormad.OpenDB(url)
		if err != nil {
			return err
		}
		defer func() { _ = closeDB() }()
		impl = gormad.New(db)
	case "ent":
		impl = entad.New(entgen.NewClient(entgen.Driver(entsql.OpenDB(entdialect.Postgres, sqlDB))))
	case "bun":
		impl = bunad.New(bun.NewDB(sqlDB, pgdialect.New()))
	default:
		return fmt.Errorf("unknown -impl %q", *implName)
	}

	res, err := loadgen.Run(ctx, impl, cfg)
	if err != nil {
		return err
	}
	line, err := json.Marshal(res)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if *out == "" {
		_, err = os.Stdout.Write(line)
		return err
	}
	f, err := os.OpenFile(*out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 -- operator-chosen output path
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.Write(line)
	return err
}
