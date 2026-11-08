// SPDX-License-Identifier: Apache-2.0

package gormad

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/azrtydxb/momo/bench/workload"
)

type gormCustomer struct {
	ID        int64 `gorm:"primaryKey"`
	Email     string
	Country   string
	Status    string
	CreatedAt time.Time   `gorm:"column:created_at"`
	Orders    []gormOrder `gorm:"foreignKey:CustomerID"`
}

func (gormCustomer) TableName() string { return "bench_customers" }

type gormOrder struct {
	ID          int64 `gorm:"primaryKey"`
	CustomerID  int64 `gorm:"column:customer_id"`
	AmountCents int64 `gorm:"column:amount_cents"`
	Status      string
	CreatedAt   time.Time `gorm:"column:created_at"`
}

func (gormOrder) TableName() string { return "bench_orders" }

type impl struct {
	db *gorm.DB
}

// New returns the GORM implementation on db.
func New(db *gorm.DB) workload.Impl { return &impl{db: db} }

func (i *impl) Name() string { return "gorm" }

func toCustomer(c gormCustomer) workload.Customer {
	return workload.Customer{ID: c.ID, Email: c.Email, Country: c.Country, Status: c.Status, CreatedAt: c.CreatedAt}
}

func toOrder(o gormOrder) workload.Order {
	return workload.Order{ID: o.ID, CustomerID: o.CustomerID, AmountCents: o.AmountCents, Status: o.Status, CreatedAt: o.CreatedAt}
}

func (i *impl) CustomerByID(ctx context.Context, id int64) (workload.Customer, error) {
	var c gormCustomer
	err := i.db.WithContext(ctx).Take(&c, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return workload.Customer{}, workload.ErrNoRow
	}
	if err != nil {
		return workload.Customer{}, err
	}
	return toCustomer(c), nil
}

func (i *impl) RecentOrders(ctx context.Context, country string, limit int) ([]workload.Order, error) {
	var rows []gormOrder
	err := i.db.WithContext(ctx).
		Table("bench_orders AS o").
		Select("o.id, o.customer_id, o.amount_cents, o.status, o.created_at").
		Joins("JOIN bench_customers c ON c.id = o.customer_id").
		Where("c.country = ?", country).
		Order("o.created_at DESC, o.id DESC").
		Limit(limit).
		Find(&rows).Error
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
	row := gormOrder{CustomerID: o.CustomerID, AmountCents: o.AmountCents, Status: o.Status, CreatedAt: o.CreatedAt}
	if err := i.db.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, err
	}
	return row.ID, nil
}

func (i *impl) SetCustomerStatus(ctx context.Context, id int64, status string) (int64, error) {
	res := i.db.WithContext(ctx).Model(&gormCustomer{}).Where("id = ?", id).Update("status", status)
	return res.RowsAffected, res.Error
}

func (i *impl) CustomersWithOrders(ctx context.Context, country string, limit int) ([]workload.CustomerWithOrders, error) {
	var rows []gormCustomer
	err := i.db.WithContext(ctx).
		Where("country = ?", country).
		Order("id ASC").
		Limit(limit).
		Preload("Orders", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
		Find(&rows).Error
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
	batch := make([]gormOrder, len(rows))
	for n, o := range rows {
		batch[n] = gormOrder{CustomerID: o.CustomerID, AmountCents: o.AmountCents, Status: o.Status, CreatedAt: o.CreatedAt}
	}
	res := i.db.WithContext(ctx).CreateInBatches(&batch, workload.BulkChunkRows)
	return res.RowsAffected, res.Error
}

func (i *impl) VectorTopK(context.Context, []float32, int) ([]int64, error) {
	return nil, workload.ErrNotSupported
}
