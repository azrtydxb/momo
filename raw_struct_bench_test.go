// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

type benchWide struct {
	C1, C2, C3, C4, C5, C6, C7 int64
	Name                       string
	Email                      string
	Active                     bool
	Score                      float64
	Note                       *string
	CreatedAt                  time.Time
}

// BenchmarkQueryPlainStruct reads 1000 rows of 13 columns into a plain struct.
// The reflection plan is built once, so per-row work is one field lookup per
// column; compare ns/op with BenchmarkQueryTupleRow, which does no reflection.
func BenchmarkQueryPlainStruct(b *testing.B) {
	at := time.Now()
	row := []any{int64(1), int64(2), int64(3), int64(4), int64(5), int64(6), int64(7), "n", "e", true, 1.5, nil, at}
	names := []string{"c1", "c2", "c3", "c4", "c5", "c6", "c7", "name", "email", "active", "score", "note", "created_at"}
	rows := make([][]any, 1000)
	for i := range rows {
		rows[i] = row
	}
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows(rows)
	db := momo.New(namedDriver{drv, names})
	b.ReportAllocs()
	for b.Loop() {
		got, err := momo.Query[benchWide](b.Context(), db, "SELECT * FROM t")
		if err != nil || len(got) != len(rows) {
			b.Fatalf("Query = %d rows, %v", len(got), err)
		}
	}
}

// BenchmarkQueryTupleRow is the baseline: the same 1000 rows through the
// metadata path (Row2).
func BenchmarkQueryTupleRow(b *testing.B) {
	rows := make([][]any, 1000)
	for i := range rows {
		rows[i] = []any{int64(1), "n"}
	}
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows(rows)
	db := momo.New(drv)
	b.ReportAllocs()
	for b.Loop() {
		got, err := momo.Query[momo.Row2[int64, string]](b.Context(), db, "SELECT * FROM t")
		if err != nil || len(got) != len(rows) {
			b.Fatalf("Query = %d rows, %v", len(got), err)
		}
	}
}
