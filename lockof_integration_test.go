// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// claimQuery is a job-claim shaped select: documents (alias d) joined to its
// tenant peers (alias p), locking only the d side.
func claimQuery(q momo.Querier, wait ...string) momo.SelectQuery[suite.Document] {
	d := momo.TableRef{Name: "documents", Alias: "d"}
	p := momo.TableRef{Name: "documents", Alias: "p"}
	return momo.Select[suite.Document](q).
		From(d).
		Join(p, suite.DocTenantID.As("d").EqExpr(suite.DocTenantID.As("p")), suite.DocID.As("d").NeExpr(suite.DocID.As("p"))).
		OrderBy(suite.DocTitle.As("d").Asc()).
		Limit(1).
		ForUpdate(wait...).
		LockOf("d")
}

// TestLockOfSkipLockedClaim is the two-connection job-claim scenario: the
// first transaction claims the first row through a join, the second skips it
// and claims the next, and a NOWAIT claim of the held row fails with ErrBusy.
func TestLockOfSkipLockedClaim(t *testing.T) {
	ctx := context.Background()
	db := testopen.Postgres(t)
	caps := suite.CapsOf(db)
	suite.CreateDocuments(t, db, caps)
	for _, title := range []string{"a", "b", "c"} {
		if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, ?)", title); err != nil {
			t.Fatal(err)
		}
	}

	first, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Rollback(ctx) }()
	got, err := claimQuery(first, "SKIP LOCKED").All(ctx)
	if err != nil || len(got) != 1 || got[0].Title != "a" {
		t.Fatalf("first claim = %+v, %v; want row a", got, err)
	}

	second, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Rollback(ctx) }()
	got, err = claimQuery(second, "SKIP LOCKED").All(ctx)
	if err != nil || len(got) != 1 || got[0].Title != "b" {
		t.Fatalf("second claim = %+v, %v; want row b (a is held)", got, err)
	}

	// A NOWAIT claim goes for the first row, which the first transaction holds.
	third, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = third.Rollback(ctx) }()
	if _, err := claimQuery(third, "NOWAIT").All(ctx); !errors.Is(err, momo.ErrBusy) {
		t.Fatalf("NOWAIT claim of the held first row: err = %v, want ErrBusy", err)
	}
}

func TestAdvisoryXactLockContention(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()

	holder, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback(ctx) }()
	if err := holder.AdvisoryXactLock(ctx, 9001); err != nil {
		t.Fatal(err)
	}
	if err := holder.AdvisoryXactLockString(ctx, "catalog:7"); err != nil {
		t.Fatal(err)
	}
	// The holder's own try succeeds again: advisory locks are re-entrant
	// within a session.
	if ok, err := holder.TryAdvisoryXactLock(ctx, 9001); err != nil || !ok {
		t.Fatalf("re-entrant try = %v, %v; want true", ok, err)
	}

	rival, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rival.Rollback(ctx) }()
	if ok, err := rival.TryAdvisoryXactLock(ctx, 9001); err != nil || ok {
		t.Fatalf("rival try on a held int key = %v, %v; want false", ok, err)
	}
	if ok, err := rival.TryAdvisoryXactLockString(ctx, "catalog:7"); err != nil || ok {
		t.Fatalf("rival try on a held string key = %v, %v; want false", ok, err)
	}
	if ok, err := rival.TryAdvisoryXactLockString(ctx, "catalog:8"); err != nil || !ok {
		t.Fatalf("rival try on a free string key = %v, %v; want true", ok, err)
	}

	// Commit releases the holder's locks.
	if err := holder.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if ok, err := rival.TryAdvisoryXactLock(ctx, 9001); err != nil || !ok {
		t.Fatalf("rival try after the holder committed = %v, %v; want true", ok, err)
	}

	// Outside a transaction the call is refused.
	if err := db.AdvisoryXactLock(ctx, 1); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("AdvisoryXactLock on the DB = %v, want ErrValidation", err)
	}
}
