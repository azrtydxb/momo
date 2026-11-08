// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/tag"
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

// The single-row write helpers read their RETURNING rows to the end, so on an
// implicit-transaction handle their insert is committed, not rolled back.
func TestImplicitTxFindOrCreateAndUpsertByPKCommit(t *testing.T) {
	for name, run := range map[string]func(db *momo.DB) error{
		"FindOrCreate": func(db *momo.DB) error {
			created, err := momo.FindOrCreate[models.Tag, *models.Tag](t.Context(), db, []momo.ColumnRef{tag.ID}, &models.Tag{ID: 1, Name: "x"})
			if err == nil && !created {
				t.Error("created = false, want true")
			}
			return err
		},
		"UpsertByPK": func(db *momo.DB) error {
			return momo.UpsertByPK[models.Tag, *models.Tag](t.Context(), db, &models.Tag{ID: 1, Name: "x"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			drv := fakedriver.New(dialect.Postgres)
			drv.SetRows([][]any{{int64(1), "x"}})
			if err := run(implicitDB(drv)); err != nil {
				t.Fatal(err)
			}
			if drv.Commits() != 1 || drv.Rollbacks() != 0 {
				t.Fatalf("commits = %d rollbacks = %d, want 1 and 0: the insert was lost", drv.Commits(), drv.Rollbacks())
			}
		})
	}
}

// UpdateQuery.Rows on an implicit-transaction handle: a caller that reads the
// first row and closes loses the update, while reading to the end commits.
func TestImplicitTxUpdateRowsMustBeReadToTheEnd(t *testing.T) {
	read := func(toEnd bool) (*fakedriver.Driver, error) {
		drv := fakedriver.New(dialect.Postgres)
		drv.SetRows([][]any{{int64(1), "x"}, {int64(2), "y"}})
		rows, err := momo.Update[models.Tag](implicitDB(drv)).Set(tag.Name.Set("z")).Where(tag.ID.Gt(0)).ReturningAll().Rows(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		rows.Next()
		if toEnd {
			for rows.Next() {
			}
		}
		return drv, rows.Close()
	}
	drv, err := read(false)
	if !errors.Is(err, momo.ErrValidation) || drv.Commits() != 0 || drv.Rollbacks() != 1 {
		t.Fatalf("first row only: err = %v commits = %d rollbacks = %d, want ErrValidation, 0 and 1", err, drv.Commits(), drv.Rollbacks())
	}
	drv, err = read(true)
	if err != nil || drv.Commits() != 1 || drv.Rollbacks() != 0 {
		t.Fatalf("read to the end: err = %v commits = %d rollbacks = %d, want nil, 1 and 0", err, drv.Commits(), drv.Rollbacks())
	}
}

// A deferred ModelRows.Close that runs during a panic still closes the cursor
// when the handle is not an implicit-transaction one, and the hook still gets
// its AfterQuery.
func TestModelRowsCloseUnderPanicReleasesCursorOnPlainHandle(t *testing.T) {
	rec := &obstest.Recorder{}
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{int64(1)}})
	db := momo.New(drv, momo.WithHook(rec))
	var got any
	func() {
		defer func() { got = recover() }()
		rows, err := momo.Insert[suite.Document](db).Model(&suite.Document{TenantID: 1, Title: "x"}).ReturningAll().Rows(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close() //nolint:errcheck // the deferred close is the code under test
		panic("mid-iteration")
	}()
	if got != "mid-iteration" {
		t.Fatalf("recovered %v, want the original panic", got)
	}
	if drv.Closed() != 1 {
		t.Fatalf("closed cursors = %d, want 1: the cursor and its connection leaked", drv.Closed())
	}
	if n := len(rec.Events()); n == 0 {
		t.Fatal("AfterQuery never fired for the abandoned statement")
	}
}

// Raw SQL read through Scalars reaches hooks as op "raw".
func TestRawReadsReportOpRaw(t *testing.T) {
	rec := &obstest.Recorder{}
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{int64(1)}})
	if _, err := momo.Scalars[int64](t.Context(), momo.New(drv, momo.WithHook(rec)), "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	evs := rec.Events()
	if len(evs) == 0 || evs[len(evs)-1].Op != "raw" {
		t.Fatalf("events = %+v, want op raw", evs)
	}
}
