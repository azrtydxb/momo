// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// Catches: the guarded INSERT ... SELECT inserting twice (the NOT EXISTS
// guard or ON CONFLICT lost), the rows-affected count being wrong, and
// RETURNING yielding a row for a skipped insert.
func TestFromSelectNotExistsOnce(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		createJobsLock(t, db)
		ctx := context.Background()
		run := func(name string) int64 {
			r, err := leaseGuarded(db, name).OnConflict(leaseName).DoNothing().Exec(ctx)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			return r.RowsAffected
		}
		if n := run("a"); n != 1 {
			t.Fatalf("first run affected %d rows, want 1", n)
		}
		if n := run("a"); n != 0 {
			t.Fatalf("second run affected %d rows, want 0", n)
		}
		if n := run("b"); n != 0 { // a live row exists, so the guard blocks a different key too
			t.Fatalf("guarded run for another key affected %d rows, want 0", n)
		}
		if n, err := momo.Select[Lease](db).Count(ctx); err != nil || n != 1 {
			t.Fatalf("rows = %d err=%v, want exactly 1", n, err)
		}
		if d != dialect.Postgres {
			return
		}
		// RETURNING: a fresh table inserts and returns the row, then returns nothing.
		createJobsLock(t, db)
		count := func() int {
			rows, err := leaseGuarded(db, "a").OnConflict(leaseName).DoNothing().Returning(leaseName, leaseHolder).Rows(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			n := 0
			for rows.Next() {
				var l Lease
				if err := rows.ScanModel(&l); err != nil {
					t.Fatal(err)
				}
				if l.Name != "a" || l.Holder != "h" {
					t.Fatalf("returned %+v", l)
				}
				n++
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			return n
		}
		if n := count(); n != 1 {
			t.Fatalf("first returning run = %d rows, want 1", n)
		}
		if n := count(); n != 0 {
			t.Fatalf("second returning run = %d rows, want 0", n)
		}
	})
}
