// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/jackc/pgx/v5/pgconn"
)

func createJobsLock(t *testing.T, db *momo.DB, extra ...string) {
	t.Helper()
	ctx := context.Background()
	stmts := append([]string{
		"DROP TABLE IF EXISTS jobs_lock",
		"CREATE TABLE jobs_lock (name text primary key, holder text not null, until timestamptz not null, live boolean not null default true)",
	}, extra...)
	for _, s := range stmts {
		if _, err := db.Driver().Exec(ctx, s, nil); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

// take is the lease statement: insert the row, or update it only when the
// current lease has expired or is already ours.
func take(ctx context.Context, db *momo.DB, holder string, until, now time.Time) (int64, error) {
	r, err := momo.Insert[Lease](db).
		Values(leaseName.Set("nightly"), leaseHolder.Set(holder), leaseUntil.Set(until)).
		OnConflict(leaseName).
		DoUpdate(leaseHolder.SetExcluded(), leaseUntil.SetExcluded()).
		Where(momo.Or(leaseUntil.Le(now), leaseHolder.Eq(holder))).
		Exec(ctx)
	return r.RowsAffected, err
}

func holderOf(t *testing.T, ctx context.Context, db *momo.DB) (string, time.Time) {
	t.Helper()
	l, err := momo.Select[Lease](db).Where(leaseName.Eq("nightly")).One(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return l.Holder, l.Until
}

// Catches: a dropped or mis-qualified DO UPDATE WHERE (B would steal a live
// lease), and a Where that blocks the legitimate renew or takeover.
func TestConflictWhereLease(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		createJobsLock(t, db)
		ctx := context.Background()
		t0 := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

		n, err := take(ctx, db, "A", t0.Add(10*time.Minute), t0)
		if err != nil || n != 1 {
			t.Fatalf("A takes: rows=%d err=%v, want 1", n, err)
		}
		// B is refused while A's lease is live.
		n, err = take(ctx, db, "B", t0.Add(20*time.Minute), t0.Add(time.Minute))
		if err != nil || n != 0 {
			t.Fatalf("B while live: rows=%d err=%v, want 0", n, err)
		}
		if h, u := holderOf(t, ctx, db); h != "A" || !u.Equal(t0.Add(10*time.Minute)) {
			t.Fatalf("row changed by refused take: %s %v", h, u)
		}
		// A renews its own lease.
		n, err = take(ctx, db, "A", t0.Add(30*time.Minute), t0.Add(2*time.Minute))
		if err != nil || n != 1 {
			t.Fatalf("A renews: rows=%d err=%v, want 1", n, err)
		}
		if _, u := holderOf(t, ctx, db); !u.Equal(t0.Add(30 * time.Minute)) {
			t.Fatalf("renew did not extend: %v", u)
		}
		// B takes it after expiry.
		n, err = take(ctx, db, "B", t0.Add(90*time.Minute), t0.Add(time.Hour))
		if err != nil || n != 1 {
			t.Fatalf("B after expiry: rows=%d err=%v, want 1", n, err)
		}
		if h, _ := holderOf(t, ctx, db); h != "B" {
			t.Fatalf("holder = %s, want B", h)
		}
	})
}

// Catches: Excluded rendering something other than the proposed row, or the
// right-hand side being bound instead of referencing EXCLUDED.
func TestConflictWhereExcludedComparison(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		createJobsLock(t, db)
		ctx := context.Background()
		t0 := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
		if _, err := take(ctx, db, "A", t0.Add(time.Hour), t0); err != nil {
			t.Fatal(err)
		}
		bump := func(until time.Time) int64 {
			r, err := momo.Insert[Lease](db).
				Values(leaseName.Set("nightly"), leaseHolder.Set("A"), leaseUntil.Set(until)).
				OnConflict(leaseName).
				DoUpdate(leaseUntil.SetExcluded()).
				Where(leaseUntil.LtExpr(leaseUntil.Excluded())).
				Exec(ctx)
			if err != nil {
				t.Fatal(err)
			}
			return r.RowsAffected
		}
		if n := bump(t0.Add(30 * time.Minute)); n != 0 {
			t.Fatalf("earlier proposed until: rows=%d, want 0", n)
		}
		if n := bump(t0.Add(2 * time.Hour)); n != 1 {
			t.Fatalf("later proposed until: rows=%d, want 1", n)
		}
	})
}

// Catches: the conflict-target WHERE being dropped (PostgreSQL then cannot
// infer the partial unique index and rejects the statement), and a target
// predicate that is qualified or bound wrongly.
func TestConflictWherePartialIndexTarget(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d == dialect.SQLite {
			// SQLite matches a conflict-target WHERE against the index predicate
			// textually, so a bound parameter never matches. Open story
			// 20261106-conditional-upsert-follow-ups-sqlite-target-literals.
			t.Skip("SQLite conflict-target predicates must be literals")
		}
		createJobsLock(t, db, "CREATE UNIQUE INDEX jobs_lock_live_holder ON jobs_lock (holder) WHERE live")
		ctx := context.Background()
		t0 := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
		// jobs_lock.name is the primary key; the partial index covers holder.
		ins := func(name string) momo.ConflictQuery[Lease] {
			return momo.Insert[Lease](db).
				Values(leaseName.Set(name), leaseHolder.Set("H"), leaseUntil.Set(t0), leaseLive.Set(true)).
				OnConflict(leaseHolder)
		}
		if _, err := ins("one").Where(leaseLive.Eq(true)).DoNothing().Exec(ctx); err != nil {
			t.Fatal(err)
		}
		// A second live row for the same holder conflicts on the partial index.
		later := t0.Add(time.Hour)
		r, err := momo.Insert[Lease](db).
			Values(leaseName.Set("two"), leaseHolder.Set("H"), leaseUntil.Set(later), leaseLive.Set(true)).
			OnConflict(leaseHolder).Where(leaseLive.Eq(true)).
			DoUpdate(leaseUntil.SetExcluded()).Exec(ctx)
		if err != nil || r.RowsAffected != 1 {
			t.Fatalf("partial target upsert: rows=%d err=%v", r.RowsAffected, err)
		}
		// The conflict updated row "one" in place; no row "two" was inserted.
		if n, err := momo.Select[Lease](db).Count(ctx); err != nil || n != 1 {
			t.Fatalf("rows after partial target upsert: n=%d err=%v, want 1", n, err)
		}
		one, err := momo.Select[Lease](db).Where(leaseName.Eq("one")).One(ctx)
		if err != nil || !one.Until.Equal(later) {
			t.Fatalf("row one until=%v err=%v, want %v", one.Until, err, later)
		}
		// Without the target WHERE PostgreSQL cannot infer the partial index.
		_, err = ins("three").DoNothing().Exec(ctx)
		if err == nil {
			t.Fatal("expected PostgreSQL to reject the target without its WHERE")
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42P10" {
			t.Fatalf("want SQLSTATE 42P10 (no matching unique index) from the database, got %v", err)
		}
	})
}

// Catches: RETURNING yielding a row when the DO UPDATE condition is false,
// and a true condition returning nothing.
func TestConflictWhereReturning(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		createJobsLock(t, db)
		ctx := context.Background()
		t0 := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
		if _, err := take(ctx, db, "A", t0.Add(time.Hour), t0); err != nil {
			t.Fatal(err)
		}
		count := func(holder string, now time.Time) int {
			rows, err := momo.Insert[Lease](db).
				Values(leaseName.Set("nightly"), leaseHolder.Set(holder), leaseUntil.Set(t0.Add(3*time.Hour))).
				OnConflict(leaseName).
				DoUpdate(leaseHolder.SetExcluded(), leaseUntil.SetExcluded()).
				Where(momo.Or(leaseUntil.Le(now), leaseHolder.Eq(holder))).
				Returning(leaseHolder).
				Rows(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			n := 0
			for rows.Next() {
				n++
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			return n
		}
		if n := count("B", t0.Add(time.Minute)); n != 0 {
			t.Fatalf("false condition returned %d rows, want 0", n)
		}
		if n := count("A", t0.Add(time.Minute)); n != 1 {
			t.Fatalf("true condition returned %d rows, want 1", n)
		}
	})
}

// Catches: Where on Upsert[T] being ignored or not combining with the
// resolved default update set.
func TestUpsertWhere(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		createJobsLock(t, db)
		ctx := context.Background()
		t0 := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
		up := func(holder string, until time.Time) int64 {
			r, err := momo.Upsert[Lease](db).
				Model(&Lease{Name: "nightly", Holder: holder, Until: until, Live: true}).
				Where(leaseUntil.LtExpr(leaseUntil.Excluded())).
				Exec(ctx)
			if err != nil {
				t.Fatal(err)
			}
			return r.RowsAffected
		}
		if n := up("A", t0.Add(time.Hour)); n != 1 {
			t.Fatalf("insert: rows=%d, want 1", n)
		}
		if n := up("B", t0); n != 0 {
			t.Fatalf("older proposed row: rows=%d, want 0", n)
		}
		if h, _ := holderOf(t, ctx, db); h != "A" {
			t.Fatalf("holder = %s, want A", h)
		}
		if n := up("B", t0.Add(2*time.Hour)); n != 1 {
			t.Fatalf("newer proposed row: rows=%d, want 1", n)
		}
		if h, _ := holderOf(t, ctx, db); h != "B" {
			t.Fatalf("holder = %s, want B", h)
		}
	})
}
