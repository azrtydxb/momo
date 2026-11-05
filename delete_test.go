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

func TestDeleteByModelAndGuard(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, 'd1'), (1, 'd2')"); err != nil {
			t.Fatal(err)
		}
		doc, err := momo.Select[suite.Document](db).Where(suite.DocTitle.Eq("d1")).One(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if res, err := momo.Delete[suite.Document](db).Model(&doc).Exec(ctx); err != nil || res.RowsAffected != 1 {
			t.Fatalf("delete by model: %+v %v", res, err)
		}
		if _, err := momo.Delete[suite.Document](db).Exec(ctx); !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Fatalf("unfiltered delete err = %v", err)
		}
		// An empty junction is not a filter.
		if _, err := momo.Delete[suite.Document](db).Where(momo.And()).Exec(ctx); !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Fatalf("Where(And()) delete err = %v", err)
		}
		if _, err := momo.Delete[suite.Document](db).Model(nil).Exec(ctx); !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("Model(nil) err = %v", err)
		}
		res, err := momo.Delete[suite.Document](db).AllRows().Exec(ctx)
		if err != nil || res.RowsAffected != 1 {
			t.Fatalf("AllRows delete: %+v %v", res, err)
		}
	})
}
