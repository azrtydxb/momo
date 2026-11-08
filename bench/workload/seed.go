// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SeedValue is the fixed random seed of the benchmark data.
const SeedValue = 42

var (
	countries = []string{"DE", "FR", "NL", "US", "GB"}
	statuses  = []string{"active", "inactive"}
	orderSt   = []string{"new", "paid", "shipped", "cancelled"}
)

const schemaSQL = `
DROP TABLE IF EXISTS bench_orders;
DROP TABLE IF EXISTS bench_customers;
CREATE TABLE bench_customers (
	id bigserial PRIMARY KEY,
	email text NOT NULL,
	country text NOT NULL,
	status text NOT NULL,
	created_at timestamptz NOT NULL
);
CREATE TABLE bench_orders (
	id bigserial PRIMARY KEY,
	customer_id bigint NOT NULL REFERENCES bench_customers (id),
	amount_cents bigint NOT NULL,
	status text NOT NULL,
	created_at timestamptz NOT NULL
);
CREATE INDEX bench_orders_customer_idx ON bench_orders (customer_id);
CREATE INDEX bench_customers_country_idx ON bench_customers (country, id);
`

// Seed drops and recreates bench_customers and bench_orders and fills them
// deterministically: the same arguments always produce the same rows.
func Seed(ctx context.Context, pool *pgxpool.Pool, customers, ordersPerCustomer int) error {
	if customers < 1 || ordersPerCustomer < 0 {
		return fmt.Errorf("workload: seed needs customers >= 1 and ordersPerCustomer >= 0, got %d and %d", customers, ordersPerCustomer)
	}
	if _, err := pool.Exec(ctx, schemaSQL, pgx.QueryExecModeSimpleProtocol); err != nil {
		return fmt.Errorf("workload: create schema: %w", err)
	}
	r := rand.New(rand.NewSource(SeedValue)) //nolint:gosec // deterministic benchmark data, not security
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	custRows := make([][]any, 0, customers)
	for i := 1; i <= customers; i++ {
		custRows = append(custRows, []any{
			int64(i), fmt.Sprintf("user%d@example.test", i),
			countries[r.Intn(len(countries))], statuses[r.Intn(len(statuses))],
			base.Add(time.Duration(i) * time.Minute),
		})
	}
	if _, err := pool.CopyFrom(ctx, pgx.Identifier{"bench_customers"},
		[]string{"id", "email", "country", "status", "created_at"}, pgx.CopyFromRows(custRows)); err != nil {
		return fmt.Errorf("workload: copy customers: %w", err)
	}

	orderRows := make([][]any, 0, customers*ordersPerCustomer)
	var oid int64
	for c := 1; c <= customers; c++ {
		for j := 0; j < ordersPerCustomer; j++ {
			oid++
			orderRows = append(orderRows, []any{
				oid, int64(c), int64(100 + r.Intn(100000)), orderSt[r.Intn(len(orderSt))],
				base.Add(time.Duration(r.Intn(1000000)) * time.Second),
			})
		}
	}
	if _, err := pool.CopyFrom(ctx, pgx.Identifier{"bench_orders"},
		[]string{"id", "customer_id", "amount_cents", "status", "created_at"}, pgx.CopyFromRows(orderRows)); err != nil {
		return fmt.Errorf("workload: copy orders: %w", err)
	}
	for _, t := range []string{"bench_customers", "bench_orders"} {
		q := fmt.Sprintf("SELECT setval(pg_get_serial_sequence('%s', 'id'), GREATEST((SELECT COALESCE(MAX(id), 0) FROM %s), 1))", t, t)
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("workload: reset sequence of %s: %w", t, err)
		}
	}
	return nil
}
