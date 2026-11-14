// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/tag"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// Catches: CreateGraph and FindOrCreate reaching a MySQL-family server, where
// the first has no RETURNING to read keys back with and the second has no
// ON CONFLICT, instead of failing with ErrUnsupported before anything is
// written; and a refused graph leaving rows behind.
func TestGraphWritesAreUnsupportedOnMySQLFamily(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		relfixture.CreateSchema(t, db)
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		doc := &models.Document{ID: 10, Title: "new", Comments: []models.Comment{{ID: 20, Body: "hi", Approved: true}}}
		err := momo.CreateGraph(ctx, db, doc, document.Comments.Include())
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("CreateGraph = %v, want ErrUnsupported", err)
		}
		n, err := momo.Select[models.Document, *models.Document](db).Count(ctx)
		if err != nil || n != 4 {
			t.Fatalf("documents after the refused graph = %d, %v, want the 4 seeded", n, err)
		}

		_, err = momo.FindOrCreate[models.Tag, *models.Tag](ctx, db, []momo.ColumnRef{tag.ID}, &models.Tag{ID: 99, Name: "x"})
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("FindOrCreate = %v, want ErrUnsupported", err)
		}
	})
}
