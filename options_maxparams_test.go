// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func TestWithMaxParamsCopiesDialect(t *testing.T) {
	shared := dialect.For(dialect.Postgres)
	before := shared.MaxParams
	db := momo.New(fakedriver.New(dialect.Postgres), momo.WithMaxParams(7))
	if got := db.Dialect().MaxParams; got != 7 {
		t.Fatalf("db MaxParams = %d, want 7", got)
	}
	if shared.MaxParams != before {
		t.Fatalf("shared dialect mutated: %d, want %d", shared.MaxParams, before)
	}
	other := momo.New(fakedriver.New(dialect.Postgres))
	if got := other.Dialect().MaxParams; got != before {
		t.Fatalf("a DB without the option sees MaxParams %d, want %d", got, before)
	}
}

func TestWithMaxParamsAppliesToTransactionsAndInserts(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithMaxParams(50)) // 5 columns per row -> 10 rows per statement
	err := db.Tx(t.Context(), momo.TxOptions{}, func(tx *momo.Tx) error {
		if got := tx.Dialect().MaxParams; got != 50 {
			t.Errorf("tx MaxParams = %d, want 50", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bulkQuery(db, 25).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range drv.Calls() {
		if len(c.Args) > 50 {
			t.Fatalf("insert statement binds %d > 50", len(c.Args))
		}
		if len(c.Args) > 0 {
			n++
		}
	}
	if n != 3 {
		t.Fatalf("25 rows at 10 per statement: want 3 insert statements, got %d", n)
	}
}
