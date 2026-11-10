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

// createWithSrc recreates del_notes (1 d1, 2 d2) and the source table del_src
// (1 new1, 2 new2) the FROM / USING tests join against.
func createWithSrc(t *testing.T, db *momo.DB) {
	t.Helper()
	createDelNotes(t, db)
	ctx := context.Background()
	for _, stmt := range []string{
		"DROP TABLE IF EXISTS del_src",
		"CREATE TABLE del_src (id integer primary key, title text not null)",
		"INSERT INTO del_src (id, title) VALUES (1, 'new1'), (2, 'new2')",
	} {
		if _, err := momo.Exec(ctx, db, stmt); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _, _ = momo.Exec(context.Background(), db, "DROP TABLE IF EXISTS del_src") })
}

var (
	delSrc      = momo.TableRef{Name: "del_src"}
	delSrcID    = momo.ColumnOf[int64]("del_src", "id")
	delSrcTitle = momo.ColumnOf[string]("del_src", "title")
)

func noteTitle(t *testing.T, db *momo.DB, id int64) string {
	t.Helper()
	n, err := momo.Select[delNote](db).Where(delNoteID.Eq(id)).First(context.Background())
	if err != nil {
		t.Fatalf("note %d: %v", id, err)
	}
	return n.Title
}

// Catches: UPDATE ... FROM (PostgreSQL, SQLite) and the multi-table UPDATE
// (MySQL) not joining the source, or updating rows the WHERE excludes.
func TestUpdateFromIntegration(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		createWithSrc(t, db)
		ctx := context.Background()
		res, err := momo.Update[delNote](db).
			Set(delNoteTitle.SetExpr(delSrcTitle)).
			From(delSrc).
			Where(delNoteID.EqExpr(delSrcID), delNoteID.Eq(2)).
			Exec(ctx)
		if err != nil || res.RowsAffected != 1 {
			t.Fatalf("update from: %+v %v", res, err)
		}
		if got := noteTitle(t, db, 2); got != "new2" {
			t.Fatalf("note 2 title = %q, want new2", got)
		}
		if got := noteTitle(t, db, 1); got != "d1" {
			t.Fatalf("note 1 title = %q, want d1 (untouched)", got)
		}
	})
}

// Catches: DELETE ... USING deleting every row instead of the joined ones, and
// SQLite not refusing it with ErrUnsupported.
func TestDeleteUsingIntegration(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		createWithSrc(t, db)
		ctx := context.Background()
		q := momo.Delete[delNote](db).Using(delSrc).Where(delNoteID.EqExpr(delSrcID), delSrcTitle.Eq("new1"))
		res, err := q.Exec(ctx)
		if d == dialect.SQLite {
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("sqlite err = %v, want ErrUnsupported", err)
			}
			return
		}
		if err != nil || res.RowsAffected != 1 {
			t.Fatalf("delete using: %+v %v", res, err)
		}
		left, err := momo.Select[delNote](db).Count(ctx)
		if err != nil || left != 1 {
			t.Fatalf("rows left = %d %v, want 1", left, err)
		}
	})
}

// Catches: a WITH clause on UPDATE and DELETE that is dropped or whose CTE
// the statement cannot read.
func TestDMLWithIntegration(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		createWithSrc(t, db)
		ctx := context.Background()
		body := momo.SelectExprs(delSrcID, momo.Value("from-cte")).From(delSrc).Where(delSrcID.Eq(1))
		res, err := momo.Update[delNote](db).
			With("s", body, "id", "title").
			Set(delNoteTitle.SetExpr(momo.ColumnOf[string]("s", "title"))).
			From(momo.CTE("s")).
			Where(delNoteID.EqExpr(momo.ColumnOf[int64]("s", "id"))).
			Exec(ctx)
		if err != nil || res.RowsAffected != 1 {
			t.Fatalf("update with: %+v %v", res, err)
		}
		if got := noteTitle(t, db, 1); got != "from-cte" {
			t.Fatalf("note 1 title = %q, want from-cte", got)
		}
		ids := momo.SelectExprs(delSrcID).From(delSrc).Where(delSrcID.Eq(2))
		res, err = momo.Delete[delNote](db).
			With("gone", ids, "id").
			Where(delNoteID.InQuery(momo.SelectExprs(momo.ColumnOf[int64]("gone", "id")).From(momo.CTE("gone")))).
			Exec(ctx)
		if err != nil || res.RowsAffected != 1 {
			t.Fatalf("delete with: %+v %v", res, err)
		}
	})
}

// Catches: a data-modifying CTE that is not executed, whose RETURNING rows the
// outer statement cannot read, or that is accepted on a dialect without it.
func TestDMLAsCTEIntegration(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		createWithSrc(t, db)
		ctx := context.Background()
		ins := momo.Insert[delNote](db).Values(delNoteID.Set(3), delNoteTitle.Set("n3")).ReturningAll()
		sel := momo.Select[delNote](db).With("x", ins).From(momo.CTE("x"))
		if d != dialect.Postgres {
			if _, err := sel.All(ctx); !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("err = %v, want ErrUnsupported", err)
			}
			return
		}
		got, err := sel.All(ctx)
		if err != nil || len(got) != 1 || got[0].ID != 3 || got[0].Title != "n3" {
			t.Fatalf("insert cte = %+v %v", got, err)
		}
		if title := noteTitle(t, db, 3); title != "n3" {
			t.Fatalf("inserted row title = %q", title)
		}

		// An UPDATE CTE read by a DELETE ... USING that removes another row. (A
		// row changed by the CTE itself would not be deleted: PostgreSQL gives
		// the UPDATE precedence within one statement.)
		upd := momo.Update[delNote](db).Set(delNoteTitle.Set("u1")).Where(delNoteID.Eq(1)).Returning(delNoteID)
		res, err := momo.Delete[delNote](db).With("u", upd).Using(momo.CTE("u")).
			Where(delNoteID.Eq(2)).Exec(ctx)
		if err != nil || res.RowsAffected != 1 {
			t.Fatalf("delete over update cte: %+v %v", res, err)
		}
		if title := noteTitle(t, db, 1); title != "u1" {
			t.Fatalf("note 1 title = %q, want u1 (the UPDATE CTE ran)", title)
		}
	})
}
