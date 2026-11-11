// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// Catches: a grouped INSERT ... SELECT inserting the wrong rows (HAVING
// ignored or GROUP BY dropped), or SQL the server rejects.
func TestExprSelectGroupedInsertExecutes(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("PostgreSQL integration")
		}
		createJobsLock(t, db,
			"INSERT INTO jobs_lock (name, holder, until) VALUES ('a', 'h1', now()), ('b', 'h1', now()), ('c', 'h2', now())")
		ctx := context.Background()
		res, err := groupedLease(db).Exec(ctx)
		if err != nil || res.RowsAffected != 1 {
			t.Fatalf("grouped insert = %+v, %v; want 1 row", res, err)
		}
		// Only h1 has two rows: it inserts (name h1, holder max(name) = b).
		l, err := momo.Select[Lease](db).Where(leaseName.Eq("h1")).One(ctx)
		if err != nil || l.Holder != "b" || !l.Live {
			t.Fatalf("inserted = %+v, %v", l, err)
		}
	})
}

// Catches: a join in an ExprSelect source dropping or duplicating rows.
func TestExprSelectJoinExecutes(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("PostgreSQL integration")
		}
		createJobsLock(t, db,
			"INSERT INTO jobs_lock (name, holder, until) VALUES ('a', 'o1', now()), ('b', 'zz', now())",
			"DROP TABLE IF EXISTS owners",
			"CREATE TABLE owners (name text primary key)",
			"INSERT INTO owners VALUES ('o1')")
		ctx := context.Background()
		src := momo.SelectExprs(momo.Value("x"), ownerName, momo.TypedSQL[time.Time]("now()")).From(leaseTable).
			Join(leaseOwners, leaseHolder.EqExpr(ownerName)).OrderBy(ownerName.Desc())
		res, err := momo.Insert[Lease](db).FromSelect(src, leaseName, leaseHolder, leaseUntil).
			OnConflict(leaseName).DoNothing().Exec(ctx)
		if err != nil || res.RowsAffected != 1 {
			t.Fatalf("join insert = %+v, %v; want 1 row", res, err)
		}
		if n, err := momo.Select[Lease](db).Where(leaseName.Eq("x"), leaseHolder.Eq("o1")).Count(ctx); err != nil || n != 1 {
			t.Fatalf("rows = %d, %v", n, err)
		}
	})
}
