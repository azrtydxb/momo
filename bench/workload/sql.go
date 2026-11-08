// SPDX-License-Identifier: Apache-2.0

package workload

// The exact statements every handwritten adapter and every semantic reference
// uses. Each ordered read has an explicit, deterministic ORDER BY.
const (
	sqlCustomerByID = `SELECT id, email, country, status, created_at FROM bench_customers WHERE id = $1`

	sqlRecentOrders = `SELECT o.id, o.customer_id, o.amount_cents, o.status, o.created_at ` +
		`FROM bench_orders o JOIN bench_customers c ON c.id = o.customer_id ` +
		`WHERE c.country = $1 ORDER BY o.created_at DESC, o.id DESC LIMIT $2`

	sqlInsertOrder = `INSERT INTO bench_orders (customer_id, amount_cents, status, created_at) ` +
		`VALUES ($1, $2, $3, $4) RETURNING id`

	sqlSetCustomerStatus = `UPDATE bench_customers SET status = $2 WHERE id = $1`

	sqlCustomersWithOrders = `SELECT id, email, country, status, created_at FROM bench_customers ` +
		`WHERE country = $1 ORDER BY id ASC LIMIT $2`

	sqlOrdersForCustomers = `SELECT id, customer_id, amount_cents, status, created_at FROM bench_orders ` +
		`WHERE customer_id = ANY($1) ORDER BY id ASC`

	sqlVectorTopK = `SELECT id FROM vec_items ORDER BY emb <=> $1::vector LIMIT $2`

	// BulkChunkRows is the number of rows per bulk INSERT statement: 4 bind
	// parameters per row keep it far below the 65535 PostgreSQL limit.
	BulkChunkRows = 2000
)

// Exported views of the statements for the adapters.
const (
	SQLCustomerByID        = sqlCustomerByID
	SQLRecentOrders        = sqlRecentOrders
	SQLInsertOrder         = sqlInsertOrder
	SQLSetCustomerStatus   = sqlSetCustomerStatus
	SQLCustomersWithOrders = sqlCustomersWithOrders
	SQLOrdersForCustomers  = sqlOrdersForCustomers
	SQLVectorTopK          = sqlVectorTopK
)

// BulkInsertSQL returns one multi-row INSERT for n rows (4 parameters each).
func BulkInsertSQL(n int) string {
	b := make([]byte, 0, 80+n*24)
	b = append(b, "INSERT INTO bench_orders (customer_id, amount_cents, status, created_at) VALUES "...)
	p := 1
	for i := 0; i < n; i++ {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '(')
		for j := 0; j < 4; j++ {
			if j > 0 {
				b = append(b, ',')
			}
			b = append(b, '$')
			b = appendInt(b, p)
			p++
		}
		b = append(b, ')')
	}
	return string(b)
}

func appendInt(b []byte, n int) []byte {
	if n >= 10 {
		b = appendInt(b, n/10)
	}
	return append(b, byte('0'+n%10))
}
