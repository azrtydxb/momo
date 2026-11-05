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

func TestBulkInsertChunking(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		const n = 40000 // above the SQLite 32766 limit: forces chunking
		ms := make([]suite.Document, n)
		for i := range ms {
			ms[i] = suite.Document{TenantID: int64(i % 5), Title: "bulk"}
		}
		res, err := momo.Insert[suite.Document](db).Omit(suite.DocBody, suite.DocSecret, suite.DocDeletedAt).Models(ms).Exec(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if res.RowsAffected != n {
			t.Fatalf("RowsAffected = %d, want %d", res.RowsAffected, n)
		}
		count, err := momo.Select[suite.Document](db).Count(ctx)
		if err != nil || count != n {
			t.Fatalf("count = %d, %v", count, err)
		}
	})
}

func TestBulkInsertReturningAllChunksInOrder(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		const n = 40000
		ms := make([]suite.Document, n)
		for i := range ms {
			ms[i] = suite.Document{TenantID: int64(i), Title: "r"}
		}
		rows, err := momo.Insert[suite.Document](db).Omit(suite.DocBody, suite.DocSecret, suite.DocDeletedAt).
			Models(ms).Returning(suite.DocTenantID).Rows(ctx)
		if err != nil {
			t.Fatal(err)
		}
		i := 0
		for rows.Next() {
			var m suite.Document
			if err := rows.ScanModel(&m); err != nil {
				t.Fatal(err)
			}
			if m.TenantID != int64(i) {
				t.Fatalf("row %d has tenant %d, want in-order rows", i, m.TenantID)
			}
			i++
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil || i != n {
			t.Fatalf("rows = %d, close %v, want %d", i, err, n)
		}
		count, err := momo.Select[suite.Document](db).Count(ctx)
		if err != nil || count != n {
			t.Fatalf("count = %d, %v", count, err)
		}
	})
}

func TestBulkUpsertChunkingWithIdentity(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		const n = 40000
		ms := make([]suite.Document, n)
		for i := range ms {
			ms[i] = suite.Document{ID: int64(i + 1), TenantID: 1}
		}
		for _, title := range []string{"v1", "v2"} {
			for i := range ms {
				ms[i].Title = title
			}
			_, err := momo.Upsert[suite.Document](db).Omit(suite.DocBody, suite.DocSecret, suite.DocDeletedAt).Models(ms).Exec(ctx)
			if err != nil {
				t.Fatal(title, err)
			}
		}
		count, err := momo.Select[suite.Document](db).Count(ctx)
		if err != nil || count != n {
			t.Fatalf("count = %d, %v, want %d (second upsert must update, not insert)", count, err, n)
		}
		v2, err := momo.Select[suite.Document](db).Where(suite.DocTitle.Eq("v2")).Count(ctx)
		if err != nil || v2 != n {
			t.Fatalf("v2 rows = %d, %v", v2, err)
		}
	})
}

func TestBulkInsertChunkFailureRollsBackEarlierChunks(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("needs PostgreSQL's 'cannot affect row a second time' error")
		}
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		const n = 40000
		ms := make([]suite.Document, n)
		for i := range ms {
			ms[i] = suite.Document{ID: int64(i + 1), TenantID: 1, Title: "x"}
		}
		ms[n-1].ID = ms[n-2].ID // duplicate key inside the last chunk fails that chunk
		_, err := momo.Upsert[suite.Document](db).Omit(suite.DocBody, suite.DocSecret, suite.DocDeletedAt).Models(ms).Exec(ctx)
		if err == nil {
			t.Fatal("want the last chunk to fail")
		}
		count, err := momo.Select[suite.Document](db).Count(ctx)
		if err != nil || count != 0 {
			t.Fatalf("count = %d, %v, want 0: earlier chunks must roll back", count, err)
		}
	})
}

func TestBulkInsertChunksInsideCallerTx(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		const n = 40000
		ms := make([]suite.Document, n)
		for i := range ms {
			ms[i] = suite.Document{TenantID: 1, Title: "tx"}
		}
		err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			res, err := momo.Insert[suite.Document](tx).Omit(suite.DocBody, suite.DocSecret, suite.DocDeletedAt).Models(ms).Exec(ctx)
			if err != nil || res.RowsAffected != n {
				t.Fatalf("res = %+v, %v", res, err)
			}
			return context.Canceled // roll the caller's transaction back
		})
		if err == nil {
			t.Fatal("want the callback error")
		}
		count, err := momo.Select[suite.Document](db).Count(ctx)
		if err != nil || count != 0 {
			t.Fatalf("count = %d, %v, want 0 after the caller rolled back", count, err)
		}
	})
}
