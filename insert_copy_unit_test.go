// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/suite"
)

// copyLog records which COPY path ran, in order, with its table, columns and rows.
type copyLog struct {
	mu     sync.Mutex
	events []string
	table  string
	cols   []string
	rows   [][]any
}

func (l *copyLog) add(ev string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, ev)
}

func (l *copyLog) copied(ev, table string, cols []string, rows [][]any) int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, ev)
	l.table, l.cols, l.rows = table, cols, rows
	return int64(len(rows))
}

func (l *copyLog) Events() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.events)
}

// copyDriver is a fake driver whose pool and transactions implement momo.Copier.
type copyDriver struct {
	*fakedriver.Driver
	log *copyLog
}

func newCopyDriver(d dialect.Name) copyDriver {
	return copyDriver{Driver: fakedriver.New(d), log: &copyLog{}}
}

func (c copyDriver) CopyFrom(_ context.Context, table string, cols []string, rows [][]any) (int64, error) {
	return c.log.copied("copy-db", table, cols, rows), nil
}

func (c copyDriver) Begin(ctx context.Context, o momo.TxOptions) (momo.DriverTx, error) {
	tx, err := c.Driver.Begin(ctx, o)
	if err != nil {
		return nil, err
	}
	c.log.add("begin")
	return copyTx{DriverTx: tx, log: c.log}, nil
}

type copyTx struct {
	momo.DriverTx
	log *copyLog
}

func (t copyTx) CopyFrom(_ context.Context, table string, cols []string, rows [][]any) (int64, error) {
	return t.log.copied("copy-tx", table, cols, rows), nil
}

// copyPinned is a pinned connection with a COPY path.
type copyPinned struct{ log *copyLog }

func (copyPinned) Query(context.Context, string, []any) (momo.Rows, error) {
	return nil, errors.New("unexpected query")
}
func (copyPinned) Exec(context.Context, string, []any) (int64, error) {
	return 0, errors.New("unexpected exec")
}
func (copyPinned) Begin(context.Context, momo.TxOptions) (momo.DriverTx, error) {
	return nil, errors.New("unexpected begin")
}
func (copyPinned) Release() error { return nil }
func (p copyPinned) CopyFrom(_ context.Context, table string, cols []string, rows [][]any) (int64, error) {
	return p.log.copied("copy-pinned", table, cols, rows), nil
}

func copyDocs() []suite.Document {
	return []suite.Document{{TenantID: 1, Title: "c1"}, {TenantID: 2, Title: "c2"}}
}

func copyQuery(db momo.Querier) momo.InsertQuery[suite.Document] {
	return momo.Insert[suite.Document](db).Omit(suite.DocBody, suite.DocSecret, suite.DocDeletedAt)
}

// Catches: columns and row values going out of step, an omitted column that is
// still copied, an unset timestamp left zero, and the row count not reported.
func TestCopyStreamsModelColumnsInOrder(t *testing.T) {
	drv := newCopyDriver(dialect.Postgres)
	db := momo.New(drv)
	res, err := copyQuery(db).Copy(t.Context(), copyDocs())
	if err != nil || res.RowsAffected != 2 {
		t.Fatalf("Copy = %+v, %v", res, err)
	}
	want := []string{"tenant_id", "title", "version", "created_at", "updated_at"}
	if drv.log.table != "documents" || !slices.Equal(drv.log.cols, want) {
		t.Fatalf("table %q cols %v, want documents %v", drv.log.table, drv.log.cols, want)
	}
	if len(drv.log.rows) != 2 || drv.log.rows[1][0] != int64(2) || drv.log.rows[1][1] != "c2" {
		t.Fatalf("rows = %v", drv.log.rows)
	}
	for _, r := range drv.log.rows {
		if len(r) != len(want) {
			t.Fatalf("row %v has %d values for %d columns", r, len(r), len(want))
		}
		if ts, ok := r[3].(time.Time); !ok || ts.IsZero() {
			t.Fatalf("created_at = %v, want the statement time", r[3])
		}
	}
	if got := drv.log.Events(); !slices.Equal(got, []string{"copy-db"}) {
		t.Fatalf("events = %v, want one pool COPY and no transaction", got)
	}
	if len(drv.Calls()) != 0 {
		t.Fatalf("COPY ran statements: %v", drv.Calls())
	}
}

// Catches: a missing Copier path falling back to a different write, or the
// error kind changing.
func TestCopyWithoutCopierIsUnsupported(t *testing.T) {
	for _, name := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
		drv := fakedriver.New(name)
		_, err := copyQuery(momo.New(drv)).Copy(t.Context(), copyDocs())
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("%s: Copy = %v, want ErrUnsupported", name, err)
		}
		if len(drv.Calls()) != 0 {
			t.Fatalf("%s: unsupported Copy ran %v", name, drv.Calls())
		}
	}
}

// Catches: Copy skipping WithTxSetup on an implicit-transaction DB (the RLS
// bypass), and the setup running after the COPY.
func TestCopyRunsTxSetupFirstOnImplicitTx(t *testing.T) {
	drv := newCopyDriver(dialect.Postgres)
	db := momo.New(drv, momo.WithImplicitTx(), momo.WithTxSetup(func(context.Context, *momo.Tx) error {
		drv.log.add("setup")
		return nil
	}))
	res, err := copyQuery(db).Copy(t.Context(), copyDocs())
	if err != nil || res.RowsAffected != 2 {
		t.Fatalf("Copy = %+v, %v", res, err)
	}
	if got := drv.log.Events(); !slices.Equal(got, []string{"begin", "setup", "copy-tx"}) {
		t.Fatalf("events = %v, want begin, setup, then the transaction's COPY", got)
	}
	if drv.Commits() != 1 || drv.Rollbacks() != 0 {
		t.Fatalf("commits %d rollbacks %d, want 1 and 0", drv.Commits(), drv.Rollbacks())
	}
}

// Catches: a failing setup still letting the COPY run.
func TestCopyStopsWhenTxSetupFails(t *testing.T) {
	drv := newCopyDriver(dialect.Postgres)
	boom := errors.New("setup failed")
	db := momo.New(drv, momo.WithImplicitTx(), momo.WithTxSetup(func(context.Context, *momo.Tx) error { return boom }))
	if _, err := copyQuery(db).Copy(t.Context(), copyDocs()); !errors.Is(err, boom) {
		t.Fatalf("Copy = %v, want the setup error", err)
	}
	if slices.Contains(drv.log.Events(), "copy-tx") || slices.Contains(drv.log.Events(), "copy-db") {
		t.Fatalf("events = %v: COPY ran after a failed setup", drv.log.Events())
	}
}

// Catches: a pinned handle with a setup copying unscoped instead of failing
// closed, and a pinned handle without one losing its COPY path.
func TestCopyOnPinnedConn(t *testing.T) {
	drv := newCopyDriver(dialect.Postgres)
	pc := copyPinned{log: drv.log}

	scoped := momo.New(drv, momo.WithTxSetup(func(context.Context, *momo.Tx) error { return nil })).OnConn(pc)
	if _, err := copyQuery(scoped).Copy(t.Context(), copyDocs()); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("Copy on a pinned handle with a setup = %v, want ErrUnsupported", err)
	}
	if len(drv.log.Events()) != 0 {
		t.Fatalf("events = %v: the unscoped COPY ran", drv.log.Events())
	}

	plain := momo.New(drv).OnConn(pc)
	res, err := copyQuery(plain).Copy(t.Context(), copyDocs())
	if err != nil || res.RowsAffected != 2 {
		t.Fatalf("Copy on a plain pinned handle = %+v, %v", res, err)
	}
	if got := drv.log.Events(); !slices.Equal(got, []string{"copy-pinned"}) {
		t.Fatalf("events = %v, want the pinned connection's COPY", got)
	}
}

// Catches: Copy opening a second transaction inside the caller's Tx, or using
// the pool and so leaving the transaction.
func TestCopyInsideTxUsesTheTransaction(t *testing.T) {
	drv := newCopyDriver(dialect.Postgres)
	db := momo.New(drv)
	err := db.Tx(t.Context(), momo.TxOptions{}, func(tx *momo.Tx) error {
		_, err := copyQuery(tx).Copy(t.Context(), copyDocs())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := drv.log.Events(); !slices.Equal(got, []string{"begin", "copy-tx"}) {
		t.Fatalf("events = %v, want one begin and the transaction's COPY", got)
	}
}

// Catches: a sensitive value reaching a hook through the COPY event.
func TestCopyHidesSensitiveValuesFromHooks(t *testing.T) {
	rec := &obstest.Recorder{}
	drv := newCopyDriver(dialect.Postgres)
	db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
	ms := []suite.Document{{TenantID: 1, Title: "t", Secret: sensitiveValue}}
	if _, err := momo.Insert[suite.Document](db).Omit(suite.DocBody, suite.DocDeletedAt).Copy(t.Context(), ms); err != nil {
		t.Fatal(err)
	}
	evs := rec.Events()
	if len(evs) != 1 || evs[0].Op != "copy" || evs[0].Rows != 1 {
		t.Fatalf("events = %+v, want one copy event of 1 row", evs)
	}
	if len(evs[0].Args) != 0 || strings.Contains(evs[0].SQL, sensitiveValue) {
		t.Fatalf("hook saw values: %q %v", evs[0].SQL, evs[0].Args)
	}
	if !strings.HasPrefix(evs[0].SQL, `COPY "documents" ("tenant_id", "title"`) {
		t.Fatalf("event SQL = %q", evs[0].SQL)
	}
	sent := false
	for _, r := range drv.log.rows {
		sent = sent || slices.Contains(r, any(sensitiveValue))
	}
	if !sent {
		t.Fatal("the driver did not receive the real value")
	}
}

// Catches: Copy accepting a query it cannot honour (conflict handling, RETURNING,
// other row sources) and dropping that part silently.
func TestCopyRejectsIncompatibleQueries(t *testing.T) {
	db := momo.New(newCopyDriver(dialect.Postgres))
	ctx := t.Context()
	if _, err := copyQuery(db).Copy(ctx, nil); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("no rows = %v, want ErrValidation", err)
	}
	cases := map[string]momo.InsertQuery[suite.Document]{
		"models":    copyQuery(db).Models(copyDocs()),
		"returning": copyQuery(db).Returning(suite.DocID),
		"conflict":  copyQuery(db).OnConflict(suite.DocID).DoNothing(),
		"values":    momo.Insert[suite.Document](db).Values(suite.DocTitle.Set("x")),
	}
	for name, q := range cases {
		if _, err := q.Copy(ctx, copyDocs()); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: Copy = %v, want ErrValidation", name, err)
		}
	}
}
