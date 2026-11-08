// SPDX-License-Identifier: Apache-2.0

//go:build integration

package relfixture_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/store"
)

func TestRelfixtureSchemaApplies(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		n, err := momo.Select[models.Document, *models.Document](db).Count(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if n != 4 {
			t.Fatalf("documents: got %d want 4", n)
		}
		if document.Author.Kind != momo.BelongsTo || document.Comments.Kind != momo.HasMany || document.Tags.Kind != momo.ManyToMany {
			t.Fatal("fixture relation kinds drifted from the generator contract")
		}
		if store.Region.ChildMeta == nil || len(store.Region.ParentCols) != 2 {
			t.Fatal("composite fixture relation is not composite")
		}
	})
}
