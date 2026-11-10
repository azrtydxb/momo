// SPDX-License-Identifier: Apache-2.0

//go:build integration

package timeop_test

import (
	"context"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/timeop"
)

// timeIn reads one timestamp expression through q.
func timeIn(t *testing.T, q momo.Querier, e momo.Expr) time.Time {
	t.Helper()
	r, err := momo.Select[tmTime](q).From(momo.TableRef{Name: "documents"}).Columns(e).
		Where(suite.DocTenantID.Eq(1)).One(context.Background())
	if err != nil {
		t.Fatalf("select time: %v", err)
	}
	return r.T
}

// TestClockTimestampAdvancesInTransaction shows the difference the Orbit lease
// code relies on: inside one transaction Now stays at the transaction start,
// StatementTimestamp moves with each statement, ClockTimestamp moves with
// every call.
func TestClockTimestampAdvancesInTransaction(t *testing.T) {
	ctx := context.Background()
	db := testopen.Postgres(t)
	seedTimeDocs(t, db)

	tx, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now1 := timeIn(t, tx, timeop.Now())
	stmt1 := timeIn(t, tx, timeop.StatementTimestamp())
	clock1 := timeIn(t, tx, timeop.ClockTimestamp())
	if _, err := momo.Exec(ctx, tx, "SELECT pg_sleep(0.05)"); err != nil {
		t.Fatal(err)
	}
	now2 := timeIn(t, tx, timeop.Now())
	stmt2 := timeIn(t, tx, timeop.StatementTimestamp())
	clock2 := timeIn(t, tx, timeop.ClockTimestamp())

	if !now1.Equal(now2) {
		t.Errorf("Now moved inside a transaction: %v then %v", now1, now2)
	}
	if !stmt2.After(stmt1) {
		t.Errorf("StatementTimestamp did not advance: %v then %v", stmt1, stmt2)
	}
	if !stmt2.After(now2) {
		t.Errorf("StatementTimestamp %v is not after the transaction start %v", stmt2, now2)
	}
	if !clock2.After(clock1) || clock2.Sub(clock1) < 40*time.Millisecond {
		t.Errorf("ClockTimestamp advanced %v, want at least the 50ms sleep", clock2.Sub(clock1))
	}
	if d := time.Since(clock2); d < -time.Minute || d > time.Minute {
		t.Errorf("ClockTimestamp = %v, far from the wall clock", clock2)
	}
}
