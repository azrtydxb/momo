// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/suite"
)

func chunkedDefaultInsert(db *momo.DB, n int) momo.InsertQuery[suite.Document] {
	ms := make([]suite.Document, n)
	for i := range ms {
		ms[i] = suite.Document{TenantID: int64(i), Title: "b"}
	}
	return momo.Insert[suite.Document](db).
		Omit(suite.DocBody, suite.DocSecret, suite.DocDeletedAt, suite.DocVersion).Models(ms)
}

// A default is evaluated once per row even when the insert is chunked, and the
// chunks bind exactly the value that was computed.
func TestFieldDefaultEvaluatedOncePerRowInChunkedInsert(t *testing.T) {
	const rows = 25 // 5 bound columns, 50 parameters: 10 rows per chunk
	calls := int64(0)
	drv := newSmallLimit(50)
	db := momo.New(drv, momo.WithFieldHooks[suite.Document]([]momo.FieldHook[suite.Document]{
		{Column: 4, Default: func(*suite.Document) any { calls++; return calls }},
	}))
	if _, err := chunkedDefaultInsert(db, rows).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	if calls != rows {
		t.Fatalf("default ran %d times for %d rows", calls, rows)
	}
	next := int64(1)
	for _, c := range drv.Calls() {
		for r := 0; r < len(c.Args)/5; r++ {
			if c.Args[r*5+2] != next {
				t.Fatalf("row %d bound version %v, want %d", next, c.Args[r*5+2], next)
			}
			next++
		}
	}
	if next != rows+1 {
		t.Fatalf("bound %d rows, want %d", next-1, rows)
	}
}

// A default that would turn invalid on a repeated call never causes a partial
// bulk insert: the planner computes each value once and chunks reuse it.
func TestFieldNondeterministicDefaultNeverPartiallyExecutes(t *testing.T) {
	const rows = 25
	calls := 0
	drv := newSmallLimit(50)
	db := momo.New(drv, momo.WithFieldHooks[suite.Document]([]momo.FieldHook[suite.Document]{{
		Column: 4,
		Default: func(*suite.Document) any {
			calls++
			if calls > rows {
				return int64(-1) // the second evaluation of a row
			}
			return int64(1)
		},
		Validate: func(v any) error {
			if n, _ := v.(int64); n < 0 {
				return errBadTitle
			}
			return nil
		},
	}}))
	if _, err := chunkedDefaultInsert(db, rows).Exec(t.Context()); err != nil {
		t.Fatalf("err = %v, want success: values validated in the pre-pass must be the written ones", err)
	}
	if calls != rows || len(drv.Calls()) != 3 {
		t.Fatalf("calls = %d, statements = %d", calls, len(drv.Calls()))
	}
}
