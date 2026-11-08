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

func TestCopyInsert(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		ms := []suite.Document{{TenantID: 1, Title: "c1"}, {TenantID: 2, Title: "c2"}}
		ins := func(q momo.Querier) momo.InsertQuery[suite.Document] {
			return momo.Insert[suite.Document](q).Omit(suite.DocBody, suite.DocSecret, suite.DocDeletedAt)
		}
		if d == dialect.SQLite {
			if _, err := ins(db).Copy(ctx, ms); !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("sqlite COPY err = %v, want ErrUnsupported", err)
			}
			return
		}
		res, err := ins(db).Copy(ctx, ms)
		if err != nil || res.RowsAffected != 2 {
			t.Fatalf("copy: %+v %v", res, err)
		}
		got, err := momo.Select[suite.Document](db).Where(suite.DocTitle.In("c1", "c2")).OrderBy(suite.DocTitle.Asc()).All(ctx)
		if err != nil || len(got) != 2 || got[0].TenantID != 1 || got[1].TenantID != 2 {
			t.Fatalf("copied rows = %+v %v", got, err)
		}
		if got[0].CreatedAt.IsZero() {
			t.Fatalf("copied row defaults = %+v", got[0])
		}

		// COPY has no parameter limit, so one call writes more rows than a single INSERT can bind.
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		const n = 70000 // above PostgreSQL's 65535 parameter limit for one INSERT
		big := make([]suite.Document, n)
		for i := range big {
			big[i] = suite.Document{TenantID: int64(i % 7), Title: "bulk"}
		}
		if res, err := ins(db).Copy(ctx, big); err != nil || res.RowsAffected != n {
			t.Fatalf("big copy = %+v, %v", res, err)
		}
		if c, err := momo.Select[suite.Document](db).Count(ctx); err != nil || c != n {
			t.Fatalf("count after copy = %d, %v", c, err)
		}

		// Inside a transaction COPY is part of it: a rollback undoes the rows.
		if err := momo.Truncate[suite.Document](ctx, db); err != nil {
			t.Fatal(err)
		}
		errRollback := errors.New("rollback")
		err = db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if _, err := ins(tx).Copy(ctx, ms); err != nil {
				return err
			}
			return errRollback
		})
		if !errors.Is(err, errRollback) {
			t.Fatalf("tx err = %v", err)
		}
		if c, err := momo.Select[suite.Document](db).Count(ctx); err != nil || c != 0 {
			t.Fatalf("count after rolled-back copy = %d, %v, want 0", c, err)
		}
	})
}
