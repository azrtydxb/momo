// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// Catches: DELETE ... RETURNING not reaching a real MariaDB (rows lost or the
// delete not applied), or MySQL running it instead of refusing.
func TestDeleteReturningMySQLFamilyIntegration(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		createDelNotes(t, db)
		ctx := context.Background()
		// A multi-table DELETE with RETURNING is refused on every server,
		// before anything reaches the database: MariaDB rejects it, MySQL has
		// no RETURNING.
		_, err := momo.Delete[delNote](db).Using(delSrc).Where(delNoteID.EqExpr(delSrcID)).Returning(delNoteID).Rows(ctx)
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("DELETE USING RETURNING = %v, want ErrUnsupported", err)
		}
		if n, err := momo.Select[delNote](db).Count(ctx); err != nil || n != 2 {
			t.Fatalf("rows after refused multi-table delete = %d, %v; want 2", n, err)
		}
		if db.Dialect().Flavor != dialect.FlavorMariaDB {
			_, err := momo.Delete[delNote](db).Where(delNoteTitle.Eq("d2")).ReturningAll().Rows(ctx)
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("DELETE RETURNING on MySQL = %v, want ErrUnsupported", err)
			}
			n, err := momo.Select[delNote](db).Count(ctx)
			if err != nil || n != 2 {
				t.Fatalf("rows after refused delete = %d, %v; want 2", n, err)
			}
			return
		}
		rows, err := momo.Delete[delNote](db).Where(delNoteTitle.Eq("d2")).ReturningAll().Rows(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var got []delNote
		for rows.Next() {
			var n delNote
			if err := rows.ScanModel(&n); err != nil {
				t.Fatal(err)
			}
			got = append(got, n)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != 2 || got[0].Title != "d2" {
			t.Fatalf("returned = %+v", got)
		}
		one, err := momo.Delete[delNote](db).Where(delNoteID.Eq(1)).Returning(delNoteTitle).One(ctx)
		if err != nil || one.Title != "d1" {
			t.Fatalf("One = %+v, %v", one, err)
		}
		left, err := momo.Select[delNote](db).Count(ctx)
		if err != nil || left != 0 {
			t.Fatalf("rows left = %d, %v", left, err)
		}
	})
}
