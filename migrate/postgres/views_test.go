// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// openOrder maps the open_orders view; Meta.View makes every write builder
// refuse it.
type openOrder struct {
	ID    int64
	Total int64
}

var openOrderMeta = &momo.Meta[openOrder]{
	Table: "open_orders",
	View:  true,
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK},
		{Name: "total"},
	},
	Ptr:      func(m *openOrder, i int) any { return [...]any{&m.ID, &m.Total}[i] },
	Value:    func(m *openOrder, i int) any { return [...]any{m.ID, m.Total}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*openOrder) MomoMeta() *momo.Meta[openOrder] { return openOrderMeta }

// orderTotal maps the order_totals materialized view.
type orderTotal struct {
	Status string
	Total  int64
}

var orderTotalMeta = &momo.Meta[orderTotal]{
	Table: "order_totals",
	View:  true,
	Columns: []momo.ColumnMeta{
		{Name: "status", Flags: momo.FlagPK},
		{Name: "total"},
	},
	Ptr:      func(m *orderTotal, i int) any { return [...]any{&m.Status, &m.Total}[i] },
	Value:    func(m *orderTotal, i int) any { return [...]any{m.Status, m.Total}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*orderTotal) MomoMeta() *momo.Meta[orderTotal] { return orderTotalMeta }

func ordersSchema(openQuery string, withViews bool) *schema.Schema {
	s := &schema.Schema{
		Dialect: "postgres",
		Tables: []schema.Table{{Name: "orders", Columns: []schema.Column{
			{Name: "id", Type: "bigint"}, {Name: "status", Type: "text"}, {Name: "total", Type: "bigint"},
		}, PrimaryKey: []string{"id"}}},
	}
	if withViews {
		s.Views = []schema.View{
			{Name: "open_orders", Query: openQuery, Depends: []string{"orders"}},
			// big_orders reads a view, so the engine must create it after open_orders,
			// drop it before open_orders, and rebuild it when open_orders is replaced.
			{Name: "big_orders", Query: "SELECT id, total FROM open_orders WHERE total > 5", Depends: []string{"open_orders"}},
			{Name: "order_totals", Materialized: true, Query: "SELECT status, sum(total)::bigint AS total FROM orders GROUP BY status", Depends: []string{"orders"}},
		}
	}
	return s
}

func countRows(t *testing.T, db *momo.DB, sql string) int {
	t.Helper()
	rows, err := db.Driver().Query(context.Background(), sql, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var n int
	if !rows.Next() {
		t.Fatalf("%s: no row", sql)
	}
	if err := rows.Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestViewsAsModels creates a view, a view over a view and a materialized
// view through migration statements, queries two of them as read-only models,
// refreshes the materialized view with RefreshMaterializedView, proves a
// write builder is refused with ErrValidation, replaces the base view with its
// dependent rebuilt, and drops everything in dependency order (a wrong order
// makes PostgreSQL refuse a step).
func TestViewsAsModels(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	empty := &schema.Schema{Dialect: "postgres"}
	openQuery := "SELECT id, total FROM orders WHERE status = 'open'"
	base := ordersSchema(openQuery, true)

	// Created: the table first, the view over it, then the view over that view.
	applyStatements(t, db, planStatements(t, empty, base, migrate.DiffOptions{}))
	mustExec(t, db.Driver(), `INSERT INTO orders (id, status, total) VALUES (1, 'open', 10), (2, 'closed', 5), (3, 'open', 3)`)

	// Queried as a read-only model.
	open, err := momo.Select[openOrder](db).OrderBy(momo.NewColumn[int64]("open_orders", "id", 0).Asc()).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 2 || open[0].ID != 1 || open[1].ID != 3 {
		t.Fatalf("open_orders must return orders 1 and 3, got %+v", open)
	}

	// The materialized view holds the data of its creation (an empty table)
	// until it is refreshed.
	if n := countRows(t, db, `SELECT count(*) FROM order_totals`); n != 0 {
		t.Fatalf("order_totals must be stale before the refresh, got %d rows", n)
	}
	if err := momo.RefreshMaterializedView(ctx, db, "order_totals", false); err != nil {
		t.Fatal(err)
	}
	totals, err := momo.Select[orderTotal](db).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(totals) != 2 {
		t.Fatalf("order_totals must have two groups after the refresh, got %+v", totals)
	}
	mustExec(t, db.Driver(), `INSERT INTO orders (id, status, total) VALUES (4, 'open', 7)`)
	if err := momo.RefreshMaterializedView(ctx, db, "order_totals", false); err != nil {
		t.Fatal(err)
	}
	totals, err = momo.Select[orderTotal](db).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tot := range totals {
		if tot.Status == "open" && tot.Total != 20 {
			t.Fatalf("open total after the second refresh = %d, want 20", tot.Total)
		}
	}

	// A write builder on a view model fails before any statement is sent.
	row := openOrder{ID: 9, Total: 1}
	if _, err := momo.Insert[openOrder](db).Model(&row).Exec(ctx); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("Insert on a view model: err = %v, want ErrValidation", err)
	}
	if n := countRows(t, db, `SELECT count(*) FROM orders`); n != 4 {
		t.Fatalf("orders has %d rows, want 4", n)
	}

	// Replaced: open_orders is dropped and recreated, and so is big_orders
	// that reads it; the dependent still works afterwards.
	changed := ordersSchema(openQuery+" AND total > 0", true)
	applyStatements(t, db, planStatements(t, base, changed, migrate.DiffOptions{}))
	if n := countRows(t, db, `SELECT count(*) FROM big_orders`); n != 2 {
		t.Fatalf("big_orders after the replace has %d rows, want 2 (orders 1 and 4)", n)
	}

	// Dropped: the views go before the table they read, the view over a view
	// before its source.
	applyStatements(t, db, planStatements(t, changed, ordersSchema("", false), migrate.DiffOptions{}))
	got := inspectNow(t, db)
	if len(got.Views) != 0 || len(got.Tables) != 1 || got.Tables[0].Name != "orders" {
		t.Fatalf("after the drop: %d views, tables %+v", len(got.Views), got.Tables)
	}
}
