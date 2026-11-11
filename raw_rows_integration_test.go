// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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
	rrOpenMode(t, rec, true, fn)
}

// rrOpenMode opens PostgreSQL and SQLite with a hook and a tx setup, with or
// without WithImplicitTx.
func rrOpenMode(t *testing.T, rec *rrRecorder, implicit bool, fn func(t *testing.T, db *momo.DB, d dialect.Name)) {
	t.Helper()
	opts := []momo.Option{
		momo.WithHook(rec),
		momo.WithTxSetup(func(context.Context, *momo.Tx) error {
			rec.mu.Lock()
			rec.setup++
			rec.mu.Unlock()
			return nil
		}),
	}
	if implicit {
		opts = append(opts, momo.WithImplicitTx())
	}
	testopen.Each(t, fn, opts...)
}

// rrBothModes runs fn with and without WithImplicitTx.
func rrBothModes(t *testing.T, fn func(t *testing.T, rec *rrRecorder, db *momo.DB, implicit bool)) {
	t.Helper()
	for _, implicit := range []bool{true, false} {
		name := "plain"
		if implicit {
			name = "implicit"
		}
		t.Run(name, func(t *testing.T) {
			rec := &rrRecorder{}
			rrOpenMode(t, rec, implicit, func(t *testing.T, db *momo.DB, _ dialect.Name) {
				rqSetup(t, db)
				fn(t, rec, db, implicit)
			})
		})
	}
}

// rrReleased proves the cursor let go of its connection and transaction: a
// write to the table the cursor read succeeds within a deadline (a leaked
// SQLite connection or an open PostgreSQL transaction on the pool would hang
// it; testopen also reports a leaked PostgreSQL connection at cleanup).
func rrReleased(t *testing.T, db *momo.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := momo.Exec(ctx, db, "UPDATE rq_items SET name = name"); err != nil {
		t.Fatalf("the connection was not released: %v", err)
	}
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
// hook pair ending without the error (with and without WithImplicitTx).
func TestRawRowsClosesOnCancel(t *testing.T) {
	rrBothModes(t, func(t *testing.T, rec *rrRecorder, db *momo.DB, implicit bool) {
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
		evs := rec.raw()
		if len(evs) != before+1 {
			t.Fatalf("cancel did not end the hook pair: %d events (before %d)", len(evs), before)
		}
		if evs[len(evs)-1].Err == nil {
			t.Fatal("the hook event of a canceled statement has a nil Err")
		}
		_ = rows.Close()
		_ = rows.Close() // safe twice
		if n := len(rec.raw()); n != before+1 {
			t.Fatalf("Close after cancel fired another hook event: %d (before %d)", n, before)
		}
		rrReleased(t, db)
	})
}

// Catches: an early Close leaking the connection or committing a half-read
// write, and the documented implicit-tx outcome (ErrValidation) changing.
func TestRawRowsEarlyClose(t *testing.T) {
	rrBothModes(t, func(t *testing.T, rec *rrRecorder, db *momo.DB, implicit bool) {
		const stmt = "SELECT id, name FROM rq_items ORDER BY id"
		ctx := context.Background()
		check := func(t *testing.T, err error) {
			t.Helper()
			if implicit && !errors.Is(err, momo.ErrValidation) {
				t.Fatalf("early Close under WithImplicitTx = %v, want ErrValidation", err)
			}
			if !implicit && err != nil {
				t.Fatalf("early Close = %v, want nil", err)
			}
		}

		// Close with no Next.
		before := len(rec.raw())
		rows, err := momo.RawRows(ctx, db, stmt)
		if err != nil {
			t.Fatal(err)
		}
		check(t, rows.Close())
		if err := rows.Close(); err != nil && !implicit {
			t.Fatalf("second Close = %v", err)
		}
		if n := len(rec.raw()); n != before+1 {
			t.Fatalf("Close without Next: %d hook events (before %d), want one", n, before)
		}
		rrReleased(t, db)

		// Close mid-iteration.
		rows, err = momo.RawRows(ctx, db, stmt)
		if err != nil {
			t.Fatal(err)
		}
		if !rows.Next() {
			t.Fatalf("Next false: %v", rows.Err())
		}
		check(t, rows.Close())
		rrReleased(t, db)

		// Read to the end: a clean Close.
		rows, err = momo.RawRows(ctx, db, stmt)
		if err != nil {
			t.Fatal(err)
		}
		if got := rrRead(t, rows); len(got) != 3 {
			t.Fatalf("rows = %v", got)
		}
		rrReleased(t, db)
	})
}

// Catches: a Scan error not reaching the hook event, or leaving the
// transaction open.
func TestRawRowsScanError(t *testing.T) {
	rrBothModes(t, func(t *testing.T, rec *rrRecorder, db *momo.DB, implicit bool) {
		before := len(rec.raw())
		rows, err := momo.RawRows(context.Background(), db, "SELECT id, name FROM rq_items ORDER BY id")
		if err != nil {
			t.Fatal(err)
		}
		if !rows.Next() {
			t.Fatalf("Next false: %v", rows.Err())
		}
		var id int64
		if err := rows.Scan(&id); err == nil { // two columns, one destination
			t.Fatal("Scan with the wrong destination count must fail")
		}
		cerr := rows.Close()
		if implicit && cerr == nil {
			t.Fatal("Close after a Scan error must report it under WithImplicitTx (the transaction rolled back)")
		}
		evs := rec.raw()
		if len(evs) != before+1 || evs[len(evs)-1].Err == nil {
			t.Fatalf("the Scan error is not on the hook event: %d events (before %d)", len(evs), before)
		}
		rrReleased(t, db)
	})
}

// Catches: a build or driver error from RawRows escaping classification.
func TestRawRowsErrors(t *testing.T) {
	rrBothModes(t, func(t *testing.T, rec *rrRecorder, db *momo.DB, implicit bool) {
		ctx := context.Background()
		_, err := momo.RawRows(ctx, db, "SELECT id FROM rq_items WHERE id = ?")
		if !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("argument-count mismatch = %v, want ErrValidation", err)
		}
		// A driver may report a server error at open or on the first Next.
		rows, err := momo.RawRows(ctx, db, "INSERT INTO rq_items (id, name) VALUES (1, 'dup') RETURNING id")
		if err == nil {
			if rows.Next() {
				t.Fatal("a duplicate key returned a row")
			}
			err = rows.Err()
			_ = rows.Close()
		}
		if !errors.Is(err, momo.ErrUniqueViolation) {
			t.Fatalf("duplicate key = %v, want ErrUniqueViolation", err)
		}
		evs := rec.raw()
		if len(evs) == 0 || evs[len(evs)-1].Err == nil {
			t.Fatal("the failed statement's hook event has a nil Err")
		}
		rrReleased(t, db)
	})
}
