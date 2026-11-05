// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// createDelNotes recreates the del_notes table behind delNote.
func createDelNotes(t *testing.T, db *momo.DB) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{
		"DROP TABLE IF EXISTS del_notes",
		"CREATE TABLE del_notes (id integer primary key, title text not null)",
		"INSERT INTO del_notes (id, title) VALUES (1, 'd1'), (2, 'd2')",
	} {
		if _, err := momo.Exec(ctx, db, stmt); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _, _ = momo.Exec(context.Background(), db, "DROP TABLE IF EXISTS del_notes") })
}

func TestDeleteByModelAndGuard(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		createDelNotes(t, db)
		ctx := context.Background()
		if res, err := momo.Delete[delNote](db).Model(&delNote{ID: 1}).Exec(ctx); err != nil || res.RowsAffected != 1 {
			t.Fatalf("delete by model: %+v %v", res, err)
		}
		if _, err := momo.Delete[delNote](db).Exec(ctx); !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Fatalf("unfiltered delete err = %v", err)
		}
		// An empty junction is not a filter.
		if _, err := momo.Delete[delNote](db).Where(momo.And()).Exec(ctx); !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Fatalf("Where(And()) delete err = %v", err)
		}
		if _, err := momo.Delete[delNote](db).Model(nil).Exec(ctx); !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("Model(nil) err = %v", err)
		}
		res, err := momo.Delete[delNote](db).AllRows().Exec(ctx)
		if err != nil || res.RowsAffected != 1 {
			t.Fatalf("AllRows delete: %+v %v", res, err)
		}
	})
}

func TestDeleteRefusesSoftDeleteModelIntegration(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, 'keep')"); err != nil {
			t.Fatal(err)
		}
		_, err := momo.Delete[suite.Document](db).Where(suite.DocTitle.Eq("keep")).Exec(ctx)
		if !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("delete on soft-delete model err = %v, want ErrValidation", err)
		}
		n, err := momo.Select[suite.Document](db).Count(ctx)
		if err != nil || n != 1 {
			t.Fatalf("row count = %d, %v; the row must survive", n, err)
		}
	})
}

func TestDeleteReturningRows(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if !suite.CapsOf(db).Returning {
			t.Skip("dialect has no RETURNING")
		}
		createDelNotes(t, db)
		ctx := context.Background()
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
		left, err := momo.Select[delNote](db).Count(ctx)
		if err != nil || left != 1 {
			t.Fatalf("rows left = %d, %v", left, err)
		}
	})
}
