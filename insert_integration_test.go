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

func TestInsertModelOmitAndConflict(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		doc := suite.Document{TenantID: 3, Title: "m", Secret: "s"}
		if _, err := momo.Insert[suite.Document](db).Model(&doc).Omit(suite.DocBody).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		res, err := momo.Insert[suite.Document](db).
			Values(suite.DocTenantID.Set(3), suite.DocTitle.Set("m")).
			OnConflict(suite.DocID).DoNothing().Exec(ctx)
		if err != nil || res.RowsAffected != 1 {
			t.Fatalf("insert on conflict do nothing with new row: %+v %v", res, err)
		}
		got, err := momo.Select[suite.Document](db).Where(suite.DocTitle.Eq("m")).Count(ctx)
		if err != nil || got != 2 {
			t.Fatalf("count = %d, %v", got, err)
		}
	})
}

func TestInsertReturningPopulatesIdentity(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		doc := suite.Document{TenantID: 7, Title: "r", Secret: "s"}
		rows, err := momo.Insert[suite.Document](db).Model(&doc).Omit(suite.DocBody).
			Returning(suite.DocID, suite.DocTitle).Rows(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !rows.Next() {
			t.Fatal("no RETURNING row")
		}
		var got suite.Document
		if err := rows.ScanModel(&got); err != nil {
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		if got.ID == 0 || got.Title != "r" || got.TenantID != 0 {
			t.Fatalf("returned %+v", got)
		}
	})
}
