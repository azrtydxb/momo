// SPDX-License-Identifier: Apache-2.0

//go:build integration

package advisory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/advisory"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// TestHash64MatchesHashTextExtended shows that the Hash64 option takes the same
// lock as application code that calls pg_advisory_xact_lock with
// hashtextextended(key, 0) directly, and a different one from the 32-bit
// default.
func TestHash64MatchesHashTextExtended(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()

	holder, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback(ctx) }()
	if _, err := momo.Exec(ctx, holder, "SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "orbit:claim"); err != nil {
		t.Fatal(err)
	}

	rival, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rival.Rollback(ctx) }()
	if ok, err := advisory.TryXactLockString(ctx, rival, "orbit:claim", advisory.Hash64(0)); err != nil || ok {
		t.Fatalf("Hash64(0) try on a key held with hashtextextended = %v, %v; want false", ok, err)
	}
	if ok, err := advisory.TryXactLockString(ctx, rival, "orbit:claim", advisory.Hash64(1)); err != nil || !ok {
		t.Fatalf("Hash64(1) try = %v, %v; want true (other seed, other key)", ok, err)
	}
	if ok, err := advisory.TryXactLockString(ctx, rival, "orbit:claim"); err != nil || !ok {
		t.Fatalf("32-bit default try = %v, %v; want true (hashtext key differs)", ok, err)
	}
}

// TestSessionLockOnPinnedConnections takes session locks on two pinned
// connections: the second cannot take a held key, the unlock frees it, and a
// pooled DB is refused.
func TestSessionLockOnPinnedConnections(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()

	pin := func() *momo.DB {
		t.Helper()
		pc, err := db.Driver().(momo.Pinner).Pin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = pc.Release() })
		return db.OnConn(pc)
	}
	a, b := pin(), pin()

	if err := advisory.Lock(ctx, db, 5); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("Lock on a pooled DB = %v, want ErrValidation", err)
	}

	if err := advisory.Lock(ctx, a, 9101); err != nil {
		t.Fatal(err)
	}
	if err := advisory.LockString(ctx, a, "migrate", advisory.Hash64(0)); err != nil {
		t.Fatal(err)
	}
	if ok, err := advisory.TryLock(ctx, b, 9101); err != nil || ok {
		t.Fatalf("TryLock on a held key = %v, %v; want false", ok, err)
	}
	if ok, err := advisory.TryLockString(ctx, b, "migrate", advisory.Hash64(0)); err != nil || ok {
		t.Fatalf("TryLockString on a held key = %v, %v; want false", ok, err)
	}
	if ok, err := advisory.TryLockString(ctx, b, "migrate"); err != nil || !ok {
		t.Fatalf("TryLockString with the 32-bit key = %v, %v; want true", ok, err)
	}
	if ok, err := advisory.UnlockString(ctx, b, "migrate"); err != nil || !ok {
		t.Fatalf("UnlockString of the 32-bit key = %v, %v; want true", ok, err)
	}

	// A connection cannot release a lock another session holds.
	if ok, err := advisory.Unlock(ctx, b, 9101); err != nil || ok {
		t.Fatalf("Unlock from the wrong session = %v, %v; want false", ok, err)
	}
	if ok, err := advisory.Unlock(ctx, a, 9101); err != nil || !ok {
		t.Fatalf("Unlock = %v, %v; want true", ok, err)
	}
	if ok, err := advisory.Unlock(ctx, a, 9101); err != nil || ok {
		t.Fatalf("second Unlock = %v, %v; want false (not held)", ok, err)
	}
	if ok, err := advisory.TryLock(ctx, b, 9101); err != nil || !ok {
		t.Fatalf("TryLock after the unlock = %v, %v; want true", ok, err)
	}
	if ok, err := advisory.UnlockString(ctx, a, "migrate", advisory.Hash64(0)); err != nil || !ok {
		t.Fatalf("UnlockString = %v, %v; want true", ok, err)
	}
	if _, err := advisory.Unlock(ctx, b, 9101); err != nil {
		t.Fatal(err)
	}
}
