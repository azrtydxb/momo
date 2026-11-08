// SPDX-License-Identifier: Apache-2.0

// Package adapters holds the connection settings shared by every adapter, so
// each compared library runs against the same server with the same limits.
package adapters

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	poolMaxConns        = 16
	poolMinConns        = 16
	poolMaxConnLifetime = time.Hour
)

// NewPool opens a pgxpool on url with MaxConns and MinConns 16, MaxConnLifetime
// 1h and QueryExecModeCacheStatement, and pings it. It fails on an empty url.
func NewPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	if url == "" {
		return nil, errors.New("adapters: the PostgreSQL URL is empty (set BENCH_PG_URL)")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = poolMaxConns
	cfg.MinConns = poolMinConns
	cfg.MaxConnLifetime = poolMaxConnLifetime
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheStatement
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// ApplySQLDBSettings applies the same limits to a *sql.DB: MaxOpenConns and
// MaxIdleConns 16, ConnMaxLifetime 1h.
func ApplySQLDBSettings(db *sql.DB) {
	db.SetMaxOpenConns(poolMaxConns)
	db.SetMaxIdleConns(poolMaxConns)
	db.SetConnMaxLifetime(poolMaxConnLifetime)
}
