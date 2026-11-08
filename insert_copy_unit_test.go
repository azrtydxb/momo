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

// Catches: Copy skipping BeforeInsert (a tenant-stamping model writes unstamped
// rows), mutating the caller's slice, or running after a hook veto.
func TestCopyRunsBeforeInsertOverACopy(t *testing.T) {
	veto := errors.New("veto")
	setHooks(t, momo.HookSet[hookedDoc]{
		BeforeInsert: func(_ context.Context, m *hookedDoc) error {
			if m.Title == "bad" {
				return veto
			}
			m.Secret = "stamped"
			return nil
		},
	})
	drv := newCopyDriver(dialect.Postgres)
	db := momo.New(drv)
	ms := []hookedDoc{{Title: "a"}, {Title: "b"}}
	if _, err := momo.Insert[hookedDoc](db).Copy(t.Context(), ms); err != nil {
		t.Fatal(err)
	}
	if len(drv.log.rows) != 2 || !slices.Equal(drv.log.cols, []string{"title", "secret"}) {
		t.Fatalf("cols %v rows %v", drv.log.cols, drv.log.rows)
	}
	for _, r := range drv.log.rows {
		if r[1] != "stamped" {
			t.Fatalf("row %v was not stamped by BeforeInsert", r)
		}
	}
	if ms[0].Secret != "" || ms[1].Secret != "" {
		t.Fatalf("BeforeInsert changed the caller's models: %+v", ms)
	}

	drv2 := newCopyDriver(dialect.Postgres)
	_, err := momo.Insert[hookedDoc](momo.New(drv2)).Copy(t.Context(), []hookedDoc{{Title: "a"}, {Title: "bad"}})
	if !errors.Is(err, momo.ErrValidation) || !errors.Is(err, veto) {
		t.Fatalf("Copy with a vetoing hook = %v", err)
	}
	if len(drv2.log.Events()) != 0 {
		t.Fatalf("events = %v: COPY ran after a veto", drv2.log.Events())
	}
}

// Catches: Copy bypassing the insert interceptors.
func TestCopyIsDeniedByInterceptor(t *testing.T) {
	drv := newCopyDriver(dialect.Postgres)
	var kind string
	db := momo.New(drv, momo.WithInterceptor(func(_ context.Context, op *momo.Operation) error {
		kind = op.Kind
		return errors.New("not on this handle")
	}))
	if _, err := copyQuery(db).Copy(t.Context(), copyDocs()); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("Copy = %v, want ErrValidation", err)
	}
	if kind != "insert" || len(drv.log.Events()) != 0 {
		t.Fatalf("kind %q events %v", kind, drv.log.Events())
	}
}

// Catches: a cancelled context still starting the COPY.
func TestCopyHonoursCancelledContext(t *testing.T) {
	drv := newCopyDriver(dialect.Postgres)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := copyQuery(momo.New(drv)).Copy(ctx, copyDocs()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Copy = %v, want context.Canceled", err)
	}
	if len(drv.log.Events()) != 0 {
		t.Fatalf("events = %v: COPY ran on a cancelled context", drv.log.Events())
	}
}

// Catches: COPY and MERGE failing on a replica handle (the router hid the
// primary's Copier and ServerVersioner), COPY going anywhere but the primary,
// and a Copy that is not recorded as a write, so a later read in the same
// unit of work misses the copied rows on the replica.
func TestReplicaHandleCopyMergeAndReadYourWrites(t *testing.T) {
	prim := versionedCopy{copyDriver: newCopyDriver(dialect.Postgres), version: 170000}
	rep := fakedriver.New(dialect.Postgres)
	rep.SetRows([][]any{{int64(0)}})
	prim.SetRows([][]any{{int64(2)}})
	rd, err := momo.New(prim, momo.WithReplicas(rep)).Replica(0)
	if err != nil {
		t.Fatal(err)
	}
	ctx := momo.NewUnitOfWork(t.Context())
	count := func() int64 {
		n, err := momo.Select[suite.Document](rd).Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(); n != 0 || len(rep.Calls()) != 1 {
		t.Fatalf("before the write: count %d, replica calls %d, want a replica read", n, len(rep.Calls()))
	}
	if res, err := copyQuery(rd).Copy(ctx, copyDocs()); err != nil || res.RowsAffected != 2 {
		t.Fatalf("Copy on a replica handle = %+v, %v", res, err)
	}
	if got := prim.log.Events(); !slices.Equal(got, []string{"copy-db"}) {
		t.Fatalf("events = %v, want the primary's COPY", got)
	}
	if n := count(); n != 2 || len(rep.Calls()) != 1 {
		t.Fatalf("after Copy: count %d, replica calls %d, want a primary read", n, len(rep.Calls()))
	}

	if _, err := mergeInsertQuery(rd).Exec(t.Context()); err != nil {
		t.Fatalf("Merge on a replica handle = %v", err)
	}
	last := prim.Calls()[len(prim.Calls())-1]
	if !strings.HasPrefix(last.SQL, "MERGE INTO") {
		t.Fatalf("last primary statement = %q, want the MERGE", last.SQL)
	}
}

// versionedCopy is a copy-capable fake driver that reports a server version.
type versionedCopy struct {
	copyDriver
	version int
}

func (v versionedCopy) ServerVersion(context.Context) (int, error) { return v.version, nil }

// Catches: Truncate running without the interceptors (a tenant handle wiping
// every tenant), a wrong kind, and MySQL TRUNCATE (an implicit commit) inside
// a transaction.
func TestTruncateInterceptorsAndMySQLTransaction(t *testing.T) {
	var kind string
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithInterceptor(func(_ context.Context, op *momo.Operation) error {
		kind = op.Kind
		return errors.New("no truncate on a tenant handle")
	}))
	if err := momo.Truncate[suite.Document](t.Context(), db); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("Truncate = %v, want ErrValidation", err)
	}
	if kind != "truncate" || len(drv.Calls()) != 0 {
		t.Fatalf("kind %q calls %v", kind, drv.Calls())
	}

	my := fakedriver.New(dialect.MySQL)
	mdb := momo.New(my)
	err := mdb.Tx(t.Context(), momo.TxOptions{}, func(tx *momo.Tx) error {
		return momo.Truncate[suite.Document](t.Context(), tx)
	})
	if !errors.Is(err, momo.ErrUnsupported) || len(my.Calls()) != 0 {
		t.Fatalf("MySQL Truncate in a Tx = %v, calls %v, want ErrUnsupported and none", err, my.Calls())
	}
	if err := momo.Truncate[suite.Document](t.Context(), mdb); err != nil {
		t.Fatal(err)
	}
	if c := my.Calls(); len(c) != 1 || !strings.HasPrefix(c[0].SQL, "TRUNCATE TABLE ") {
		t.Fatalf("MySQL calls = %+v", c)
	}
}
