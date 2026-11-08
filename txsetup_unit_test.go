// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
)

func implicitDB(drv *fakedriver.Driver) *momo.DB {
	return momo.New(drv, momo.WithImplicitTx(), momo.WithTxSetup(func(context.Context, *momo.Tx) error { return nil }))
}

// A typed read that was read to the end commits even when the context was
// cancelled meanwhile: nothing could be lost.
func TestImplicitTxDrainedReadCommitsDespiteCancel(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	got, err := momo.Select[suite.Document](implicitDB(drv)).All(ctx)
	if err != nil || len(got) != 0 {
		t.Fatalf("All = %v, %v; want no rows and no error", got, err)
	}
	if drv.Commits() != 1 || drv.Rollbacks() != 0 {
		t.Fatalf("commits = %d rollbacks = %d, want 1 and 0", drv.Commits(), drv.Rollbacks())
	}
}

// Raw SQL may change data (INSERT ... RETURNING through Scalars), so a drained
// raw statement under a cancelled context is rolled back and reports it.
func TestImplicitTxRawStatementRollsBackOnCancel(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{int64(1)}})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := momo.Scalars[int64](ctx, implicitDB(drv), "INSERT INTO t (a) VALUES (1) RETURNING a")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Scalars = %v, want context.Canceled", err)
	}
	if drv.Commits() != 0 || drv.Rollbacks() != 1 {
		t.Fatalf("commits = %d rollbacks = %d, want 0 and 1", drv.Commits(), drv.Rollbacks())
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

// The rollback does not depend on how Close is deferred: a closure around Close
// cannot recover the panic, so the write is rolled back because its rows were
// not read to the end.
func TestImplicitTxModelRowsClosureClosePanicRollsBack(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{int64(1)}, {int64(2)}})
	var got any
	func() {
		defer func() { got = recover() }()
		rows, err := momo.Insert[suite.Document](implicitDB(drv)).Models([]suite.Document{{TenantID: 1, Title: "x"}, {TenantID: 1, Title: "y"}}).ReturningAll().Rows(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rows.Close() }()
		rows.Next()
		panic("mid-iteration")
	}()
	if got != "mid-iteration" {
		t.Fatalf("recovered %v, want the original panic", got)
	}
	if drv.Commits() != 0 || drv.Rollbacks() != 1 {
		t.Fatalf("commits = %d rollbacks = %d, want 0 and 1", drv.Commits(), drv.Rollbacks())
	}
}

// Closing a write before reading its rows is reported and rolled back.
func TestImplicitTxWriteClosedUnreadIsRolledBack(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{int64(1)}})
	rows, err := momo.Insert[suite.Document](implicitDB(drv)).Model(&suite.Document{TenantID: 1, Title: "x"}).ReturningAll().Rows(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("Close = %v, want ErrValidation", err)
	}
	if drv.Commits() != 0 || drv.Rollbacks() != 1 {
		t.Fatalf("commits = %d rollbacks = %d, want 0 and 1", drv.Commits(), drv.Rollbacks())
	}
}
