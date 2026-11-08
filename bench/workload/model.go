// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"strconv"
	"time"
)

// Customer is one row of bench_customers.
type Customer struct {
	ID    int64
	Email string
	// Country is an ISO 3166 alpha-2 code.
	Country string
	Status  string
	// CreatedAt is stored as timestamptz; the canonical form truncates it to
	// microseconds in UTC.
	CreatedAt time.Time
}

// Order is one row of bench_orders.
type Order struct {
	ID          int64
	CustomerID  int64
	AmountCents int64
	Status      string
	CreatedAt   time.Time
}

// CustomerWithOrders is a customer and its orders, ordered by ID ascending.
type CustomerWithOrders struct {
	Customer Customer
	Orders   []Order
}

// The canonical encoding of a row joins its fields with a pipe. It assumes the
// workload data has no NULLs, no floating point values and no pipe character in
// any text field; the seed guarantees that. A workload that adds such a column
// must extend the encoding first, or two different rows could encode alike.
const canonicalTimeLayout = "2006-01-02T15:04:05.000000Z07:00"

func canonicalTime(t time.Time) string {
	return t.UTC().Truncate(time.Microsecond).Format(canonicalTimeLayout)
}

func (c Customer) canonicalRow() []string {
	return []string{
		strconv.FormatInt(c.ID, 10), c.Email, c.Country, c.Status, canonicalTime(c.CreatedAt),
	}
}

func (o Order) canonicalRow() []string {
	return []string{
		strconv.FormatInt(o.ID, 10), strconv.FormatInt(o.CustomerID, 10),
		strconv.FormatInt(o.AmountCents, 10), o.Status, canonicalTime(o.CreatedAt),
	}
}
