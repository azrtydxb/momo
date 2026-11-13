// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
)

// smallLimit is a fake driver whose dialect allows only max bind parameters.
type smallLimit struct {
	*fakedriver.Driver
	d *dialect.Dialect
}

func (s smallLimit) Dialect() *dialect.Dialect { return s.d }

func newSmallLimit(max int) smallLimit {
	d := *dialect.For(dialect.Postgres)
	d.MaxParams = max
	return smallLimit{Driver: fakedriver.New(dialect.Postgres), d: &d}
}

// bulkQuery builds an insert of n documents that omits body, secret and
// deleted_at, so each row binds 5 columns: tenant_id, title, version,
// created_at and updated_at (the identity column is never written).
func bulkQuery(db *momo.DB, n int) momo.InsertQuery[suite.Document] {
	ms := make([]suite.Document, n)
	for i := range ms {
		ms[i] = suite.Document{TenantID: int64(i), Title: "b"}
	}
	return momo.Insert[suite.Document](db).Omit(suite.DocBody, suite.DocSecret, suite.DocDeletedAt).Models(ms)
}

func TestBulkChunkBoundaries(t *testing.T) {
	const cols, max = 5, 50 // 10 rows per statement
	for _, tc := range []struct {
		name string
		rows int
		want []int // rows per statement
	}{
		{"below limit", 9, []int{9}},
		{"exactly at limit", 10, []int{10}},
		{"limit plus one", 11, []int{10, 1}},
		{"two full chunks", 20, []int{10, 10}},
		{"two full plus one", 21, []int{10, 10, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			drv := newSmallLimit(max)
			db := momo.New(drv)
			if _, err := bulkQuery(db, tc.rows).Exec(t.Context()); err != nil {
				t.Fatal(err)
			}
			calls := drv.Calls()
			if len(calls) != len(tc.want) {
				t.Fatalf("%d statements, want %d", len(calls), len(tc.want))
			}
			for i, c := range calls {
				if len(c.Args) != tc.want[i]*cols || len(c.Args) > max {
					t.Errorf("statement %d binds %d args, want %d (limit %d)", i, len(c.Args), tc.want[i]*cols, max)
				}
			}
			wantTx := 0
			if len(tc.want) > 1 {
				wantTx = 1
			}
			if drv.Commits() != wantTx {
				t.Errorf("commits = %d, want %d: chunks must share one transaction", drv.Commits(), wantTx)
			}
			// rows keep their order across chunks
			next := int64(0)
			for _, c := range calls {
				for r := 0; r < len(c.Args)/cols; r++ {
					if c.Args[r*cols] != next {
						t.Fatalf("row order broken at %d", next)
					}
					next++
				}
			}
		})
	}
}

func TestBulkSingleRowOverLimit(t *testing.T) {
	drv := newSmallLimit(4) // a row binds 5 parameters
	db := momo.New(drv)
	_, err := bulkQuery(db, 3).Exec(t.Context())
	if !errors.Is(err, momo.ErrLimitExceeded) {
		t.Fatalf("err = %v, want ErrLimitExceeded", err)
	}
	if n := len(drv.Calls()); n != 0 {
		t.Fatalf("%d statements ran, want none", n)
	}
}

func TestBulkChunksShareOneTimestamp(t *testing.T) {
	drv := newSmallLimit(50)
	reads := 0
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	db := momo.New(drv, momo.WithClock(func() time.Time { reads++; return base.Add(time.Duration(reads) * time.Hour) }))
	if _, err := bulkQuery(db, 25).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	if reads != 1 {
		t.Fatalf("clock read %d times, want once for the whole Exec", reads)
	}
	var ts any
	for _, c := range drv.Calls() {
		for r := 0; r < len(c.Args)/5; r++ {
			got := c.Args[r*5+3] // created_at
			if ts == nil {
				ts = got
			}
			if got != ts {
				t.Fatalf("created_at %v differs from %v across chunks", got, ts)
			}
		}
	}
}

func TestBulkUpsertChunksKeepIdentityConsistent(t *testing.T) {
	drv := newSmallLimit(60) // 6 bound columns with identity: 10 rows per statement
	db := momo.New(drv)
	ms := make([]suite.Document, 25)
	for i := range ms {
		ms[i] = suite.Document{ID: int64(i + 1), TenantID: 1, Title: "u"}
	}
	if _, err := momo.Upsert[suite.Document](db).Omit(suite.DocBody, suite.DocSecret, suite.DocDeletedAt).Models(ms).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	calls := drv.Calls()
	if len(calls) != 3 {
		t.Fatalf("%d statements, want 3", len(calls))
	}
	for i, c := range calls {
		if len(c.Args) > 60 {
			t.Errorf("statement %d binds %d args", i, len(c.Args))
		}
		for _, frag := range []string{`OVERRIDING SYSTEM VALUE`, `("id", `, `ON CONFLICT ("id") DO UPDATE SET`} {
			if !strings.Contains(c.SQL, frag) {
				t.Errorf("statement %d lacks %q: %s", i, frag, c.SQL)
			}
		}
	}
}

func TestBulkRowsReturnsChunksInOrder(t *testing.T) {
	drv := newSmallLimit(50)
	db := momo.New(drv)
	drv.SetRows([][]any{{int64(1)}, {int64(2)}})
	rows, err := bulkQuery(db, 25).Returning(suite.DocID).Rows(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for rows.Next() {
		n++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(drv.Calls()) != 3 || n != 6 { // 3 chunks x 2 scripted rows
		t.Fatalf("statements = %d, rows = %d, want 3 and 6", len(drv.Calls()), n)
	}
	if drv.Commits() != 1 {
		t.Fatalf("commits = %d, want 1", drv.Commits())
	}
}

func TestBulkRowsEarlyCloseRunsRemainingChunks(t *testing.T) {
	drv := newSmallLimit(50)
	db := momo.New(drv)
	rows, err := bulkQuery(db, 25).Returning(suite.DocID).Rows(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(drv.Calls()) != 3 || drv.Commits() != 1 {
		t.Fatalf("statements = %d commits = %d, want 3 and 1", len(drv.Calls()), drv.Commits())
	}
}

func TestBulkChunksOnCallerTxUseSavepoint(t *testing.T) {
	drv := newSmallLimit(50)
	db := momo.New(drv)
	tx, err := db.Begin(t.Context(), momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ms := make([]suite.Document, 25)
	_, err = momo.Insert[suite.Document](tx).Omit(suite.DocBody, suite.DocSecret, suite.DocDeletedAt).Models(ms).Exec(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var sp, rel int
	for _, c := range drv.Calls() {
		switch {
		case strings.HasPrefix(c.SQL, "SAVEPOINT"):
			sp++
		case strings.HasPrefix(c.SQL, "RELEASE"):
			rel++
		}
	}
	if sp != 1 || rel != 1 {
		t.Fatalf("savepoints = %d releases = %d, want one scope around the chunks", sp, rel)
	}
	_ = tx.Rollback(t.Context())
}

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
	// 5 row columns + 2 bound values (the DoUpdate value and the updated_at
	// bump): (50-2)/5 = 9 rows, not 10.
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
	// exactly one row fits next to the fixed values
	drv = newSmallLimit(7)
	if _, err := doUpdate(momo.New(drv), 2).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := rowsPerStatement(t, drv); len(got) != 2 || got[0] != 1 || got[1] != 1 {
		t.Fatalf("rows per statement = %v, want [1 1]", got)
	}
	// the fixed values leave no room for a row
	_, err := doUpdate(momo.New(newSmallLimit(6)), 2).Exec(t.Context())
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
