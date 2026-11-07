// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestUpsertRefreshesUpdatedAtKeepsCreatedAt(t *testing.T) {
	t1 := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	t2 := t1.Add(2 * time.Hour)
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		db1 := momo.New(db.Driver(), clockAt(t1))
		db2 := momo.New(db.Driver(), clockAt(t2))
		doc := suite.Document{ID: 501, TenantID: 1, Title: "one"}
		if _, err := momo.Upsert[suite.Document](db1).Model(&doc).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		doc2 := suite.Document{ID: 501, TenantID: 1, Title: "two"}
		if _, err := momo.Upsert[suite.Document](db2).Model(&doc2).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		got, err := momo.Select[suite.Document](db).Where(suite.DocID.Eq(501)).One(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.Title != "two" || !got.UpdatedAt.Equal(t2) || !got.CreatedAt.Equal(t1) {
			t.Fatalf("created %v updated %v title %q; want created %v updated %v", got.CreatedAt, got.UpdatedAt, got.Title, t1, t2)
		}
	})
}

func TestUpsertLeavesVersionUnchangedOnConflict(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		doc := suite.Document{ID: 502, TenantID: 1, Title: "v", Version: 5}
		if _, err := momo.Upsert[suite.Document](db).Model(&doc).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		again := suite.Document{ID: 502, TenantID: 1, Title: "v2", Version: 99}
		if _, err := momo.Upsert[suite.Document](db).Model(&again).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		got, err := momo.Select[suite.Document](db).Where(suite.DocID.Eq(502)).One(ctx)
		if err != nil || got.Title != "v2" || got.Version != 5 {
			t.Fatalf("version = %d title %q (%v); want version 5 untouched and title v2", got.Version, got.Title, err)
		}
	})
}
