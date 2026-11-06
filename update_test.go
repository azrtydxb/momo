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

// TestMutationGuardrails covers the unfiltered UPDATE and DELETE guardrail.
func TestMutationGuardrails(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, 'g')"); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("x")).Exec(ctx); !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Fatalf("unfiltered update err = %v", err)
		}
		// documents is a soft-delete model, which Delete refuses outright, so
		// the unfiltered-DELETE guard is asserted on a model without one.
		createDelNotes(t, db)
		if _, err := momo.Delete[delNote](db).Exec(ctx); !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Fatalf("unfiltered delete err = %v", err)
		}
		res, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("all")).AllRows().Exec(ctx)
		if err != nil || res.RowsAffected != 1 {
			t.Fatalf("AllRows update: %+v %v", res, err)
		}
	})
}

func TestOptimisticLock(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, 'v')"); err != nil {
			t.Fatal(err)
		}
		doc, err := momo.Select[suite.Document](db).First(ctx)
		if err != nil {
			t.Fatal(err)
		}
		stale := doc
		doc.Title = "first"
		if _, err := momo.Update[suite.Document](db).Model(&doc).Exec(ctx); err != nil {
			t.Fatalf("fresh update: %v", err)
		}
		stale.Title = "second"
		if _, err := momo.Update[suite.Document](db).Model(&stale).Exec(ctx); !errors.Is(err, momo.ErrOptimisticLock) {
			t.Fatalf("stale update err = %v, want ErrOptimisticLock", err)
		}
		gone := doc
		gone.ID = 9999
		if _, err := momo.Update[suite.Document](db).Model(&gone).Exec(ctx); !errors.Is(err, momo.ErrNotFound) {
			t.Fatalf("missing row err = %v, want ErrNotFound", err)
		}
	})
}
