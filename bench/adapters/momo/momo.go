// SPDX-License-Identifier: Apache-2.0

// Package momoad is the Momo implementation of workload.Impl.
package momoad

import (
	"context"
	"errors"

	"github.com/azrtydxb/momo"
	models "github.com/azrtydxb/momo/bench/models/momo"
	"github.com/azrtydxb/momo/bench/models/momo/customer"
	"github.com/azrtydxb/momo/bench/models/momo/order"
	"github.com/azrtydxb/momo/bench/workload"
)

type impl struct {
	db *momo.DB
}

// New returns the Momo implementation on db.
func New(db *momo.DB) workload.Impl { return &impl{db: db} }

func (i *impl) Name() string { return "momo" }

func toCustomer(c models.Customer) workload.Customer {
	return workload.Customer{ID: c.ID, Email: c.Email, Country: c.Country, Status: c.Status, CreatedAt: c.CreatedAt}
}

func toOrder(o models.Order) workload.Order {
	return workload.Order{ID: o.ID, CustomerID: o.CustomerID, AmountCents: o.AmountCents, Status: o.Status, CreatedAt: o.CreatedAt}
}

func (i *impl) CustomerByID(ctx context.Context, id int64) (workload.Customer, error) {
	c, err := momo.Select[models.Customer](i.db).Where(customer.ID.Eq(id)).One(ctx)
	if errors.Is(err, momo.ErrNotFound) {
		return workload.Customer{}, workload.ErrNoRow
	}
	if err != nil {
		return workload.Customer{}, err
	}
	return toCustomer(c), nil
}

func (i *impl) RecentOrders(ctx context.Context, country string, limit int) ([]workload.Order, error) {
	rows, err := momo.Select[models.Order](i.db).
		Join(momo.TableRef{Name: "bench_customers"}, order.CustomerID.EqExpr(customer.ID)).
		Where(customer.Country.Eq(country)).
		OrderBy(order.CreatedAt.Desc(), order.ID.Desc()).
		Limit(int64(limit)).
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
	row := models.Order{CustomerID: o.CustomerID, AmountCents: o.AmountCents, Status: o.Status, CreatedAt: o.CreatedAt}
	rows, err := momo.Insert[models.Order](i.db).Model(&row).Returning(order.ID).Rows(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := rows.ScanModel(&row); err != nil {
			return 0, err
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (i *impl) SetCustomerStatus(ctx context.Context, id int64, status string) (int64, error) {
	res, err := momo.Update[models.Customer](i.db).
		Set(customer.Status.Set(status)).
		Where(customer.ID.Eq(id)).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected, nil
}

func (i *impl) CustomersWithOrders(ctx context.Context, country string, limit int) ([]workload.CustomerWithOrders, error) {
	rows, err := momo.Select[models.Customer](i.db).
		Where(customer.Country.Eq(country)).
		OrderBy(customer.ID.Asc()).
		Limit(int64(limit)).
		Preload(customer.Orders.Scope(momo.LoadOrderBy[models.Order](order.ID.Asc()))).
		All(ctx)
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
	batch := make([]models.Order, len(rows))
	for n, o := range rows {
		batch[n] = models.Order{CustomerID: o.CustomerID, AmountCents: o.AmountCents, Status: o.Status, CreatedAt: o.CreatedAt}
	}
	res, err := momo.Insert[models.Order](i.db).Models(batch).Exec(ctx)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected, nil
}

func (i *impl) VectorTopK(context.Context, []float32, int) ([]int64, error) {
	return nil, workload.ErrNotSupported
}
