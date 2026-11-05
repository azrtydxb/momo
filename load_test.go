// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
)

func allDocs(t *testing.T, db *momo.DB) []models.Document {
	t.Helper()
	docs, err := momo.Select[models.Document, *models.Document](db).OrderBy(document.ID.Asc()).All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return docs
}

func TestEagerLoadBelongsTo(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		docs := allDocs(t, db)
		h.After = nil

		if err := momo.Load(context.Background(), db, docs, document.Author); err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 || h.After[0] != "documents.Author" {
			t.Fatalf("want exactly one child statement tagged documents.Author, got %v", h.After)
		}
		wantNames := map[int64]string{1: "Ada", 2: "Ada", 3: "Grace"}
		for _, d := range docs {
			want, ok := wantNames[d.ID]
			if !ok {
				if d.Author != nil {
					t.Fatalf("doc %d with NULL FK must keep Author nil", d.ID)
				}
				continue
			}
			if d.Author == nil || d.Author.Name != want {
				t.Fatalf("doc %d: author %+v, want %s", d.ID, d.Author, want)
			}
		}
		if docs[0].Author == docs[1].Author {
			t.Fatal("parents sharing a key must get distinct copies, not one shared pointer")
		}
	})
}

func TestEagerLoadHasOne(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		authors, err := momo.Select[models.Author, *models.Author](db).OrderBy(author.ID.Asc()).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if err := momo.Load(context.Background(), db, authors, author.FirstDocument); err != nil {
			t.Fatal(err)
		}
		if authors[0].FirstDocument == nil || authors[0].FirstDocument.ID != 1 || authors[1].FirstDocument == nil || authors[1].FirstDocument.ID != 2 {
			t.Fatalf("has-one loaded wrong documents: %+v %+v", authors[0].FirstDocument, authors[1].FirstDocument)
		}
	})
}

func TestRelationHookEvents(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		docs := allDocs(t, db)
		if err := momo.Load(context.Background(), db, docs, document.Author); err != nil {
			t.Fatal(err)
		}
		if len(h.Events) != 1 {
			t.Fatalf("want one RelationEvent, got %d", len(h.Events))
		}
		e := h.Events[0]
		if e.Parent != "documents" || e.Name != "Author" || e.Kind != momo.BelongsTo || e.Parents != 4 || e.Children != 2 || e.Queries != 1 {
			t.Fatalf("unexpected event %+v", e)
		}
	})
}
