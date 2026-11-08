// SPDX-License-Identifier: Apache-2.0

// Package bunad is the Bun implementation of workload.Impl, on the pgx stdlib
// *sql.DB with pgdialect.
package bunad

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"

	"github.com/azrtydxb/momo/bench/workload"
)

type bunCustomer struct {
	bun.BaseModel `bun:"table:bench_customers,alias:c"`

	ID        int64 `bun:",pk,autoincrement"`
	Email     string
	Country   string
	Status    string
	CreatedAt time.Time   `bun:"created_at"`
	Orders    []*bunOrder `bun:"rel:has-many,join:id=customer_id"`
}

type bunOrder struct {
	bun.BaseModel `bun:"table:bench_orders,alias:o"`

	ID          int64 `bun:",pk,autoincrement"`
	CustomerID  int64 `bun:"customer_id"`
	AmountCents int64 `bun:"amount_cents"`
	Status      string
	CreatedAt   time.Time `bun:"created_at"`
}

type impl struct {
	db *bun.DB
}

// New returns the Bun implementation on db.
func New(db *bun.DB) workload.Impl { return &impl{db: db} }

func (i *impl) Name() string { return "bun" }

func toCustomer(c *bunCustomer) workload.Customer {
	return workload.Customer{ID: c.ID, Email: c.Email, Country: c.Country, Status: c.Status, CreatedAt: c.CreatedAt}
}

func toOrder(o *bunOrder) workload.Order {
	return workload.Order{ID: o.ID, CustomerID: o.CustomerID, AmountCents: o.AmountCents, Status: o.Status, CreatedAt: o.CreatedAt}
}

func (i *impl) CustomerByID(ctx context.Context, id int64) (workload.Customer, error) {
	c := new(bunCustomer)
	err := i.db.NewSelect().Model(c).Where("c.id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return workload.Customer{}, workload.ErrNoRow
	}
	if err != nil {
		return workload.Customer{}, err
	}
	return toCustomer(c), nil
}

func (i *impl) RecentOrders(ctx context.Context, country string, limit int) ([]workload.Order, error) {
	var rows []*bunOrder
	err := i.db.NewSelect().Model(&rows).
		Join("JOIN bench_customers AS c ON c.id = o.customer_id").
		Where("c.country = ?", country).
		OrderExpr("o.created_at DESC, o.id DESC").
		Limit(int64(limit)).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]workload.Order, len(rows))
	for n, r := range rows {
		out[n] = toOrder(r)
	}
	return out, nil
}

func (i *impl) InsertOrder(ctx context.Context, o workload.Order) (int64, error) {
	row := &bunOrder{CustomerID: o.CustomerID, AmountCents: o.AmountCents, Status: o.Status, CreatedAt: o.CreatedAt}
	if _, err := i.db.NewInsert().Model(row).Returning("id").Exec(ctx); err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (i *impl) SetCustomerStatus(ctx context.Context, id int64, status string) (int64, error) {
	res, err := i.db.NewUpdate().Model((*bunCustomer)(nil)).Set("status = ?", status).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (i *impl) CustomersWithOrders(ctx context.Context, country string, limit int) ([]workload.CustomerWithOrders, error) {
	var rows []*bunCustomer
	err := i.db.NewSelect().Model(&rows).
		Where("c.country = ?", country).
		OrderExpr("c.id ASC").
		Limit(int64(limit)).
		Relation("Orders", func(q *bun.SelectQuery) *bun.SelectQuery { return q.Order("id ASC") }).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]workload.CustomerWithOrders, len(rows))
	for n, r := range rows {
		cw := workload.CustomerWithOrders{Customer: toCustomer(r)}
		for _, o := range r.Orders {
			cw.Orders = append(cw.Orders, toOrder(o))
		}
		out[n] = cw
	}
	return out, nil
}

func (i *impl) BulkInsertOrders(ctx context.Context, rows []workload.Order) (int64, error) {
	var total int64
	for start := 0; start < len(rows); start += workload.BulkChunkRows {
		end := min(start+workload.BulkChunkRows, len(rows))
		batch := make([]*bunOrder, 0, end-start)
		for _, o := range rows[start:end] {
			batch = append(batch, &bunOrder{CustomerID: o.CustomerID, AmountCents: o.AmountCents, Status: o.Status, CreatedAt: o.CreatedAt})
		}
		res, err := i.db.NewInsert().Model(&batch).Exec(ctx)
		if err != nil {
			return total, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (i *impl) VectorTopK(context.Context, []float32, int) ([]int64, error) {
	return nil, workload.ErrNotSupported
}
