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

func TestReturningRendersAndScans(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		rows, err := momo.Insert[suite.Document](db).
			Values(suite.DocTenantID.Set(7), suite.DocTitle.Set("r")).
			Returning(suite.DocID, suite.DocTitle).
			Rows(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var doc suite.Document
		if !rows.Next() {
			t.Fatal("no RETURNING row")
		}
		if err := rows.ScanModel(&doc); err != nil {
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		if doc.ID == 0 || doc.Title != "r" {
			t.Fatalf("returned %+v", doc)
		}
	})
}

func TestReturningOnUpdateScansInColumnOrder(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (3, 'a'), (3, 'b'), (4, 'c')"); err != nil {
			t.Fatal(err)
		}
		// Title is listed before ID: scan order must follow the list.
		rows, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("z")).
			Where(suite.DocTenantID.Eq(3)).Returning(suite.DocTitle, suite.DocID).Rows(ctx)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for rows.Next() {
			var got suite.Document
			if err := rows.ScanModel(&got); err != nil {
				t.Fatal(err)
			}
			if got.Title != "z" || got.ID == 0 || got.TenantID != 0 {
				t.Fatalf("returned %+v", got)
			}
			n++
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		if n != 2 {
			t.Fatalf("returned %d rows, want 2", n)
		}

		all, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("q")).
			Where(suite.DocTenantID.Eq(4)).ReturningAll().Rows(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = all.Close() }()
		if !all.Next() {
			t.Fatal("no row")
		}
		var full suite.Document
		if err := all.ScanModel(&full); err != nil {
			t.Fatal(err)
		}
		if full.Title != "q" || full.TenantID != 4 || full.ID == 0 {
			t.Fatalf("ReturningAll %+v", full)
		}
	})
}

func TestWriteOneReturnsTheRow(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()

		// Insert with an explicit list, then without one (every column).
		got, err := momo.Insert[suite.Document](db).
			Values(suite.DocTenantID.Set(7), suite.DocTitle.Set("one")).
			Returning(suite.DocID, suite.DocTitle).One(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.ID == 0 || got.Title != "one" || got.TenantID != 0 {
			t.Fatalf("insert One = %+v", got)
		}
		all, err := momo.Insert[suite.Document](db).
			Values(suite.DocTenantID.Set(7), suite.DocTitle.Set("two")).One(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if all.ID == 0 || all.Title != "two" || all.TenantID != 7 {
			t.Fatalf("insert One without Returning = %+v", all)
		}

		up, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("upd")).
			Where(suite.DocID.Eq(all.ID)).One(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if up.ID != all.ID || up.Title != "upd" || up.TenantID != 7 {
			t.Fatalf("update One = %+v", up)
		}

		del, err := momo.ForceDelete[suite.Document](db).Where(suite.DocID.Eq(got.ID)).
			Returning(suite.DocID, suite.DocTitle).One(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if del.ID != got.ID || del.Title != "one" {
			t.Fatalf("delete One = %+v", del)
		}
	})
}

func TestWriteOneNotFound(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		if _, err := momo.Exec(ctx, db, "CREATE UNIQUE INDEX one_title_uq ON documents (tenant_id, title)"); err != nil {
			t.Fatal(err)
		}
		first, err := momo.Insert[suite.Document](db).
			Values(suite.DocTenantID.Set(1), suite.DocTitle.Set("a")).One(ctx)
		if err != nil {
			t.Fatal(err)
		}

		// ON CONFLICT DO NOTHING on an existing id returns no row.
		_, err = momo.Insert[suite.Document](db).
			Values(suite.DocTenantID.Set(1), suite.DocTitle.Set("a")).
			OnConflict(suite.DocTenantID, suite.DocTitle).DoNothing().ReturningAll().One(ctx)
		if !errors.Is(err, momo.ErrNotFound) {
			t.Fatalf("DO NOTHING insert err = %v, want ErrNotFound", err)
		}

		// A guarded update matching no row.
		_, err = momo.Update[suite.Document](db).Set(suite.DocTitle.Set("x")).
			Where(suite.DocID.Eq(first.ID), suite.DocTenantID.Eq(99)).One(ctx)
		if !errors.Is(err, momo.ErrNotFound) {
			t.Fatalf("guarded update err = %v, want ErrNotFound", err)
		}

		_, err = momo.ForceDelete[suite.Document](db).Where(suite.DocID.Eq(first.ID + 1000)).One(ctx)
		if !errors.Is(err, momo.ErrNotFound) {
			t.Fatalf("delete err = %v, want ErrNotFound", err)
		}
	})
}

func TestWriteOneTooManyRows(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (3, 'a'), (3, 'b')"); err != nil {
			t.Fatal(err)
		}
		_, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("z")).
			Where(suite.DocTenantID.Eq(3)).One(ctx)
		if !errors.Is(err, momo.ErrTooManyRows) {
			t.Fatalf("multi-row update err = %v, want ErrTooManyRows", err)
		}
		_, err = momo.ForceDelete[suite.Document](db).Where(suite.DocTenantID.Eq(3)).One(ctx)
		if !errors.Is(err, momo.ErrTooManyRows) {
			t.Fatalf("multi-row delete err = %v, want ErrTooManyRows", err)
		}
		// With WithImplicitTx the statement ran in a transaction that was rolled back: both rows are intact.
		n, err := momo.Select[suite.Document](db).Where(suite.DocTenantID.Eq(3), suite.DocTitle.In("a", "b")).Count(ctx)
		if err != nil || n != 2 {
			t.Fatalf("rows after refused writes = %d, %v; want 2", n, err)
		}
	}, momo.WithImplicitTx())
}
