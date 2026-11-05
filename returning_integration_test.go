// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
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
