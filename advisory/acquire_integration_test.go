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

// Catches: Shared, Try and Xact that are accepted and ignored on a real
// server, and a string key whose WithHash64 form differs from
// hashtextextended(key, 52) used directly.
func TestAcquireTransactionLocks(t *testing.T) {
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
	reader1, reader2, writer := begin(), begin(), begin()

	if ok, err := advisory.Acquire(ctx, reader1, "obj:acquire", advisory.Xact(), advisory.Shared(), advisory.WithHash64(52)); err != nil || !ok {
		t.Fatalf("shared acquire = %v, %v; want true", ok, err)
	}
	if ok, err := advisory.Acquire(ctx, reader2, "obj:acquire", advisory.Xact(), advisory.Shared(), advisory.Try(), advisory.WithHash64(52)); err != nil || !ok {
		t.Fatalf("second shared try = %v, %v; want true", ok, err)
	}
	if ok, err := advisory.Acquire(ctx, writer, "obj:acquire", advisory.Xact(), advisory.Try(), advisory.WithHash64(52)); err != nil || ok {
		t.Fatalf("exclusive try while shared held = %v, %v; want false", ok, err)
	}
	got, err := momo.Scalars[bool](ctx, writer, "SELECT pg_try_advisory_xact_lock(hashtextextended(?::text, 52))", "obj:acquire")
	if err != nil || len(got) != 1 || got[0] {
		t.Fatalf("raw exclusive try on hashtextextended(key, 52) = %v, %v; want false", got, err)
	}
}

// Catches: a Release that targets the wrong lock mode, and a Try that waits
// instead of reporting.
func TestAcquireAndReleaseSessionLocks(t *testing.T) {
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

	if ok, err := advisory.Acquire(ctx, a, 9401, advisory.Shared()); err != nil || !ok {
		t.Fatalf("shared acquire = %v, %v; want true", ok, err)
	}
	if ok, err := advisory.Acquire(ctx, b, 9401, advisory.Try()); err != nil || ok {
		t.Fatalf("exclusive try while shared held = %v, %v; want false", ok, err)
	}
	if ok, err := advisory.Release(ctx, a, 9401); err != nil || ok {
		t.Fatalf("Release without Shared of a shared lock = %v, %v; want false", ok, err)
	}
	if ok, err := advisory.Release(ctx, a, 9401, advisory.Shared()); err != nil || !ok {
		t.Fatalf("Release with Shared = %v, %v; want true", ok, err)
	}
	if ok, err := advisory.Acquire(ctx, b, "acquire:k", advisory.Try()); err != nil || !ok {
		t.Fatalf("exclusive string try = %v, %v; want true", ok, err)
	}
	if ok, err := advisory.Acquire(ctx, a, "acquire:k", advisory.Try()); err != nil || ok {
		t.Fatalf("second session try on a held string key = %v, %v; want false", ok, err)
	}
	if ok, err := advisory.Release(ctx, b, "acquire:k"); err != nil || !ok {
		t.Fatalf("Release of the string key = %v, %v; want true", ok, err)
	}
}
