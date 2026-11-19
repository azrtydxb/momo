// SPDX-License-Identifier: Apache-2.0

//go:build integration

//lint:file-ignore SA1019 these tests keep the deprecated functions covered

package advisory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/advisory"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// pinOn pins one connection of db for the test and returns the handle that
// runs every statement on it.
func pinOn(t *testing.T, db *momo.DB) *momo.DB {
	t.Helper()
	pc, err := db.Driver().(momo.Pinner).Pin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Release() })
	return db.OnConn(pc)
}

// TestMySQLSessionLockContention runs on MySQL 8.4, MySQL 9.7 and MariaDB 11.8:
// two pinned connections contend for one key, a try-lock fails while the lock
// is held, and the release frees it.
func TestMySQLSessionLockContention(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		a, b := pinOn(t, db), pinOn(t, db)

		if err := advisory.Lock(ctx, db, 7101); !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("Lock on a pooled DB = %v, want ErrValidation", err)
		}

		if err := advisory.Lock(ctx, a, 7101); err != nil {
			t.Fatal(err)
		}
		if err := advisory.LockString(ctx, a, "orbit:claim"); err != nil {
			t.Fatal(err)
		}
		if ok, err := advisory.TryLock(ctx, b, 7101); err != nil || ok {
			t.Fatalf("TryLock on a held key = %v, %v; want false", ok, err)
		}
		if ok, err := advisory.TryLockString(ctx, b, "orbit:claim"); err != nil || ok {
			t.Fatalf("TryLockString on a held key = %v, %v; want false", ok, err)
		}
		if ok, err := advisory.TryLock(ctx, b, 7102); err != nil || !ok {
			t.Fatalf("TryLock on another key = %v, %v; want true", ok, err)
		}

		// A connection cannot release a lock another session holds.
		if ok, err := advisory.Unlock(ctx, b, 7101); err != nil || ok {
			t.Fatalf("Unlock from the wrong session = %v, %v; want false", ok, err)
		}
		if ok, err := advisory.TryLock(ctx, b, 7101); err != nil || ok {
			t.Fatalf("TryLock after a refused foreign unlock = %v, %v; want false", ok, err)
		}

		if ok, err := advisory.Unlock(ctx, a, 7101); err != nil || !ok {
			t.Fatalf("Unlock = %v, %v; want true", ok, err)
		}
		if ok, err := advisory.Unlock(ctx, a, 7101); err != nil || ok {
			t.Fatalf("second Unlock = %v, %v; want false (not held)", ok, err)
		}
		if ok, err := advisory.TryLock(ctx, b, 7101); err != nil || !ok {
			t.Fatalf("TryLock after the release = %v, %v; want true", ok, err)
		}
		if ok, err := advisory.UnlockString(ctx, a, "orbit:claim"); err != nil || !ok {
			t.Fatalf("UnlockString = %v, %v; want true", ok, err)
		}
		if ok, err := advisory.TryLockString(ctx, b, "orbit:claim"); err != nil || !ok {
			t.Fatalf("TryLockString after the release = %v, %v; want true", ok, err)
		}
		for _, k := range []int64{7101, 7102} {
			if ok, err := advisory.Unlock(ctx, b, k); err != nil || !ok {
				t.Fatalf("Unlock(%d) = %v, %v; want true", k, ok, err)
			}
		}
		if ok, err := advisory.UnlockString(ctx, b, "orbit:claim"); err != nil || !ok {
			t.Fatalf("UnlockString = %v, %v; want true", ok, err)
		}
	})
}

// TestMySQLSessionLockIsCounted checks that a key locked twice on one
// connection needs two unlocks, as on PostgreSQL.
func TestMySQLSessionLockIsCounted(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		a, b := pinOn(t, db), pinOn(t, db)
		for range 2 {
			if err := advisory.Lock(ctx, a, 7201); err != nil {
				t.Fatal(err)
			}
		}
		if ok, err := advisory.Unlock(ctx, a, 7201); err != nil || !ok {
			t.Fatalf("first Unlock = %v, %v; want true", ok, err)
		}
		if ok, err := advisory.TryLock(ctx, b, 7201); err != nil || ok {
			t.Fatalf("TryLock with one hold left = %v, %v; want false", ok, err)
		}
		if ok, err := advisory.Unlock(ctx, a, 7201); err != nil || !ok {
			t.Fatalf("second Unlock = %v, %v; want true", ok, err)
		}
		if ok, err := advisory.TryLock(ctx, b, 7201); err != nil || !ok {
			t.Fatalf("TryLock after both unlocks = %v, %v; want true", ok, err)
		}
	})
}

// TestMySQLBlockingLockWaitsForRelease checks that Lock waits for a held key
// and returns once the holder unlocks, and that a canceled wait returns the
// context error.
func TestMySQLBlockingLockWaitsForRelease(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		a, b := pinOn(t, db), pinOn(t, db)
		if err := advisory.Lock(ctx, a, 7301); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- advisory.Lock(ctx, b, 7301) }()
		select {
		case err := <-done:
			t.Fatalf("Lock on a held key returned %v before the release", err)
		case <-time.After(1500 * time.Millisecond):
		}
		if ok, err := advisory.Unlock(ctx, a, 7301); err != nil || !ok {
			t.Fatalf("Unlock = %v, %v; want true", ok, err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Lock after the release = %v", err)
			}
		case <-time.After(15 * time.Second):
			t.Fatal("Lock did not return after the holder unlocked")
		}
		if ok, err := advisory.Unlock(ctx, b, 7301); err != nil || !ok {
			t.Fatalf("Unlock = %v, %v; want true", ok, err)
		}
	})
}

// TestMySQLRefusalsOnServer checks the refusals against a real
// server: nothing is locked.
func TestMySQLRefusalsOnServer(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if err := advisory.XactLock(ctx, tx, 7401); !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("XactLock = %v, want ErrUnsupported", err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		a, b := pinOn(t, db), pinOn(t, db)
		if err := advisory.LockShared(ctx, a, 7401); !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("LockShared = %v, want ErrUnsupported", err)
		}
		if ok, err := advisory.TryLock(ctx, b, 7401); err != nil || !ok {
			t.Errorf("TryLock after refused locks = %v, %v; want true (nothing was taken)", ok, err)
		}
	})
}

// TestMySQLLockIsScopedToTheDatabase checks that, as on PostgreSQL, one key in
// two databases of the same server is two locks: GET_LOCK names are global to
// the server, so the name carries the current database.
func TestMySQLLockIsScopedToTheDatabase(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		const other = "momo_advisory_scope"
		if _, err := momo.Exec(ctx, db, "CREATE DATABASE IF NOT EXISTS "+other); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = momo.Exec(context.Background(), db, "DROP DATABASE IF EXISTS "+other) })

		a, b, c := pinOn(t, db), pinOn(t, db), pinOn(t, db)
		if _, err := momo.Exec(ctx, b, "USE "+other); err != nil {
			t.Fatal(err)
		}
		if err := advisory.Lock(ctx, a, 7501); err != nil {
			t.Fatal(err)
		}
		if ok, err := advisory.TryLock(ctx, b, 7501); err != nil || !ok {
			t.Fatalf("TryLock of the same key in another database = %v, %v; want true", ok, err)
		}
		if ok, err := advisory.TryLock(ctx, c, 7501); err != nil || ok {
			t.Fatalf("TryLock of the same key in the same database = %v, %v; want false", ok, err)
		}
		if ok, err := advisory.TryLockString(ctx, b, "scope"); err != nil || !ok {
			t.Fatalf("TryLockString in the other database = %v, %v; want true", ok, err)
		}
		if ok, err := advisory.Unlock(ctx, a, 7501); err != nil || !ok {
			t.Fatalf("Unlock = %v, %v; want true", ok, err)
		}
		if ok, err := advisory.Unlock(ctx, b, 7501); err != nil || !ok {
			t.Fatalf("Unlock in the other database = %v, %v; want true", ok, err)
		}
		if ok, err := advisory.UnlockString(ctx, b, "scope"); err != nil || !ok {
			t.Fatalf("UnlockString in the other database = %v, %v; want true", ok, err)
		}
	})
}

// TestMySQLLockDeadlineReturnsPromptly checks that a Lock whose context ends
// while another session holds the key returns a context error promptly, and
// that once its connection is discarded and the holder unlocks, a fresh
// connection takes the key.
func TestMySQLLockDeadlineReturnsPromptly(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		ctx := context.Background()
		a := pinOn(t, db)
		if err := advisory.Lock(ctx, a, 7601); err != nil {
			t.Fatal(err)
		}

		pc, err := db.Driver().(momo.Pinner).Pin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		b := db.OnConn(pc)
		dctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		defer cancel()
		start := time.Now()
		err = advisory.Lock(dctx, b, 7601)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Lock past its deadline = %v, want a context.DeadlineExceeded error", err)
		}
		if d := time.Since(start); d > 3*time.Second {
			t.Fatalf("Lock returned after %v, want about its 300ms deadline", d)
		}
		d, ok := pc.(momo.Discarder)
		if !ok {
			t.Fatal("the pinned connection cannot be discarded")
		}
		if err := d.Discard(); err != nil {
			t.Logf("Discard of the canceled connection: %v", err)
		}

		if ok, err := advisory.Unlock(ctx, a, 7601); err != nil || !ok {
			t.Fatalf("Unlock = %v, %v; want true", ok, err)
		}
		c := pinOn(t, db)
		// The server may need a moment to drop the abandoned session.
		deadline := time.Now().Add(10 * time.Second)
		for {
			ok, err := advisory.TryLock(ctx, c, 7601)
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("a fresh connection could not take the key after the holder unlocked")
			}
			time.Sleep(200 * time.Millisecond)
		}
		if ok, err := advisory.Unlock(ctx, c, 7601); err != nil || !ok {
			t.Fatalf("Unlock = %v, %v; want true", ok, err)
		}
	})
}
