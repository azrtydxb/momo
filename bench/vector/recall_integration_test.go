// SPDX-License-Identifier: Apache-2.0

//go:build integration

package vector

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// pgSweepDB adapts a one-connection pool to DB. The pool has one connection, so
// the session setting WithSearchEf sets is the one every query sees.
type pgSweepDB struct {
	pool  *pgxpool.Pool
	table string
}

func (d pgSweepDB) TopK(ctx context.Context, query []float32, k int) ([]int64, error) {
	// The id tie-break keeps the exact result deterministic.
	sql := fmt.Sprintf("SELECT id FROM %s ORDER BY embedding <-> $1::vector, id LIMIT $2", d.table)
	rows, err := d.pool.Query(ctx, sql, VectorText(query), k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (d pgSweepDB) set(ctx context.Context, name, value string) error {
	_, err := d.pool.Exec(ctx, "SELECT set_config($1, $2, false)", name, value)
	return err
}

func (d pgSweepDB) WithSearchEf(ctx context.Context, ef int, fn func() error) error {
	if err := d.set(ctx, "hnsw.ef_search", strconv.Itoa(ef)); err != nil {
		return err
	}
	ferr := fn()
	if err := d.set(ctx, "hnsw.ef_search", "40"); err != nil && ferr == nil {
		return err
	}
	return ferr
}

func (d pgSweepDB) WithExact(ctx context.Context, fn func() error) error {
	for _, name := range []string{"enable_indexscan", "enable_bitmapscan"} {
		if err := d.set(ctx, name, "off"); err != nil {
			return err
		}
	}
	ferr := fn()
	for _, name := range []string{"enable_indexscan", "enable_bitmapscan"} {
		if err := d.set(ctx, name, "on"); err != nil && ferr == nil {
			return err
		}
	}
	return ferr
}

// oneConnPool opens a pool with a single connection on BENCH_PG_URL.
func oneConnPool(tb testing.TB) *pgxpool.Pool {
	tb.Helper()
	url := os.Getenv("BENCH_PG_URL")
	if url == "" {
		tb.Skip("BENCH_PG_URL is not set")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		tb.Fatal(err)
	}
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(pool.Close)
	return pool
}

// loadItems recreates table with an HNSW index on the dataset, 1000 rows per
// statement. The embedding column is named embedding.
func loadItems(tb testing.TB, ctx context.Context, pool *pgxpool.Pool, table string, ds Dataset) {
	tb.Helper()
	for _, stmt := range []string{
		"CREATE EXTENSION IF NOT EXISTS vector",
		"DROP TABLE IF EXISTS " + table,
		fmt.Sprintf("CREATE TABLE %s (id bigint PRIMARY KEY, embedding vector(%d) NOT NULL)", table, ds.Dim),
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			tb.Fatal(err)
		}
	}
	insert := fmt.Sprintf("INSERT INTO %s (id, embedding) SELECT i, e::vector FROM unnest($1::bigint[], $2::text[]) AS t(i, e)", table)
	const chunk = 1000
	for start := 0; start < len(ds.Vectors); start += chunk {
		end := min(start+chunk, len(ds.Vectors))
		ids := make([]int64, 0, end-start)
		texts := make([]string, 0, end-start)
		for i := start; i < end; i++ {
			ids = append(ids, int64(i+1))
			texts = append(texts, VectorText(ds.Vectors[i]))
		}
		if _, err := pool.Exec(ctx, insert, ids, texts); err != nil {
			tb.Fatal(err)
		}
	}
	index := fmt.Sprintf("CREATE INDEX %s_embedding ON %s USING hnsw (embedding vector_l2_ops)", table, table)
	if _, err := pool.Exec(ctx, index); err != nil {
		tb.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "ANALYZE "+table); err != nil {
		tb.Fatal(err)
	}
}

func TestRecallMatchedSweep(t *testing.T) {
	ctx := context.Background()
	pool := oneConnPool(t)
	ds := Generate(2000, 50, 32, 7)
	loadItems(t, ctx, pool, "recall_items", ds)

	db := pgSweepDB{pool: pool, table: "recall_items"}
	efs := []int{10, 20, 40, 80, 160, 320, 640}
	ef1, recall, err := Sweep(ctx, db, ds, 10, 0.95, efs)
	if err != nil {
		t.Fatal(err)
	}
	if recall < 0.95 {
		t.Fatalf("recall@10 = %.3f at ef_search %d, want at least 0.95", recall, ef1)
	}
	ef2, _, err := Sweep(ctx, db, ds, 10, 0.95, efs)
	if err != nil {
		t.Fatal(err)
	}
	if ef1 != ef2 {
		t.Fatalf("sweep is not deterministic: %d then %d", ef1, ef2)
	}
}
