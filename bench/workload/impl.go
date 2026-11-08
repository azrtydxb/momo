// SPDX-License-Identifier: Apache-2.0

// Package workload defines the benchmark operations once, in terms of Impl,
// so every compared library runs the same SQL meaning and returns rows in one
// canonical encoding.
package workload

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Impl is implemented by every adapter. Each method has one SQL meaning,
// defined in sql.go.
type Impl interface {
	Name() string
	// CustomerByID returns (Customer{}, ErrNoRow) when no row matches.
	CustomerByID(ctx context.Context, id int64) (Customer, error)
	// RecentOrders orders by created_at DESC, id DESC.
	RecentOrders(ctx context.Context, country string, limit int) ([]Order, error)
	// InsertOrder returns the generated id.
	InsertOrder(ctx context.Context, o Order) (int64, error)
	// SetCustomerStatus returns the rows affected.
	SetCustomerStatus(ctx context.Context, id int64, status string) (int64, error)
	// CustomersWithOrders returns customers ordered by id ascending, limited,
	// each with its orders ordered by id ascending.
	CustomersWithOrders(ctx context.Context, country string, limit int) ([]CustomerWithOrders, error)
	// BulkInsertOrders returns the rows affected.
	BulkInsertOrders(ctx context.Context, rows []Order) (int64, error)
	// VectorTopK may return ErrNotSupported.
	VectorTopK(ctx context.Context, query []float32, k int) ([]int64, error)
}

// ErrNoRow is returned by CustomerByID when no row matches; adapters map their
// own not-found error to it.
var ErrNoRow = errors.New("workload: no row")

// ErrNotSupported is returned by operations an implementation does not offer.
var ErrNotSupported = errors.New("workload: operation not supported by this implementation")

type canonicalRower interface {
	canonicalRow() []string
}

// Canonical returns a stable textual form of rows: one line per row, fields
// separated by a pipe, times in UTC with microseconds. Row types must come
// from this package; any other type panics, because an unencodable row would
// make an equivalence check meaningless.
func Canonical[T any](rows []T) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		c, ok := any(r).(canonicalRower)
		if !ok {
			panic(fmt.Sprintf("workload: %T has no canonical encoding", r))
		}
		out = append(out, strings.Join(c.canonicalRow(), "|"))
	}
	return out
}

// VectorLiteral renders a float32 slice as a pgvector text literal.
func VectorLiteral(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%g", f)
	}
	b.WriteByte(']')
	return b.String()
}
