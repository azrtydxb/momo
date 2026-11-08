// SPDX-License-Identifier: Apache-2.0

// Package pgxhand is the handwritten pgx reference implementation of
// workload.Impl.
package pgxhand

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo/bench/workload"
)

type impl struct {
	pool *pgxpool.Pool
}

// New returns the handwritten pgx implementation on pool.
func New(pool *pgxpool.Pool) workload.Impl { return &impl{pool: pool} }

func (i *impl) Name() string { return "pgx" }

func (i *impl) CustomerByID(ctx context.Context, id int64) (workload.Customer, error) {
	var c workload.Customer
	err := i.pool.QueryRow(ctx, workload.SQLCustomerByID, id).
		Scan(&c.ID, &c.Email, &c.Country, &c.Status, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return workload.Customer{}, workload.ErrNoRow
	}
	return c, err
}

func scanOrders(rows pgx.Rows) ([]workload.Order, error) {
	defer rows.Close()
	var out []workload.Order
	for rows.Next() {
		var o workload.Order
		if err := rows.Scan(&o.ID, &o.CustomerID, &o.AmountCents, &o.Status, &o.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (i *impl) RecentOrders(ctx context.Context, country string, limit int) ([]workload.Order, error) {
	rows, err := i.pool.Query(ctx, workload.SQLRecentOrders, country, limit)
	if err != nil {
		return nil, err
	}
	return scanOrders(rows)
}

func (i *impl) InsertOrder(ctx context.Context, o workload.Order) (int64, error) {
	var id int64
	err := i.pool.QueryRow(ctx, workload.SQLInsertOrder, o.CustomerID, o.AmountCents, o.Status, o.CreatedAt).Scan(&id)
	return id, err
}

func (i *impl) SetCustomerStatus(ctx context.Context, id int64, status string) (int64, error) {
	tag, err := i.pool.Exec(ctx, workload.SQLSetCustomerStatus, id, status)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (i *impl) CustomersWithOrders(ctx context.Context, country string, limit int) ([]workload.CustomerWithOrders, error) {
	rows, err := i.pool.Query(ctx, workload.SQLCustomersWithOrders, country, limit)
	if err != nil {
		return nil, err
	}
	var out []workload.CustomerWithOrders
	ids := make([]int64, 0, limit)
	for rows.Next() {
		var c workload.Customer
		if err := rows.Scan(&c.ID, &c.Email, &c.Country, &c.Status, &c.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, workload.CustomerWithOrders{Customer: c})
		ids = append(ids, c.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	orows, err := i.pool.Query(ctx, workload.SQLOrdersForCustomers, ids)
	if err != nil {
		return nil, err
	}
	orders, err := scanOrders(orows)
	if err != nil {
		return nil, err
	}
	index := make(map[int64]int, len(out))
	for n := range out {
		index[out[n].Customer.ID] = n
	}
	for _, o := range orders {
		n := index[o.CustomerID]
		out[n].Orders = append(out[n].Orders, o)
	}
	return out, nil
}

func (i *impl) BulkInsertOrders(ctx context.Context, rows []workload.Order) (int64, error) {
	var total int64
	for start := 0; start < len(rows); start += workload.BulkChunkRows {
		end := min(start+workload.BulkChunkRows, len(rows))
		chunk := rows[start:end]
		args := make([]any, 0, len(chunk)*4)
		for _, o := range chunk {
			args = append(args, o.CustomerID, o.AmountCents, o.Status, o.CreatedAt)
		}
		tag, err := i.pool.Exec(ctx, workload.BulkInsertSQL(len(chunk)), args...)
		if err != nil {
			return total, err
		}
		total += tag.RowsAffected()
	}
	return total, nil
}

func (i *impl) VectorTopK(ctx context.Context, query []float32, k int) ([]int64, error) {
	rows, err := i.pool.Query(ctx, workload.SQLVectorTopK, workload.VectorLiteral(query), k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
