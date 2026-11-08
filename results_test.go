// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
)

// Pluck selects only the given expression; the model's other columns stay out
// of the statement. Fails if Pluck scans into the model's full column list.
func TestPluckSelectsOneColumn(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{"a"}, {"b"}})
	titles, err := momo.Pluck(context.Background(), momo.Select[unitDocument](momo.New(drv)).OrderBy(DocID.Asc()), DocTitle)
	if err != nil || len(titles) != 2 || titles[1] != "b" {
		t.Fatalf("Pluck = %v, %v", titles, err)
	}
	sql := drv.Calls()[0].SQL
	if !strings.HasPrefix(sql, `SELECT "documents"."title" FROM`) || strings.Contains(sql, `"published"`) {
		t.Fatalf("SQL = %s", sql)
	}
}

func TestPluckHonorsMaxRows(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{"a"}, {"b"}, {"c"}})
	_, err := momo.Pluck(context.Background(), momo.Select[unitDocument](momo.New(drv)).MaxRows(2), DocTitle)
	if !errors.Is(err, momo.ErrLimitExceeded) {
		t.Fatalf("err = %v, want ErrLimitExceeded", err)
	}
}

func TestIDsAndFirstID(t *testing.T) {
	ctx := context.Background()
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{int64(7)}, {int64(9)}})
	db := momo.New(drv)
	ids, err := momo.IDs(ctx, momo.Select[unitDocument](db))
	if err != nil || len(ids) != 2 || ids[0] != 7 {
		t.Fatalf("IDs = %v, %v", ids, err)
	}
	if got := drv.Calls()[0].SQL; !strings.HasPrefix(got, `SELECT "documents"."id" FROM`) {
		t.Fatalf("SQL = %s", got)
	}
	id, err := momo.FirstID(ctx, momo.Select[unitDocument](db))
	if err != nil || id != 7 {
		t.Fatalf("FirstID = %d, %v", id, err)
	}
	if got := drv.Calls()[1]; !strings.Contains(got.SQL, "LIMIT") || got.Args[len(got.Args)-1] != int64(1) {
		t.Fatalf("FirstID call = %+v, want LIMIT 1", got)
	}
	drv.SetRows(nil)
	if _, err := momo.FirstID(ctx, momo.Select[unitDocument](db)); !errors.Is(err, momo.ErrNotFound) {
		t.Fatalf("FirstID on empty = %v, want ErrNotFound", err)
	}
}

// scriptedRows replays fixed rows and reports column names.
type scriptedRows struct {
	cols []string
	rows [][]any
	pos  int
}

func (r *scriptedRows) Next() bool                 { r.pos++; return r.pos <= len(r.rows) }
func (r *scriptedRows) Err() error                 { return nil }
func (r *scriptedRows) Close() error               { return nil }
func (r *scriptedRows) Columns() ([]string, error) { return r.cols, nil }
func (r *scriptedRows) Scan(dest ...any) error {
	for i, d := range dest {
		v := r.rows[r.pos-1][i]
		switch p := d.(type) {
		case *any:
			*p = v
		case *int64:
			*p = v.(int64)
		default:
			return errors.New("scriptedRows: unsupported destination")
		}
	}
	return nil
}

// scriptDriver answers statements for which pick returns rows and records all.
type scriptDriver struct {
	*fakedriver.Driver
	pick func(sql string) *scriptedRows
}

func (d scriptDriver) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	r, err := d.Driver.Query(ctx, sql, args)
	if rs := d.pick(sql); rs != nil {
		_ = r.Close()
		return rs, nil
	}
	return r, err
}

// The page keeps LIMIT and OFFSET and returns its rows; the total ignores both.
func TestFindAndCountStatements(t *testing.T) {
	fake := fakedriver.New(dialect.SQLite)
	fake.SetRows(threeRows())
	drv := scriptDriver{fake, func(sql string) *scriptedRows {
		if strings.HasPrefix(sql, "SELECT COUNT(*)") {
			return &scriptedRows{cols: []string{"count"}, rows: [][]any{{int64(7)}}}
		}
		return nil
	}}
	page, total, err := momo.FindAndCount(context.Background(), momo.Select[unitDocument](momo.New(drv)).OrderBy(DocID.Asc()).Limit(2).Offset(1))
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 3 || page[0].Title != "a" || total != 7 {
		t.Fatalf("page = %v total = %d, want the scripted page and total 7", page, total)
	}
	calls := fake.Calls()
	if len(calls) != 2 {
		t.Fatalf("%d statements, want 2", len(calls))
	}
	if !strings.Contains(calls[0].SQL, "LIMIT") || !strings.Contains(calls[0].SQL, "OFFSET") {
		t.Fatalf("page SQL = %s", calls[0].SQL)
	}
	if !strings.HasPrefix(calls[1].SQL, "SELECT COUNT(*)") || strings.Contains(calls[1].SQL, "LIMIT") || strings.Contains(calls[1].SQL, "OFFSET") {
		t.Fatalf("count SQL = %s", calls[1].SQL)
	}
}

func TestMapRows(t *testing.T) {
	ctx := context.Background()
	script := func(cols []string, rows ...[]any) momo.Driver {
		return scriptDriver{fakedriver.New(dialect.Postgres), func(string) *scriptedRows {
			return &scriptedRows{cols: cols, rows: rows}
		}}
	}
	got, err := momo.MapRows(ctx, momo.New(script([]string{"id", "title"}, []any{int64(1), "a"}, []any{int64(2), "b"})), "SELECT id, title FROM documents")
	if err != nil || len(got) != 2 || got[1]["title"] != "b" || got[0]["id"] != int64(1) {
		t.Fatalf("MapRows = %v, %v", got, err)
	}
	// With a hook the rows are wrapped; names must still come through.
	hooked := momo.New(script([]string{"id"}, []any{int64(1)}), momo.WithHook(&obstest.Recorder{}))
	if got, err := momo.MapRows(ctx, hooked, "SELECT id FROM documents"); err != nil || len(got) != 1 {
		t.Fatalf("hooked MapRows = %v, %v", got, err)
	}
	empty, err := momo.MapRows(ctx, momo.New(script([]string{"id"})), "SELECT id FROM documents WHERE false")
	if err != nil || empty != nil {
		t.Fatalf("empty result = %v, %v; want nil, nil", empty, err)
	}
	_, err = momo.MapRows(ctx, momo.New(script([]string{"id", "id"}, []any{int64(1), int64(2)})), "SELECT a.id, b.id FROM a, b")
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("duplicate names err = %v, want ErrValidation", err)
	}
	// A driver whose rows cannot name columns, bare or behind a hook wrapper.
	for name, db := range map[string]*momo.DB{
		"bare":   momo.New(fakedriver.New(dialect.Postgres)),
		"hooked": momo.New(fakedriver.New(dialect.Postgres), momo.WithHook(&obstest.Recorder{})),
	} {
		if _, err := momo.MapRows(ctx, db, "SELECT 1"); !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("%s: err = %v, want ErrUnsupported", name, err)
		}
	}
}

func TestBatchesRejectsMaxRows(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	err := momo.Batches(context.Background(), momo.Select[unitDocument](db).MaxRows(10), 2, func([]unitDocument) error { return nil })
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}

// Batches pages by the primary key, never OFFSET: the second statement
// resumes after the last key of the first batch. Fails if it used OFFSET.
func TestBatchesKeyset(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows(threeRows())
	stop := errors.New("stop")
	batches := 0
	err := momo.Batches(context.Background(), momo.Select[unitDocument](momo.New(drv)), 3, func(b []unitDocument) error {
		batches++
		if len(b) != 3 {
			t.Fatalf("batch %d has %d rows", batches, len(b))
		}
		if batches == 2 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || batches != 2 {
		t.Fatalf("err = %v after %d batches", err, batches)
	}
	calls := drv.Calls()
	if len(calls) != 2 {
		t.Fatalf("%d statements, want 2", len(calls))
	}
	for i, c := range calls {
		if strings.Contains(c.SQL, "OFFSET") || !strings.Contains(c.SQL, `ORDER BY "documents"."id"`) || !strings.Contains(c.SQL, "LIMIT") {
			t.Fatalf("statement %d = %s", i, c.SQL)
		}
	}
	// The first page binds only the LIMIT; the second binds the last key 3
	// before it.
	if len(calls[0].Args) != 1 || len(calls[1].Args) != 2 || calls[1].Args[0] != int64(3) {
		t.Fatalf("args = %v then %v, want [3] then [3 3]", calls[0].Args, calls[1].Args)
	}
}

func TestBatchesRejectsOwnedClauses(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	nop := func([]unitDocument) error { return nil }
	ctx := context.Background()
	q := momo.Select[unitDocument](momo.New(drv))
	for name, err := range map[string]error{
		"size":   momo.Batches(ctx, q, 0, nop),
		"order":  momo.Batches(ctx, q.OrderBy(DocTitle.Asc()), 2, nop),
		"limit":  momo.Batches(ctx, q.Limit(5), 2, nop),
		"offset": momo.Batches(ctx, q.Offset(5), 2, nop),
	} {
		if !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("%s: err = %v, want ErrValidation", name, err)
		}
	}
	if len(drv.Calls()) != 0 {
		t.Fatal("a rejected Batches call must not reach the database")
	}
}
