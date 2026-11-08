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

func TestMaxRowsGuard(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1,'a'),(1,'b'),(1,'c')"); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Select[suite.Document](db).MaxRows(2).All(ctx); !errors.Is(err, momo.ErrLimitExceeded) {
			t.Fatalf("MaxRows(2) err = %v, want ErrLimitExceeded", err)
		}
		rows, err := momo.Select[suite.Document](db).MaxRows(3).All(ctx)
		if err != nil || len(rows) != 3 {
			t.Fatalf("MaxRows(3) rows=%d err=%v", len(rows), err)
		}
	})
}

func TestInListCap(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		capped := momo.New(db.Driver(), momo.WithMaxInList(2))
		_, err := momo.Select[suite.Document](capped).Where(suite.DocTitle.In("a", "b", "c")).SQL()
		if !errors.Is(err, momo.ErrLimitExceeded) {
			t.Fatalf("IN over cap err = %v", err)
		}
		if _, err := momo.Select[suite.Document](capped).Where(suite.DocTitle.In("a", "b")).SQL(); err != nil {
			t.Fatalf("IN at cap: %v", err)
		}
	})
}
