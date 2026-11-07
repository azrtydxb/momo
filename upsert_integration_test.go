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

func TestUpsertUpdatesAllColumns(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		doc := suite.Document{ID: 77, TenantID: 1, Title: "first"}
		if _, err := momo.Upsert[suite.Document](db).Model(&doc).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		doc.Title = "second"
		doc.TenantID = 9
		if _, err := momo.Upsert[suite.Document](db).Model(&doc).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		got, err := momo.Select[suite.Document](db).Where(suite.DocID.Eq(77)).One(ctx)
		if err != nil || got.Title != "second" || got.TenantID != 9 {
			t.Fatalf("upsert did not update all columns: %+v %v", got, err)
		}
		n, err := momo.Select[suite.Document](db).Count(ctx)
		if err != nil || n != 1 {
			t.Fatalf("rows = %d, %v; want 1", n, err)
		}
	})
}

func TestUpsertZeroIdentityInsertsNewRow(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		doc := suite.Document{TenantID: 1, Title: "fresh"}
		if _, err := momo.Upsert[suite.Document](db).Model(&doc).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		got, err := momo.Select[suite.Document](db).Where(suite.DocTitle.Eq("fresh")).One(ctx)
		if err != nil || got.ID == 0 {
			t.Fatalf("zero identity must be generated: %+v %v", got, err)
		}
	})
}
