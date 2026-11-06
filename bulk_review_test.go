// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/suite"
)

var errChunk = errors.New("chunk failed")

// hooks scripts the INSERT statements a hookDriver sees.
type hooks struct {
	inserts  int
	panicAt  int // 1-based INSERT that panics
	failAt   int // 1-based INSERT that fails
	affected int64
}

func (h *hooks) stmt(sql string) (int64, error) {
	if !strings.HasPrefix(sql, "INSERT") {
		return 0, nil
	}
	h.inserts++
	if h.inserts == h.panicAt {
		panic("boom")
	}
	if h.inserts == h.failAt {
		return 0, errChunk
	}
	return h.affected, nil
}

type hookDriver struct {
	smallLimit
	h *hooks
}

func newHookDriver(max int, h *hooks) hookDriver { return hookDriver{newSmallLimit(max), h} }

func (d hookDriver) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	n, err := d.h.stmt(sql)
	if err != nil {
		return 0, err
	}
	if _, err := d.Driver.Exec(ctx, sql, args); err != nil {
		return 0, err
	}
	return n, nil
}

func (d hookDriver) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	if _, err := d.h.stmt(sql); err != nil {
		return nil, err
	}
	return d.Driver.Query(ctx, sql, args)
}

func (d hookDriver) Begin(ctx context.Context, o momo.TxOptions) (momo.DriverTx, error) {
	tx, err := d.Driver.Begin(ctx, o)
	if err != nil {
		return nil, err
	}
	return hookTx{tx, d.h}, nil
}

type hookTx struct {
	momo.DriverTx
	h *hooks
}

func (t hookTx) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	n, err := t.h.stmt(sql)
	if err != nil {
		return 0, err
	}
	if _, err := t.DriverTx.Exec(ctx, sql, args); err != nil {
		return 0, err
	}
	return n, nil
}

func (t hookTx) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	if _, err := t.h.stmt(sql); err != nil {
		return nil, err
	}
	return t.DriverTx.Query(ctx, sql, args)
}

func TestBulkExecPanicRollsBackAndRepanics(t *testing.T) {
	drv := newHookDriver(50, &hooks{panicAt: 2})
	db := momo.New(drv)
	var got any
	func() {
		defer func() { got = recover() }()
		_, _ = bulkQuery(db, 25).Exec(t.Context())
	}()
	if got != "boom" {
		t.Fatalf("recovered %v, want the original panic", got)
	}
	if drv.Rollbacks() != 1 || drv.Commits() != 0 {
		t.Fatalf("rollbacks = %d commits = %d, want 1 and 0: the transaction leaked", drv.Rollbacks(), drv.Commits())
	}
}

func TestBulkRowsFirstChunkErrorFromRows(t *testing.T) {
	drv := newHookDriver(50, &hooks{failAt: 1})
	db := momo.New(drv)
	rows, err := bulkQuery(db, 25).Returning(suite.DocID).Rows(t.Context())
	if !errors.Is(err, errChunk) || rows != nil {
		t.Fatalf("rows = %v, err = %v, want the first chunk error from Rows", rows, err)
	}
	if drv.Rollbacks() != 1 || drv.Commits() != 0 {
		t.Fatalf("rollbacks = %d commits = %d, want 1 and 0", drv.Rollbacks(), drv.Commits())
	}
}

func TestBulkRowsLaterChunkErrorFromCloseRollsBack(t *testing.T) {
	drv := newHookDriver(50, &hooks{failAt: 2})
	drv.SetRows([][]any{{int64(1)}, {int64(2)}})
	db := momo.New(drv)
	rows, err := bulkQuery(db, 25).Returning(suite.DocID).Rows(t.Context())
	if err != nil {
		t.Fatalf("Rows: %v, the first chunk succeeds", err)
	}
	if !rows.Next() { // early break: chunks 2 and 3 are still pending
		t.Fatal("no first row")
	}
	if err := rows.Close(); !errors.Is(err, errChunk) {
		t.Fatalf("Close = %v, want the later chunk error", err)
	}
	if drv.Rollbacks() != 1 || drv.Commits() != 0 {
		t.Fatalf("rollbacks = %d commits = %d, want the whole insert rolled back", drv.Rollbacks(), drv.Commits())
	}
}

// fakePinned pins statements to a fake driver without transactions.
type fakePinned struct{ hookDriver }

func (fakePinned) Release() error { return nil }

func TestBulkChunkedInsertOnPinnedConnIsRefused(t *testing.T) {
	drv := newHookDriver(50, &hooks{})
	db := momo.New(drv).OnConn(fakePinned{drv})
	_, err := bulkQuery(db, 25).Exec(t.Context())
	if !errors.Is(err, momo.ErrUnsupported) || !strings.Contains(err.Error(), "chunked insert needs a transaction") {
		t.Fatalf("Exec err = %v, want ErrUnsupported", err)
	}
	_, err = bulkQuery(db, 25).Returning(suite.DocID).Rows(t.Context())
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("Rows err = %v, want ErrUnsupported", err)
	}
	if n := len(drv.Calls()); n != 0 {
		t.Fatalf("%d statements ran, want none", n)
	}
	// a single statement still runs on the pinned connection
	if _, err := bulkQuery(db, 10).Exec(t.Context()); err != nil {
		t.Fatalf("single-statement insert: %v", err)
	}
}

func TestBulkRowsAffectedSumsChunks(t *testing.T) {
	drv := newHookDriver(50, &hooks{affected: 7})
	res, err := bulkQuery(momo.New(drv), 25).Exec(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.RowsAffected != 21 { // 3 chunks x 7
		t.Fatalf("RowsAffected = %d, want 21", res.RowsAffected)
	}
}

func rowsPerStatement(t *testing.T, drv smallLimit) []int {
	t.Helper()
	var out []int
	for _, c := range drv.Calls() {
		if strings.HasPrefix(c.SQL, "INSERT") {
			out = append(out, strings.Count(c.SQL, "), (")+1)
		}
	}
	return out
}

func TestBulkChunkSizeIsFloorOfLimitOverRowWidth(t *testing.T) {
	for _, tc := range []struct{ max, want int }{{49, 9}, {50, 10}, {54, 10}, {55, 11}} {
		drv := newSmallLimit(tc.max)
		if _, err := bulkQuery(momo.New(drv), tc.want+1).Exec(t.Context()); err != nil {
			t.Fatal(err)
		}
		got := rowsPerStatement(t, drv)
		if len(got) != 2 || got[0] != tc.want || got[1] != 1 {
			t.Errorf("limit %d: rows per statement = %v, want [%d 1]", tc.max, got, tc.want)
		}
		if a := len(drv.Calls()[0].Args); a > tc.max || a != tc.want*5 {
			t.Errorf("limit %d: first statement binds %d, want %d", tc.max, a, tc.want*5)
		}
	}
}

func TestBulkOneRowPerChunk(t *testing.T) {
	drv := newSmallLimit(5) // a row binds exactly 5 parameters
	if _, err := bulkQuery(momo.New(drv), 3).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := rowsPerStatement(t, drv); len(got) != 3 || got[0] != 1 || got[1] != 1 || got[2] != 1 {
		t.Fatalf("rows per statement = %v, want [1 1 1]", got)
	}
}

func TestBulkFixedParamsShrinkChunks(t *testing.T) {
	doUpdate := func(db *momo.DB, n int) momo.InsertQuery[suite.Document] {
		return bulkQuery(db, n).OnConflict(suite.DocID).DoUpdate(suite.DocTenantID.Set(5))
	}
	// 5 row columns + 1 bound DoUpdate value: (50-1)/5 = 9 rows, not 10.
	drv := newSmallLimit(50)
	if _, err := doUpdate(momo.New(drv), 25).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	got := rowsPerStatement(t, drv)
	if len(got) != 3 || got[0] != 9 || got[1] != 9 || got[2] != 7 {
		t.Fatalf("rows per statement = %v, want [9 9 7]", got)
	}
	for i, c := range drv.Calls() {
		if len(c.Args) > 50 {
			t.Errorf("statement %d binds %d, over the limit", i, len(c.Args))
		}
	}
	// exactly one row fits next to the fixed value
	drv = newSmallLimit(6)
	if _, err := doUpdate(momo.New(drv), 2).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := rowsPerStatement(t, drv); len(got) != 2 || got[0] != 1 || got[1] != 1 {
		t.Fatalf("rows per statement = %v, want [1 1]", got)
	}
	// the fixed value leaves no room for a row
	_, err := doUpdate(momo.New(newSmallLimit(5)), 2).Exec(t.Context())
	if !errors.Is(err, momo.ErrLimitExceeded) {
		t.Fatalf("err = %v, want ErrLimitExceeded", err)
	}
}

func TestBulkNoParamLimitRunsOneStatement(t *testing.T) {
	for _, max := range []int{0, -1} {
		drv := newSmallLimit(max)
		if _, err := bulkQuery(momo.New(drv), 100).Exec(t.Context()); err != nil {
			t.Fatalf("limit %d: %v", max, err)
		}
		if n := len(drv.Calls()); n != 1 || drv.Commits() != 0 {
			t.Errorf("limit %d: %d statements, %d commits, want one statement and no transaction", max, n, drv.Commits())
		}
	}
}
