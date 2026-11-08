// SPDX-License-Identifier: Apache-2.0

//go:build integration

package bench

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/schema"
)

type bulkRowBench struct {
	ID int64
	A  int64
	B  string
}

func (r *bulkRowBench) MomoMeta() *momo.Meta[bulkRowBench] { return &bulkRowBenchMeta }

var bulkRowBenchMeta = momo.Meta[bulkRowBench]{
	Table:   "bulk_rows",
	PK:      []int{0},
	Columns: []momo.ColumnMeta{{Name: "id"}, {Name: "a"}, {Name: "b"}},
	Ptr:     func(r *bulkRowBench, i int) any { return [...]any{&r.ID, &r.A, &r.B}[i] },
	Value:   func(r *bulkRowBench, i int) any { return [...]any{r.ID, r.A, r.B}[i] },
	TableSchema: &schema.Table{
		Name: "bulk_rows",
		Columns: []schema.Column{
			{Name: "id", Type: "bigint"},
			{Name: "a", Type: "bigint"},
			{Name: "b", Type: "text"},
		},
		PrimaryKey: []string{"id"},
	},
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

// BenchmarkBulkInsert compares the three PostgreSQL bulk paths at 1000, 10000
// and 40000 rows: chunked multi-row VALUES, COPY, and one unnest statement.
// The table is truncated outside the timer.
func BenchmarkBulkInsert(b *testing.B) {
	ctx := context.Background()
	url := os.Getenv("BENCH_PG_URL")
	if url == "" {
		b.Skip("BENCH_PG_URL is not set")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		b.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "DROP TABLE IF EXISTS bulk_rows"); err != nil {
		b.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "CREATE TABLE bulk_rows (id bigint PRIMARY KEY, a bigint NOT NULL, b text NOT NULL)"); err != nil {
		b.Fatal(err)
	}
	drv := pgxdriver.New(pool)
	copier, ok := drv.(momo.Copier)
	if !ok {
		b.Fatal("pgx driver does not implement momo.Copier")
	}
	for _, size := range []int{1000, 10000, 40000} {
		rows := make([]bulkRowBench, size)
		copyRows := make([][]any, size)
		for i := range rows {
			rows[i] = bulkRowBench{ID: int64(i + 1), A: int64(i), B: "row"}
			copyRows[i] = []any{rows[i].ID, rows[i].A, rows[i].B}
		}
		for _, path := range []string{"values", "copy", "unnest"} {
			b.Run(fmt.Sprintf("%s/%d", path, size), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					b.StopTimer()
					if _, err := pool.Exec(ctx, "TRUNCATE bulk_rows"); err != nil {
						b.Fatal(err)
					}
					b.StartTimer()
					var err error
					switch path {
					case "values":
						_, err = momo.Insert[bulkRowBench](momo.New(drv)).Models(rows).Exec(ctx)
					case "unnest":
						_, err = momo.Insert[bulkRowBench](momo.New(drv)).Unnest().Models(rows).Exec(ctx)
					case "copy":
						_, err = copier.CopyFrom(ctx, "bulk_rows", []string{"id", "a", "b"}, copyRows)
					}
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
