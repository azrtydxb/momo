// SPDX-License-Identifier: Apache-2.0

// Package sqlhand is the handwritten database/sql reference implementation of
// workload.Impl, on the pgx stdlib driver.
package sqlhand

import (
	"context"
	"database/sql"
	"errors"

	"github.com/azrtydxb/momo/bench/workload"
)

type impl struct {
	db *sql.DB
}

// New returns the handwritten database/sql implementation on db. The caller
// opens db with the pgx stdlib driver and adapters.ApplySQLDBSettings.
func New(db *sql.DB) workload.Impl { return &impl{db: db} }

func (i *impl) Name() string { return "database/sql" }

func (i *impl) CustomerByID(ctx context.Context, id int64) (workload.Customer, error) {
	var c workload.Customer
	err := i.db.QueryRowContext(ctx, workload.SQLCustomerByID, id).
		Scan(&c.ID, &c.Email, &c.Country, &c.Status, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return workload.Customer{}, workload.ErrNoRow
	}
	return c, err
}

func scanOrders(rows *sql.Rows) ([]workload.Order, error) {
	defer func() { _ = rows.Close() }()
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
	rows, err := i.db.QueryContext(ctx, workload.SQLRecentOrders, country, limit)
	if err != nil {
		return nil, err
	}
	return scanOrders(rows)
}

func (i *impl) InsertOrder(ctx context.Context, o workload.Order) (int64, error) {
	var id int64
	err := i.db.QueryRowContext(ctx, workload.SQLInsertOrder, o.CustomerID, o.AmountCents, o.Status, o.CreatedAt).Scan(&id)
	return id, err
}

func (i *impl) SetCustomerStatus(ctx context.Context, id int64, status string) (int64, error) {
	res, err := i.db.ExecContext(ctx, workload.SQLSetCustomerStatus, id, status)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (i *impl) CustomersWithOrders(ctx context.Context, country string, limit int) ([]workload.CustomerWithOrders, error) {
	rows, err := i.db.QueryContext(ctx, workload.SQLCustomersWithOrders, country, limit)
	if err != nil {
		return nil, err
	}
	var out []workload.CustomerWithOrders
	ids := make([]int64, 0, limit)
	for rows.Next() {
		var c workload.Customer
		if err := rows.Scan(&c.ID, &c.Email, &c.Country, &c.Status, &c.CreatedAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		out = append(out, workload.CustomerWithOrders{Customer: c})
		ids = append(ids, c.ID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	orows, err := i.db.QueryContext(ctx, workload.SQLOrdersForCustomers, ids)
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
		res, err := i.db.ExecContext(ctx, workload.BulkInsertSQL(len(chunk)), args...)
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

func (i *impl) VectorTopK(ctx context.Context, query []float32, k int) ([]int64, error) {
	rows, err := i.db.QueryContext(ctx, workload.SQLVectorTopK, workload.VectorLiteral(query), k)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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
