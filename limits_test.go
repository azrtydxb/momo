// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// MaxRows turns an oversized result into ErrLimitExceeded and never truncates;
// a result at the cap passes whole. Fails if rows past the cap are dropped.
func TestMaxRowsGuardUnit(t *testing.T) {
	ctx := context.Background()
	drv := fakedriver.New(dialect.SQLite)
	drv.SetRows(threeRows())
	db := momo.New(drv)
	rows, err := momo.Select[unitDocument](db).MaxRows(2).All(ctx)
	if !errors.Is(err, momo.ErrLimitExceeded) || rows != nil {
		t.Fatalf("MaxRows(2) = %v, %v; want nil, ErrLimitExceeded", rows, err)
	}
	if drv.Closed() == 0 {
		t.Fatal("cursor not closed after the cap was hit")
	}
	rows, err = momo.Select[unitDocument](db).MaxRows(3).All(ctx)
	if err != nil || len(rows) != 3 {
		t.Fatalf("MaxRows(3) rows=%d err=%v", len(rows), err)
	}
	if _, err := momo.Select[unitDocument](db).MaxRows(0).All(ctx); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("MaxRows(0) err = %v, want ErrValidation", err)
	}
}

// The IN cap fails the build with ErrLimitExceeded, for In and NotIn, and lets
// a list at the cap through. Fails if an over-cap list reaches the SQL.
func TestInListCapUnit(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres), momo.WithMaxInList(2))
	for name, p := range map[string]momo.Predicate{
		"In":    DocTitle.In("a", "b", "c"),
		"NotIn": DocTitle.NotIn("a", "b", "c"),
	} {
		if _, err := momo.Select[unitDocument](db).Where(p).SQL(); !errors.Is(err, momo.ErrLimitExceeded) {
			t.Fatalf("%s over cap err = %v, want ErrLimitExceeded", name, err)
		}
	}
	if _, err := momo.Select[unitDocument](db).Where(DocTitle.In("a", "b")).SQL(); err != nil {
		t.Fatalf("IN at cap: %v", err)
	}
	other := momo.New(fakedriver.New(dialect.Postgres))
	if _, err := momo.Select[unitDocument](other).Where(DocTitle.In("a", "b", "c")).SQL(); err != nil {
		t.Fatalf("a DB without the option must not be capped: %v", err)
	}
}

func TestWithMaxInListPanicsOnNonPositive(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("WithMaxInList(0) did not panic")
		}
	}()
	momo.New(fakedriver.New(dialect.Postgres), momo.WithMaxInList(0))
}
