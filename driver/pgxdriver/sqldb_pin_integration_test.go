// SPDX-License-Identifier: Apache-2.0

//go:build integration

package pgxdriver

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
)

// within fails the test when f does not return in time, instead of hanging the run.
func within(t *testing.T, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { f(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s did not return within 10s", what)
	}
}

func backendGone(t *testing.T, db *sql.DB, pid int) bool {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var n int
		if err := db.QueryRowContext(context.Background(), "SELECT count(*) FROM pg_stat_activity WHERE pid = $1", pid).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestFromSQLDBReleaseWithOpenTx(t *testing.T) {
	db, d := openSQLDriver(t)
	ctx := context.Background()
	p, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "SELECT 1", nil); err != nil {
		t.Fatal(err)
	}
	var relErr error
	within(t, "Release with an open tx", func() { relErr = p.Release() })
	if relErr != nil {
		t.Fatalf("Release: %v", relErr)
	}
	if err := tx.Commit(ctx); !errors.Is(err, momo.ErrTxDone) {
		t.Fatalf("Commit after Release = %v, want ErrTxDone", err)
	}
	if st := db.Stats(); st.InUse != 0 {
		t.Fatalf("InUse = %d after Release, want 0", st.InUse)
	}
}

func TestFromSQLDBDiscardWithOpenTx(t *testing.T) {
	db, d := openSQLDriver(t)
	ctx := context.Background()
	p, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var pid int
	scanOne(t, p, "SELECT pg_backend_pid()", &pid)
	if _, err := p.Begin(ctx, momo.TxOptions{}); err != nil {
		t.Fatal(err)
	}
	var disErr error
	within(t, "Discard with an open tx", func() { disErr = p.(momo.Discarder).Discard() })
	if disErr != nil {
		t.Fatalf("Discard: %v", disErr)
	}
	if !backendGone(t, db, pid) {
		t.Fatalf("backend %d still alive after Discard", pid)
	}
}

// Two Begins on one pinned connection, both left open: Release and Discard must roll back
// every open transaction instead of hanging on the first one's close lock.
func TestFromSQLDBTwoOpenTxs(t *testing.T) {
	for _, mode := range []string{"Release", "Discard"} {
		t.Run(mode, func(t *testing.T) {
			db, d := openSQLDriver(t)
			ctx := context.Background()
			p, err := d.(momo.Pinner).Pin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var pid int
			scanOne(t, p, "SELECT pg_backend_pid()", &pid)
			for range 2 {
				if _, err := p.Begin(ctx, momo.TxOptions{}); err != nil {
					t.Fatal(err)
				}
			}
			var endErr error
			within(t, mode+" with two open txs", func() {
				if mode == "Release" {
					endErr = p.Release()
				} else {
					endErr = p.(momo.Discarder).Discard()
				}
			})
			if endErr != nil {
				t.Fatalf("%s: %v", mode, endErr)
			}
			if mode == "Discard" && !backendGone(t, db, pid) {
				t.Fatalf("backend %d still alive after Discard", pid)
			}
			if st := db.Stats(); st.InUse != 0 {
				t.Fatalf("InUse = %d, want 0", st.InUse)
			}
		})
	}
}

func TestFromSQLDBPinnedBeginAndFinishedTx(t *testing.T) {
	_, d := openSQLDriver(t)
	ctx := context.Background()
	p, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(ctx, momo.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "CREATE TEMP TABLE pin_ro (i int)", nil); err == nil {
		t.Fatal("write in a read-only tx succeeded")
	}
	_ = tx.Rollback(ctx)
	tx, err = p.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := p.Release(); err != nil {
		t.Fatalf("Release after finished tx: %v", err)
	}
}

func TestFromSQLDBTxSurvivesBeginContext(t *testing.T) {
	_, d := openSQLDriver(t)
	bctx, cancel := context.WithCancel(context.Background())
	tx, err := d.Begin(bctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	time.Sleep(100 * time.Millisecond)
	if _, err := tx.Exec(context.Background(), "SELECT 1", nil); err != nil {
		t.Fatalf("tx died with the Begin context: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	cctx, ccancel := context.WithCancel(context.Background())
	ccancel()
	if _, err := d.Begin(cctx, momo.TxOptions{}); err == nil {
		t.Fatal("Begin with a cancelled context succeeded")
	}
}

func TestFromSQLDBBeginRejectsIsolation(t *testing.T) {
	_, d := openSQLDriver(t)
	_, err := d.Begin(context.Background(), momo.TxOptions{Isolation: sql.LevelSnapshot})
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("Begin(LevelSnapshot) = %v, want ErrUnsupported", err)
	}
}

func TestFromSQLDBPinnedBusyAndDoubleDiscard(t *testing.T) {
	_, d := openSQLDriver(t)
	ctx := context.Background()
	p, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sp := p.(*sqlPinned)
	execDone := make(chan error, 1)
	go func() {
		_, err := p.Exec(ctx, "SELECT pg_sleep(1.5)", nil)
		execDone <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		sp.mu.Lock()
		n := sp.calls
		sp.mu.Unlock()
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Exec never started")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := p.Release(); !errors.Is(err, momo.ErrPinnedBusy) {
		t.Fatalf("Release during Exec = %v, want ErrPinnedBusy", err)
	}
	if err := p.(momo.Discarder).Discard(); !errors.Is(err, momo.ErrPinnedBusy) {
		t.Fatalf("Discard during Exec = %v, want ErrPinnedBusy", err)
	}
	if err := <-execDone; err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if err := p.(momo.Discarder).Discard(); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if err := p.(momo.Discarder).Discard(); err != nil {
		t.Fatalf("second Discard: %v", err)
	}
	if err := p.Release(); err != nil {
		t.Fatalf("Release after Discard: %v", err)
	}
}

func TestFromSQLDBVectorRoundTrip(t *testing.T) {
	_, d := openSQLDriver(t)
	ctx := context.Background()
	if _, err := d.Exec(ctx, "CREATE TABLE sqldb_vec (id int, v vector(3), h halfvec(3), s sparsevec(5), b bit(9))", nil); err != nil {
		t.Fatal(err)
	}
	in := codecInput()
	if _, err := d.Exec(ctx, "INSERT INTO sqldb_vec VALUES (1, $1, $2, $3, $4)", []any{in.v, in.h, in.s, in.b}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	rows, err := d.Query(ctx, "SELECT v, h, s, b FROM sqldb_vec WHERE id = 1", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatalf("no row: %v", rows.Err())
	}
	var out codecRow
	if err := rows.Scan(&out.v, &out.h, &out.s, &out.b); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip = %+v, want %+v", out, in)
	}
}
