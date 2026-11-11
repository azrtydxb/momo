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

var (
	delOther    = momo.TableRef{Name: "del_other"}
	delOtherID  = momo.ColumnOf[int64]("del_other", "id")
	delOtherTag = momo.ColumnOf[string]("del_other", "tag")
)

// claimedRow is a plain struct that receives columns of the target, of a CTE
// and of a table in one RETURNING list.
type claimedRow struct {
	ID    int64
	Title string
	Tag   string
}

// createWithOther adds del_other (1 t1, 2 t2) next to del_notes and del_src.
func createWithOther(t *testing.T, db *momo.DB) {
	t.Helper()
	createWithSrc(t, db)
	ctx := context.Background()
	for _, stmt := range []string{
		"DROP TABLE IF EXISTS del_other",
		"CREATE TABLE del_other (id integer primary key, tag text not null)",
		"INSERT INTO del_other (id, tag) VALUES (1, 't1'), (2, 't2')",
	} {
		if _, err := momo.Exec(ctx, db, stmt); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _, _ = momo.Exec(context.Background(), db, "DROP TABLE IF EXISTS del_other") })
}

// Catches: a claim-and-return UPDATE with a CTE and a table as sources, whose
// RETURNING reads columns of both, failing to run or scan: the second source
// dropped, the columns unqualified, or the source columns not reaching the
// plain struct.
func TestUpdateFromTwoSourcesReturningIntegration(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		createWithOther(t, db)
		ctx := context.Background()
		due := momo.SelectExprs(delSrcID).From(delSrc).Where(delSrcID.Eq(1))
		q := momo.Update[delNote](db).With("due", due, "id").
			Set(delNoteTitle.Set("claimed")).
			From(momo.CTE("due"), delOther).
			Where(delNoteID.EqExpr(momo.ColumnOf[int64]("due", "id")), delNoteID.EqExpr(delOtherID)).
			Returning(delNoteID, delNoteTitle, delOtherTag)
		if d != dialect.Postgres {
			if _, err := momo.ScanUpdate[claimedRow](ctx, q); !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("%s err = %v, want ErrUnsupported", d, err)
			}
			return
		}
		got, err := momo.ScanUpdate[claimedRow](ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0] != (claimedRow{ID: 1, Title: "claimed", Tag: "t1"}) {
			t.Fatalf("rows = %+v, want one {1 claimed t1}", got)
		}
		if title := noteTitle(t, db, 2); title != "d2" {
			t.Fatalf("note 2 title = %q, want d2 (untouched)", title)
		}
	})
}

// Catches: an unqualified RETURNING with a source that shares a column name
// with the target (title is in both del_notes and del_src), which PostgreSQL
// rejects as ambiguous.
func TestReturningSharedNameQualifiedIntegration(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("a qualified RETURNING with a source is PostgreSQL only")
		}
		createWithSrc(t, db)
		ctx := context.Background()
		rows, err := momo.Update[delNote](db).Set(delNoteTitle.SetExpr(delSrcTitle)).From(delSrc).
			Where(delNoteID.EqExpr(delSrcID), delNoteID.Eq(2)).Returning(delNoteID, delNoteTitle).Rows(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rows.Close() }()
		var n delNote
		if !rows.Next() {
			t.Fatalf("no row: %v", rows.Err())
		}
		if err := rows.ScanModel(&n); err != nil || n.ID != 2 || n.Title != "new2" {
			t.Fatalf("row = %+v %v, want {2 new2}", n, err)
		}
	})
}

// Catches: DELETE ... USING with two sources and source columns in RETURNING
// deleting the wrong rows or not returning the source's values.
func TestDeleteUsingTwoSourcesReturningIntegration(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("DELETE ... RETURNING with a source is PostgreSQL only")
		}
		createWithOther(t, db)
		ctx := context.Background()
		got, err := momo.ScanDelete[claimedRow](ctx, momo.Delete[delNote](db).Using(delSrc, delOther).
			Where(delNoteID.EqExpr(delSrcID), delNoteID.EqExpr(delOtherID), delSrcTitle.Eq("new1")).
			Returning(delNoteID, delNoteTitle, delOtherTag))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0] != (claimedRow{ID: 1, Title: "d1", Tag: "t1"}) {
			t.Fatalf("rows = %+v, want one {1 d1 t1}", got)
		}
		left, err := momo.Select[delNote](db).Count(ctx)
		if err != nil || left != 1 {
			t.Fatalf("rows left = %d %v, want 1", left, err)
		}
	})
}
