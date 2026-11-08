// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
)

func implicitDB(drv *fakedriver.Driver) *momo.DB {
	return momo.New(drv, momo.WithImplicitTx(), momo.WithTxSetup(func(context.Context, *momo.Tx) error { return nil }))
}

// A pure read that was read to the end commits even when the context was
// cancelled meanwhile: nothing could be lost.
func TestImplicitTxDrainedReadCommitsDespiteCancel(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{int64(1)}, {int64(2)}})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	got, err := momo.Scalars[int64](ctx, implicitDB(drv), "SELECT 1")
	if err != nil || len(got) != 2 {
		t.Fatalf("Scalars = %v, %v; want 2 rows and no error", got, err)
	}
	if drv.Commits() != 1 || drv.Rollbacks() != 0 {
		t.Fatalf("commits = %d rollbacks = %d, want 1 and 0", drv.Commits(), drv.Rollbacks())
	}
}

// A write with RETURNING that is closed after the context ended is rolled
// back and reports the cancellation.
func TestImplicitTxWriteRowsRollBackOnCancel(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{int64(1)}})
	ctx, cancel := context.WithCancel(t.Context())
	rows, err := momo.Insert[suite.Document](implicitDB(drv)).Model(&suite.Document{TenantID: 1, Title: "x"}).ReturningAll().Rows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := rows.Close(); err == nil {
		t.Fatal("Close after cancel returned nil")
	}
	if drv.Commits() != 0 || drv.Rollbacks() != 1 {
		t.Fatalf("commits = %d rollbacks = %d, want 0 and 1", drv.Commits(), drv.Rollbacks())
	}
}

// A panic unwinding through a deferred ModelRows.Close rolls back.
func TestImplicitTxModelRowsClosePanicRollsBack(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{int64(1)}})
	var got any
	func() {
		defer func() { got = recover() }()
		rows, err := momo.Insert[suite.Document](implicitDB(drv)).Model(&suite.Document{TenantID: 1, Title: "x"}).ReturningAll().Rows(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close() //nolint:errcheck // the deferred close is the code under test
		panic("mid-iteration")
	}()
	if got != "mid-iteration" {
		t.Fatalf("recovered %v, want the original panic", got)
	}
	if drv.Commits() != 0 || drv.Rollbacks() != 1 {
		t.Fatalf("commits = %d rollbacks = %d, want 0 and 1", drv.Commits(), drv.Rollbacks())
	}
}
