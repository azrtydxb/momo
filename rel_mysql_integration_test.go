// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/comment"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/tag"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// Catches: CreateGraph on MySQL or MariaDB dropping the graph, changing the
// keys it was given, or leaving the child row out, and FindOrCreate reporting
// created for a row that exists (or the reverse). The read-back and race
// behaviour is covered in driver/mysqldriver.
func TestGraphWritesWorkOnMySQLFamily(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		relfixture.CreateSchema(t, db)
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		doc := &models.Document{ID: 10, Title: "new", Comments: []models.Comment{{ID: 20, Body: "hi", Approved: true}}}
		if err := momo.CreateGraph(ctx, db, doc, document.Comments.Include()); err != nil {
			t.Fatalf("CreateGraph = %v, want success", err)
		}
		if doc.ID != 10 || doc.Comments[0].ID != 20 || doc.Comments[0].DocumentID != 10 {
			t.Fatalf("graph keys = %d, %d, %d, want 10, 20, 10", doc.ID, doc.Comments[0].ID, doc.Comments[0].DocumentID)
		}
		n, err := momo.Select[models.Document, *models.Document](db).Count(ctx)
		if err != nil || n != 5 {
			t.Fatalf("documents after the graph = %d, %v, want the 4 seeded plus 1", n, err)
		}
		got, err := momo.Select[models.Comment, *models.Comment](db).Where(comment.ID.Eq(20)).One(ctx)
		if err != nil || got.DocumentID != 10 || got.Body != "hi" {
			t.Fatalf("stored comment = %+v, %v, want document 10 body hi", got, err)
		}

		key := []momo.ColumnRef{tag.ID}
		first := &models.Tag{ID: 99, Name: "x"}
		created, err := momo.FindOrCreate[models.Tag, *models.Tag](ctx, db, key, first)
		if err != nil || !created {
			t.Fatalf("first FindOrCreate = created %v, %v, want created", created, err)
		}
		again := &models.Tag{ID: 99, Name: "other"}
		created, err = momo.FindOrCreate[models.Tag, *models.Tag](ctx, db, key, again)
		if err != nil || created || again.Name != "x" {
			t.Fatalf("second FindOrCreate = created %v, name %q, %v, want found with the stored name x", created, again.Name, err)
		}
	})
}
