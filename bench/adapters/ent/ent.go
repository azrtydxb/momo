// SPDX-License-Identifier: Apache-2.0

// Package ent is the Ent implementation of workload.Impl. The Ent client code
// is generated into ./entgen by make bench-generate.
package ent

import (
	"context"

	"entgo.io/ent/dialect/sql"

	"github.com/azrtydxb/momo/bench/adapters/ent/entgen"
	"github.com/azrtydxb/momo/bench/adapters/ent/entgen/customer"
	"github.com/azrtydxb/momo/bench/adapters/ent/entgen/order"
	"github.com/azrtydxb/momo/bench/workload"
)

type impl struct {
	client *entgen.Client
}

// New returns the Ent implementation on client.
func New(client *entgen.Client) workload.Impl { return &impl{client: client} }

func (i *impl) Name() string { return "ent" }

func toCustomer(c *entgen.Customer) workload.Customer {
	return workload.Customer{ID: int64(c.ID), Email: c.Email, Country: c.Country, Status: c.Status, CreatedAt: c.CreatedAt}
}

func toOrder(o *entgen.Order) workload.Order {
	return workload.Order{
		ID: int64(o.ID), CustomerID: int64(o.CustomerID), AmountCents: o.AmountCents,
		Status: o.Status, CreatedAt: o.CreatedAt,
	}
}

func (i *impl) CustomerByID(ctx context.Context, id int64) (workload.Customer, error) {
	c, err := i.client.Customer.Get(ctx, int(id))
	if entgen.IsNotFound(err) {
		return workload.Customer{}, workload.ErrNoRow
	}
	if err != nil {
		return workload.Customer{}, err
	}
	return toCustomer(c), nil
}

func (i *impl) RecentOrders(ctx context.Context, country string, limit int) ([]workload.Order, error) {
	rows, err := i.client.Order.Query().
		Where(order.HasCustomerWith(customer.CountryEQ(country))).
		Order(order.ByCreatedAt(sql.OrderDesc()), order.ByID(sql.OrderDesc())).
		Limit(limit).
		All(ctx)
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
	row, err := i.client.Order.Create().
		SetCustomerID(int(o.CustomerID)).
		SetAmountCents(o.AmountCents).
		SetStatus(o.Status).
		SetCreatedAt(o.CreatedAt).
		Save(ctx)
	if err != nil {
		return 0, err
	}
	return int64(row.ID), nil
}

func (i *impl) SetCustomerStatus(ctx context.Context, id int64, status string) (int64, error) {
	n, err := i.client.Customer.Update().Where(customer.ID(int(id))).SetStatus(status).Save(ctx)
	return int64(n), err
}

func (i *impl) CustomersWithOrders(ctx context.Context, country string, limit int) ([]workload.CustomerWithOrders, error) {
	rows, err := i.client.Customer.Query().
		Where(customer.CountryEQ(country)).
		Order(customer.ByID()).
		Limit(limit).
		WithOrders(func(q *entgen.OrderQuery) { q.Order(order.ByID()) }).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]workload.CustomerWithOrders, len(rows))
	for n, r := range rows {
		cw := workload.CustomerWithOrders{Customer: toCustomer(r)}
		for _, o := range r.Edges.Orders {
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
		builders := make([]*entgen.OrderCreate, 0, end-start)
		for _, o := range rows[start:end] {
			builders = append(builders, i.client.Order.Create().
				SetCustomerID(int(o.CustomerID)).
				SetAmountCents(o.AmountCents).
				SetStatus(o.Status).
				SetCreatedAt(o.CreatedAt))
		}
		saved, err := i.client.Order.CreateBulk(builders...).Save(ctx)
		if err != nil {
			return total, err
		}
		total += int64(len(saved))
	}
	return total, nil
}

func (i *impl) VectorTopK(context.Context, []float32, int) ([]int64, error) {
	return nil, workload.ErrNotSupported
}
