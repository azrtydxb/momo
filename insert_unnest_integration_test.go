// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/schema"
)

type bulkRow struct {
	ID int64
	A  int64
	B  string
}

func (r *bulkRow) MomoMeta() *momo.Meta[bulkRow] { return &bulkRowMeta }

var bulkRowMeta = momo.Meta[bulkRow]{
	Table:   "bulk_rows",
	PK:      []int{0},
	Columns: []momo.ColumnMeta{{Name: "id"}, {Name: "a"}, {Name: "b"}},
	Ptr: func(r *bulkRow, i int) any {
		return [...]any{&r.ID, &r.A, &r.B}[i]
	},
	Value: func(r *bulkRow, i int) any {
		return [...]any{r.ID, r.A, r.B}[i]
	},
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

// statementCounter counts the statements that reach the driver.
type statementCounter struct{ n int }

func (c *statementCounter) BeforeQuery(ctx context.Context, _ *momo.QueryEvent) context.Context {
	c.n++
	return ctx
}

func (c *statementCounter) AfterQuery(context.Context, *momo.QueryEvent) {}

// bulkSnapshot returns the table's rows ordered by id.
func bulkSnapshot(t *testing.T, pool *pgxpool.Pool) [][3]any {
	t.Helper()
	rows, err := pool.Query(context.Background(), "SELECT id, a, b FROM bulk_rows ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out [][3]any
	for rows.Next() {
		var id, a int64
		var b string
		if err := rows.Scan(&id, &a, &b); err != nil {
			t.Fatal(err)
		}
		out = append(out, [3]any{id, a, b})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestUnnestBulkInsert inserts 40000 rows of three columns (120000 binds as
// VALUES, over the 65535 limit) through three paths and compares the tables.
func TestUnnestBulkInsert(t *testing.T) {
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "CREATE TABLE bulk_rows (id bigint PRIMARY KEY, a bigint NOT NULL, b text NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	truncate := func() {
		t.Helper()
		if _, err := pool.Exec(ctx, "TRUNCATE bulk_rows"); err != nil {
			t.Fatal(err)
		}
	}

	rows := make([]bulkRow, 40000)
	for i := range rows {
		rows[i] = bulkRow{ID: int64(i + 1), A: int64(i * 3), B: "row"}
	}
	drv := pgxdriver.New(pool)

	// Path 1: chunked VALUES.
	res, err := momo.Insert[bulkRow](momo.New(drv)).Models(rows).Exec(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.RowsAffected != int64(len(rows)) {
		t.Fatalf("VALUES path affected %d rows, want %d", res.RowsAffected, len(rows))
	}
	valuesSnap := bulkSnapshot(t, pool)
	truncate()

	// Path 2: unnest, one statement.
	counter := &statementCounter{}
	res, err = momo.Insert[bulkRow](momo.New(drv, momo.WithHook(counter))).Unnest().Models(rows).Exec(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counter.n != 1 {
		t.Fatalf("unnest path sent %d statements, want 1", counter.n)
	}
	if res.RowsAffected != int64(len(rows)) {
		t.Fatalf("unnest path affected %d rows, want %d", res.RowsAffected, len(rows))
	}
	if got := bulkSnapshot(t, pool); !reflect.DeepEqual(got, valuesSnap) {
		t.Fatal("unnest rows differ from the VALUES rows")
	}
	truncate()

	// Path 3: COPY through the driver's Copier.
	copier, ok := drv.(momo.Copier)
	if !ok {
		t.Fatal("pgx driver does not implement momo.Copier")
	}
	copyRows := make([][]any, len(rows))
	for i, r := range rows {
		copyRows[i] = []any{r.ID, r.A, r.B}
	}
	if _, err := copier.CopyFrom(ctx, "bulk_rows", []string{"id", "a", "b"}, copyRows); err != nil {
		t.Fatal(err)
	}
	if got := bulkSnapshot(t, pool); !reflect.DeepEqual(got, valuesSnap) {
		t.Fatal("COPY rows differ from the VALUES rows")
	}
	if len(valuesSnap) != len(rows) {
		t.Fatalf("VALUES path wrote %d rows, want %d", len(valuesSnap), len(rows))
	}
}
