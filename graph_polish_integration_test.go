// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/comment"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/note"
	"github.com/azrtydxb/momo/internal/relfixture/tag"
)

// A soft-deleted row is not "found": FindOrCreate fails with
// ErrUniqueViolation, leaves the deleted row as it was and creates no second one.
func TestFindOrCreateSoftDeletedRowIntegration(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		for _, s := range []struct {
			id      int64
			deleted bool
		}{{1, true}, {2, false}} {
			if _, err := momo.Exec(ctx, db, `INSERT INTO notes (id, document_id, body, deleted) VALUES (?, 1, ?, ?)`, s.id, "old", s.deleted); err != nil {
				t.Fatal(err)
			}
		}
		key := []momo.ColumnRef{note.ID}

		m := &models.Note{ID: 1, DocumentID: 1, Body: "new"}
		created, err := momo.FindOrCreate[models.Note, *models.Note](ctx, db, key, m)
		if created || !errors.Is(err, momo.ErrUniqueViolation) {
			t.Fatalf("soft-deleted match: created=%v err=%v, want ErrUniqueViolation", created, err)
		}
		if m.Body != "new" || m.Deleted {
			t.Fatalf("m must not be overwritten by the deleted row: %+v", m)
		}
		rows, err := momo.Select[models.Note, *models.Note](db).WithDeleted().Where(note.ID.Eq(1)).All(ctx)
		if err != nil || len(rows) != 1 || !rows[0].Deleted || rows[0].Body != "old" {
			t.Fatalf("the deleted row must stay as it was: %+v err=%v", rows, err)
		}

		// A live row is found, and an unused key is created.
		live := &models.Note{ID: 2, DocumentID: 1, Body: "ignored"}
		if created, err = momo.FindOrCreate[models.Note, *models.Note](ctx, db, key, live); err != nil || created || live.Body != "old" {
			t.Fatalf("live row: created=%v err=%v %+v", created, err, live)
		}
		fresh := &models.Note{ID: 3, DocumentID: 1, Body: "fresh"}
		if created, err = momo.FindOrCreate[models.Note, *models.Note](ctx, db, key, fresh); err != nil || !created {
			t.Fatalf("unused key: created=%v err=%v", created, err)
		}
	})
}

// A many-to-many child that is already stored is inserted again, not linked:
// the graph fails with ErrUniqueViolation and is undone. Link is the way to
// associate existing children.
func TestCreateGraphPersistedManyToManyChildIntegration(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		doc := &models.Document{ID: 20, Title: "graph", Tags: []models.Tag{{ID: 1, Name: "go"}}}
		err := momo.CreateGraph(ctx, db, doc, document.Tags.Include())
		if !errors.Is(err, momo.ErrUniqueViolation) {
			t.Fatalf("persisted many-to-many child: want ErrUniqueViolation, got %v", err)
		}
		n, err := momo.Select[models.Document, *models.Document](db).Where(document.ID.Eq(20)).Count(ctx)
		if err != nil || n != 0 {
			t.Fatalf("the failed graph must be undone: %d documents, err=%v", n, err)
		}

		// Link associates the stored tag with a stored document.
		doc = &models.Document{ID: 20, Title: "graph"}
		if err := momo.CreateGraph(ctx, db, doc); err != nil {
			t.Fatal(err)
		}
		existing := &models.Tag{ID: 1, Name: "go"}
		if err := document.Tags.Link(ctx, db, doc, existing); err != nil {
			t.Fatal(err)
		}
		if n, err := document.Tags.Count(ctx, db, doc); err != nil || n != 1 {
			t.Fatalf("Link: %d join rows, err=%v", n, err)
		}
		if c, err := momo.Select[models.Tag, *models.Tag](db).Where(tag.ID.Eq(1)).Count(ctx); err != nil || c != 1 {
			t.Fatalf("Link must not insert the tag again: %d, err=%v", c, err)
		}
	})
}

// 1000 children are written and read back through the bulk path.
func TestCreateGraphThousandChildrenIntegration(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		doc := &models.Document{ID: 30, Title: "big", Comments: make([]models.Comment, 1000)}
		for i := range doc.Comments {
			doc.Comments[i] = models.Comment{ID: int64(1000 + i), Body: "b", Approved: i%2 == 0}
		}
		if err := momo.CreateGraph(ctx, db, doc, document.Comments.Include()); err != nil {
			t.Fatal(err)
		}
		n, err := momo.Select[models.Comment, *models.Comment](db).Where(comment.DocumentID.Eq(30)).Count(ctx)
		if err != nil || n != 1000 {
			t.Fatalf("stored children: %d, err=%v", n, err)
		}
		if doc.Comments[999].DocumentID != 30 {
			t.Fatalf("child key not written back: %+v", doc.Comments[999])
		}
	})
}
