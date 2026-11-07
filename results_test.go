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

// The total ignores LIMIT and OFFSET while the page keeps them. The scripted
// rows are model-shaped, so the count scan itself fails; the two statements
// are what this test inspects.
func TestFindAndCountStatements(t *testing.T) {
	drv := fakedriver.New(dialect.SQLite)
	drv.SetRows(threeRows())
	_, _, _ = momo.FindAndCount(context.Background(), momo.Select[unitDocument](momo.New(drv)).OrderBy(DocID.Asc()).Limit(2).Offset(1))
	calls := drv.Calls()
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
