// SPDX-License-Identifier: Apache-2.0

// Package models holds the Momo models of the benchmark schema. The
// generated metadata and column packages come from make bench-generate.
package models

import (
	"time"

	"github.com/azrtydxb/momo"
)

// Customer is one row of bench_customers.
type Customer struct {
	momo.Model `momo:"table:bench_customers"`
	ID         int64     `momo:"pk,identity"`
	Email      string    `momo:"notnull"`
	Country    string    `momo:"notnull,index"`
	Status     string    `momo:"notnull"`
	CreatedAt  time.Time `momo:"column:created_at,notnull"`

	Orders []Order `momo:"has_many,fk:CustomerID"`
}

// Order is one row of bench_orders.
type Order struct {
	momo.Model  `momo:"table:bench_orders"`
	ID          int64     `momo:"pk,identity"`
	CustomerID  int64     `momo:"column:customer_id,notnull,index"`
	AmountCents int64     `momo:"column:amount_cents,notnull"`
	Status      string    `momo:"notnull"`
	CreatedAt   time.Time `momo:"column:created_at,notnull"`
}
