// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func mustExecPG(t *testing.T, q momo.Querier, sql string, args ...any) {
	t.Helper()
	if _, err := momo.Exec(context.Background(), q, sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func pgPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testdb.PostgresURL(t))
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func queryIDs(t *testing.T, pool *pgxpool.Pool, c momo.Compiled) []int64 {
	t.Helper()
	rows, err := pool.Query(context.Background(), c.SQL, c.Args...)
	if err != nil {
		t.Fatalf("query %q: %v", c.SQL, err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			t.Fatalf("values: %v", err)
		}
		id, ok := vals[0].(int64)
		if !ok {
			t.Fatalf("first column is %T, want int64", vals[0])
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func queryIDFloat(t *testing.T, pool *pgxpool.Pool, c momo.Compiled) map[int64]float64 {
	t.Helper()
	rows, err := pool.Query(context.Background(), c.SQL, c.Args...)
	if err != nil {
		t.Fatalf("query %q: %v", c.SQL, err)
	}
	defer rows.Close()
	out := map[int64]float64{}
	for rows.Next() {
		var id int64
		var v float64
		if err := rows.Scan(&id, &v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[id] = v
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
