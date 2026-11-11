// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// rrRecorder records the finished statements and counts the tx setups.
type rrRecorder struct {
	mu    sync.Mutex
	after []momo.QueryEvent
	setup int
}

func (r *rrRecorder) BeforeQuery(ctx context.Context, _ *momo.QueryEvent) context.Context {
	return ctx
}

func (r *rrRecorder) AfterQuery(_ context.Context, e *momo.QueryEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.after = append(r.after, *e)
}

// raw returns the finished events whose op is "raw".
func (r *rrRecorder) raw() []momo.QueryEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []momo.QueryEvent
	for _, e := range r.after {
		if e.Op == "raw" {
			out = append(out, e)
		}
	}
	return out
}

func (r *rrRecorder) setups() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.setup
}

func rrOpen(t *testing.T, rec *rrRecorder, fn func(t *testing.T, db *momo.DB, d dialect.Name)) {
	t.Helper()
	testopen.Each(t, fn,
		momo.WithHook(rec),
		momo.WithImplicitTx(),
		momo.WithTxSetup(func(context.Context, *momo.Tx) error {
			rec.mu.Lock()
			rec.setup++
			rec.mu.Unlock()
			return nil
		}))
}

// rrRead drains rows into [id, name] pairs through a dynamic Scan.
func rrRead(t *testing.T, rows momo.ColumnRows) [][2]any {
	t.Helper()
	cols, err := rows.Columns()
	if err != nil || len(cols) != 2 || cols[0] != "id" || cols[1] != "name" {
		t.Fatalf("Columns = %v, %v", cols, err)
	}
	var out [][2]any
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatal(err)
		}
		out = append(out, [2]any{id, name})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	return out
}

// Catches: RawRows bypassing hooks (op "raw"), WithTxSetup, or the
// *DB / *Tx / adopted-transaction paths, on a real driver.
func TestRawRowsPaths(t *testing.T) {
	rec := &rrRecorder{}
	rrOpen(t, rec, func(t *testing.T, db *momo.DB, d dialect.Name) {
		rqSetup(t, db)
		ctx := context.Background()
		const stmt = "SELECT id, name FROM rq_items WHERE id > ? ORDER BY id"
		check := func(t *testing.T, q momo.Querier) {
			t.Helper()
			rows, err := momo.RawRows(ctx, q, stmt, 1)
			if err != nil {
				t.Fatal(err)
			}
			got := rrRead(t, rows)
			if len(got) != 2 || got[0] != [2]any{int64(2), "two"} || got[1] != [2]any{int64(3), "three"} {
				t.Fatalf("rows = %v", got)
			}
		}

		before, setupsBefore := len(rec.raw()), rec.setups()
		check(t, db)
		if n := len(rec.raw()) - before; n != 1 {
			t.Fatalf("*DB: %d raw hook events, want 1", n)
		}
		if rec.setups() <= setupsBefore {
			t.Fatal("*DB: WithTxSetup did not run for RawRows")
		}
		if ev := rec.raw()[len(rec.raw())-1]; ev.Err != nil || ev.Rows != 2 || ev.SQL == "" {
			t.Fatalf("*DB hook event = %+v", ev)
		}

		before = len(rec.raw())
		if err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			check(t, tx)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		evs := rec.raw()
		if len(evs) != before+1 || !evs[len(evs)-1].InTx {
			t.Fatalf("*Tx: raw hook events = %d (before %d), want one with InTx", len(evs), before)
		}

		dtx, err := db.Driver().Begin(ctx, momo.TxOptions{})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = dtx.Rollback(context.WithoutCancel(ctx)) }()
		before = len(rec.raw())
		check(t, db.AdoptTx(dtx))
		if len(rec.raw()) != before+1 {
			t.Fatalf("adopted tx: raw hook events = %d, want %d", len(rec.raw()), before+1)
		}
	})
}

// Catches: a canceled context leaving the cursor open or silent, and the
// hook pair never ending.
func TestRawRowsClosesOnCancel(t *testing.T) {
	rec := &rrRecorder{}
	rrOpen(t, rec, func(t *testing.T, db *momo.DB, d dialect.Name) {
		rqSetup(t, db)
		cctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		before := len(rec.raw())
		rows, err := momo.RawRows(cctx, db, "SELECT id, name FROM rq_items ORDER BY id")
		if err != nil {
			t.Fatal(err)
		}
		if !rows.Next() {
			t.Fatalf("first Next false: %v", rows.Err())
		}
		cancel()
		if rows.Next() {
			t.Fatal("Next after cancel returned true")
		}
		if err := rows.Err(); !errors.Is(err, momo.ErrCanceled) {
			t.Fatalf("Err = %v, want ErrCanceled", err)
		}
		if n := len(rec.raw()); n != before+1 {
			t.Fatalf("cancel did not end the hook pair: %d events (before %d)", n, before)
		}
		_ = rows.Close()
		_ = rows.Close() // safe twice
		if n := len(rec.raw()); n != before+1 {
			t.Fatalf("Close after cancel fired another hook event: %d (before %d)", n, before)
		}
	})
}

// Catches: a build error or a driver error from RawRows escaping the raw
// path's checks.
func TestRawRowsErrors(t *testing.T) {
	rec := &rrRecorder{}
	rrOpen(t, rec, func(t *testing.T, db *momo.DB, d dialect.Name) {
		rqSetup(t, db)
		ctx := context.Background()
		if _, err := momo.RawRows(ctx, db, "SELECT id FROM rq_items WHERE id = ?"); err == nil {
			t.Fatal("argument-count mismatch must be a build error")
		}
		// A driver may report a server error at open or on the first Next.
		rows, err := momo.RawRows(ctx, db, "SELECT * FROM rq_missing_table")
		if err == nil {
			defer func() { _ = rows.Close() }()
			if rows.Next() {
				t.Fatal("missing table returned a row")
			}
			err = rows.Err()
		}
		if err == nil {
			t.Fatal("missing table must fail")
		}
	})
}
