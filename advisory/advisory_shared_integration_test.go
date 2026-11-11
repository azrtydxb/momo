// SPDX-License-Identifier: Apache-2.0

//go:build integration

package advisory_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/advisory"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// Catches: a shared lock that excludes other shared holders, one that does not
// exclude the exclusive lock, and a Hash64(52) key that differs from
// hashtextextended(key, 52) used directly (the Owl reader/writer pair).
func TestXactLockSharedReaderWriter(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()

	begin := func() *momo.Tx {
		t.Helper()
		tx, err := db.Begin(ctx, momo.TxOptions{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = tx.Rollback(ctx) })
		return tx
	}
	reader1, reader2, writer, raw := begin(), begin(), begin(), begin()

	if err := advisory.XactLockSharedString(ctx, reader1, "obj:ledger", advisory.Hash64(52)); err != nil {
		t.Fatal(err)
	}
	if ok, err := advisory.TryXactLockSharedString(ctx, reader2, "obj:ledger", advisory.Hash64(52)); err != nil || !ok {
		t.Fatalf("second shared try = %v, %v; want true", ok, err)
	}
	if ok, err := advisory.TryXactLockString(ctx, writer, "obj:ledger", advisory.Hash64(52)); err != nil || ok {
		t.Fatalf("exclusive try while shared held = %v, %v; want false", ok, err)
	}
	// The same key spelled in SQL is the same lock.
	got, err := momo.Scalars[bool](ctx, raw, "SELECT pg_try_advisory_xact_lock(hashtextextended(?::text, 52))", "obj:ledger")
	if err != nil || len(got) != 1 || got[0] {
		t.Fatalf("raw exclusive try on hashtextextended(key, 52) = %v, %v; want false", got, err)
	}
}

// Catches: an exclusive holder that does not block shared takers, and a
// commit that does not release the lock.
func TestXactLockSharedBlockedByExclusive(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()

	writer, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Rollback(ctx) }()
	if err := advisory.XactLock(ctx, writer, 9301); err != nil {
		t.Fatal(err)
	}
	reader, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Rollback(ctx) }()
	if ok, err := advisory.TryXactLockShared(ctx, reader, 9301); err != nil || ok {
		t.Fatalf("shared try while exclusive held = %v, %v; want false", ok, err)
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if ok, err := advisory.TryXactLockShared(ctx, reader, 9301); err != nil || !ok {
		t.Fatalf("shared try after the exclusive commit = %v, %v; want true", ok, err)
	}
	if err := advisory.XactLockShared(ctx, reader, 9301); err != nil {
		t.Fatal(err)
	}
}

// Catches: session shared locks that exclude each other, that do not exclude
// the session exclusive lock, and an UnlockShared that releases the wrong
// lock.
func TestSessionLockShared(t *testing.T) {
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
	a, b, c := pin(), pin(), pin()

	if err := advisory.LockShared(ctx, a, 9302); err != nil {
		t.Fatal(err)
	}
	if ok, err := advisory.TryLockShared(ctx, b, 9302); err != nil || !ok {
		t.Fatalf("second session shared try = %v, %v; want true", ok, err)
	}
	if ok, err := advisory.TryLock(ctx, c, 9302); err != nil || ok {
		t.Fatalf("exclusive try while shared held = %v, %v; want false", ok, err)
	}
	if ok, err := advisory.Unlock(ctx, a, 9302); err != nil || ok {
		t.Fatalf("Unlock of a shared lock = %v, %v; want false", ok, err)
	}
	if ok, err := advisory.UnlockShared(ctx, a, 9302); err != nil || !ok {
		t.Fatalf("UnlockShared = %v, %v; want true", ok, err)
	}
	if ok, err := advisory.TryLock(ctx, c, 9302); err != nil || ok {
		t.Fatalf("exclusive try with one shared holder left = %v, %v; want false", ok, err)
	}
	if ok, err := advisory.UnlockShared(ctx, b, 9302); err != nil || !ok {
		t.Fatalf("UnlockShared on b = %v, %v; want true", ok, err)
	}
	if ok, err := advisory.TryLock(ctx, c, 9302); err != nil || !ok {
		t.Fatalf("exclusive try after all unlocks = %v, %v; want true", ok, err)
	}
	if ok, err := advisory.Unlock(ctx, c, 9302); err != nil || !ok {
		t.Fatalf("Unlock = %v, %v; want true", ok, err)
	}

	if err := advisory.LockSharedString(ctx, a, "k", advisory.Hash64(52)); err != nil {
		t.Fatal(err)
	}
	if ok, err := advisory.TryLockSharedString(ctx, b, "k", advisory.Hash64(52)); err != nil || !ok {
		t.Fatalf("TryLockSharedString = %v, %v; want true", ok, err)
	}
	if ok, err := advisory.UnlockSharedString(ctx, b, "k", advisory.Hash64(52)); err != nil || !ok {
		t.Fatalf("UnlockSharedString on b = %v, %v; want true", ok, err)
	}
	if ok, err := advisory.UnlockSharedString(ctx, a, "k", advisory.Hash64(52)); err != nil || !ok {
		t.Fatalf("UnlockSharedString on a = %v, %v; want true", ok, err)
	}
}
