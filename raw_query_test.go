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

// namedDriver reports column names for the rows of the fake driver, as the
// real drivers do.
type namedDriver struct {
	*fakedriver.Driver
	names []string
}

type namedRows struct {
	momo.Rows
	names []string
}

func (r namedRows) Columns() ([]string, error) { return r.names, nil }

func (d namedDriver) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	r, err := d.Driver.Query(ctx, sql, args)
	if err != nil {
		return nil, err
	}
	return namedRows{Rows: r, names: d.names}, nil
}

type rawEvents struct{ ops []string }

func (h *rawEvents) BeforeQuery(ctx context.Context, _ *momo.QueryEvent) context.Context { return ctx }
func (h *rawEvents) AfterQuery(_ context.Context, e *momo.QueryEvent)                    { h.ops = append(h.ops, e.Op) }

// Catches: a tuple row not scanned positionally, or the SQL not rendered
// through the raw placeholder rules.
func TestQueryTupleRow(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{"a", int64(1)}, {"b", int64(2)}})
	db := momo.New(drv)
	got, err := momo.Query[momo.Row2[string, int64]](t.Context(), db, "SELECT title, n FROM t WHERE n > ? AND k = $2", 0)
	if err == nil {
		t.Fatalf("mixing ? and $n must fail, got %v", got)
	}
	got, err = momo.Query[momo.Row2[string, int64]](t.Context(), db, "SELECT title, n FROM t WHERE n > ?", 0)
	if err != nil || len(got) != 2 || got[0].C1 != "a" || got[1].C2 != 2 {
		t.Fatalf("Query = %+v, %v", got, err)
	}
	if c := drv.Calls(); len(c) != 1 || c[0].SQL != "SELECT title, n FROM t WHERE n > $1" {
		t.Fatalf("calls = %+v", c)
	}
	got, err = momo.Query[momo.Row2[string, int64]](t.Context(), db, "SELECT title, n FROM t WHERE n > $1 OR n < $1", 5)
	if err != nil || len(got) != 2 {
		t.Fatalf("native $n = %+v, %v", got, err)
	}
	if c := drv.Calls(); c[len(c)-1].SQL != "SELECT title, n FROM t WHERE n > $1 OR n < $1" || len(c[len(c)-1].Args) != 1 {
		t.Fatalf("calls = %+v", c)
	}
}

// Catches: an empty result becoming an error or a non-nil slice.
func TestQueryEmptyResult(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	got, err := momo.Query[momo.Row1[int64]](t.Context(), momo.New(drv), "SELECT 1 WHERE false")
	if err != nil || got != nil {
		t.Fatalf("Query = %v, %v; want nil, nil", got, err)
	}
}

// Catches: a tuple whose arity differs from the result being scanned.
func TestQueryTupleArityMismatch(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{"a", int64(1)}})
	db := momo.New(namedDriver{drv, []string{"title", "n"}})
	_, err := momo.Query[momo.Row3[string, int64, bool]](t.Context(), db, "SELECT title, n FROM t")
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
	if drv.Closed() < 1 {
		t.Fatalf("rows closed %d times, want 1", drv.Closed())
	}
}

// Catches: a model scanned positionally when the SQL lists its columns in
// another order, and the unknown/missing/duplicate column errors.
func TestQueryModelMatchesByName(t *testing.T) {
	ctx := t.Context()
	run := func(names []string, rows [][]any) ([]hookedDoc, error) {
		drv := fakedriver.New(dialect.Postgres)
		drv.SetRows(rows)
		return momo.Query[hookedDoc](ctx, momo.New(namedDriver{drv, names}), "SELECT ...")
	}

	got, err := run([]string{"title", "secret", "id"}, [][]any{{"t", "s", int64(7)}})
	if err != nil || len(got) != 1 || got[0].ID != 7 || got[0].Title != "t" || got[0].Secret != "s" {
		t.Fatalf("reordered = %+v, %v", got, err)
	}
	got, err = run([]string{"ID", "Title", "SECRET"}, [][]any{{int64(1), "t", "s"}})
	if err != nil || len(got) != 1 || got[0].Title != "t" {
		t.Fatalf("case-folded = %+v, %v", got, err)
	}

	for name, names := range map[string][]string{
		"unknown":   {"id", "title", "secret", "extra"},
		"missing":   {"id", "title"},
		"duplicate": {"id", "title", "title"},
	} {
		if _, err := run(names, nil); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s column: err = %v, want ErrValidation", name, err)
		}
	}
}

// Catches: the unsupported-names fallback not scanning positionally.
func TestQueryModelWithoutColumnNames(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{int64(3), "t", "s"}})
	got, err := momo.Query[hookedDoc](t.Context(), momo.New(drv), "SELECT * FROM hooked_docs")
	if err != nil || len(got) != 1 || got[0].ID != 3 {
		t.Fatalf("Query = %+v, %v", got, err)
	}
}

// Catches: the raw statement bypassing the observability hooks.
func TestQueryRunsHooksAndTx(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{int64(1)}})
	h := &rawEvents{}
	db := momo.New(drv, momo.WithHook(h))
	ctx := t.Context()
	if _, err := momo.Query[momo.Row1[int64]](ctx, db, "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		got, err := momo.Query[momo.Row1[int64]](ctx, tx, "SELECT 1")
		if err == nil && len(got) != 1 {
			err = errors.New("no row")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	raw := 0
	for _, op := range h.ops {
		if op == "raw" {
			raw++
		}
	}
	if raw != 2 {
		t.Fatalf("hook ops = %v, want two raw events", h.ops)
	}
}

// Catches: a build error (argument count) reaching the database, or the
// streaming form buffering/leaking its cursor on early break.
func TestQueryRows(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{int64(1)}, {int64(2)}, {int64(3)}})
	db := momo.New(drv)
	ctx := t.Context()

	n := 0
	for r, err := range momo.QueryRows[momo.Row1[int64]](ctx, db, "SELECT id FROM t") {
		if err != nil {
			t.Fatal(err)
		}
		n++
		if r.C1 == 2 {
			break
		}
	}
	if n != 2 || drv.Closed() < 1 {
		t.Fatalf("early break: %d rows read, %d cursors closed; want 2 and at least 1", n, drv.Closed())
	}

	afterBreak := drv.Closed()
	var sum int64
	for r, err := range momo.QueryRows[momo.Row1[int64]](ctx, db, "SELECT id FROM t") {
		if err != nil {
			t.Fatal(err)
		}
		sum += r.C1
	}
	if sum != 6 || drv.Closed() <= afterBreak {
		t.Fatalf("full read: sum %d, closed %d after %d; want 6 and a further close", sum, drv.Closed(), afterBreak)
	}

	before := len(drv.Calls())
	var gotErr error
	for _, err := range momo.QueryRows[momo.Row1[int64]](ctx, db, "SELECT id FROM t WHERE a = ?") {
		gotErr = err
	}
	if gotErr == nil || len(drv.Calls()) != before {
		t.Fatalf("argument-count mismatch: err = %v, statements = %d (want %d)", gotErr, len(drv.Calls()), before)
	}

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var cancelErr error
	for _, err := range momo.QueryRows[momo.Row1[int64]](cctx, db, "SELECT id FROM t") {
		if err != nil {
			cancelErr = err
			break
		}
		cancel()
	}
	if !errors.Is(cancelErr, momo.ErrCanceled) {
		t.Fatalf("canceled stream err = %v, want ErrCanceled", cancelErr)
	}
}
