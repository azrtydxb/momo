// SPDX-License-Identifier: Apache-2.0

//go:build integration

package advisory_test

import (
	"context"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/advisory"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestXactLockContention(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()

	holder, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback(ctx) }()
	if err := advisory.XactLock(ctx, holder, 9001); err != nil {
		t.Fatal(err)
	}
	if err := advisory.XactLockString(ctx, holder, "catalog:7"); err != nil {
		t.Fatal(err)
	}
	// The holder's own try succeeds again: advisory locks are re-entrant
	// within a session.
	if ok, err := advisory.TryXactLock(ctx, holder, 9001); err != nil || !ok {
		t.Fatalf("re-entrant try = %v, %v; want true", ok, err)
	}

	rival, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rival.Rollback(ctx) }()
	if ok, err := advisory.TryXactLock(ctx, rival, 9001); err != nil || ok {
		t.Fatalf("rival try on a held int key = %v, %v; want false", ok, err)
	}
	if ok, err := advisory.TryXactLockString(ctx, rival, "catalog:7"); err != nil || ok {
		t.Fatalf("rival try on a held string key = %v, %v; want false", ok, err)
	}
	if ok, err := advisory.TryXactLockString(ctx, rival, "catalog:8"); err != nil || !ok {
		t.Fatalf("rival try on a free string key = %v, %v; want true", ok, err)
	}

	// Commit releases the holder's locks.
	if err := holder.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if ok, err := advisory.TryXactLock(ctx, rival, 9001); err != nil || !ok {
		t.Fatalf("rival try after the holder committed = %v, %v; want true", ok, err)
	}
}

// TestXactLockBlocksUntilCommit shows the blocking form: a second connection
// waits in XactLock until the first transaction commits.
func TestXactLockBlocksUntilCommit(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()

	holder, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback(ctx) }()
	if err := advisory.XactLock(ctx, holder, 9002); err != nil {
		t.Fatal(err)
	}

	rival, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rival.Rollback(ctx) }()

	done := make(chan error, 1)
	go func() { done <- advisory.XactLock(ctx, rival, 9002) }()

	select {
	case err := <-done:
		t.Fatalf("rival XactLock returned %v while the holder still held the lock", err)
	case <-time.After(500 * time.Millisecond):
	}

	if err := holder.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("rival XactLock after the commit = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("rival XactLock still blocked after the holder committed")
	}
}
