// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/comment"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/tagging"
)

// One loader carries a where, an order and a nested load: two statements, and
// the nested level runs over the kept children only (the excluded tagging's tag
// is never used as a nested parent key).
func TestScopeThenFiltersOrdersAndNestsIntegration(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		// Tag 3 is reachable only through a tagging the where excludes.
		if _, err := momo.Exec(ctx, db, `INSERT INTO tags (id, name) VALUES (3, 'extra')`); err != nil {
			t.Fatal(err)
		}
		if _, err := momo.Exec(ctx, db, `INSERT INTO taggings (document_id, tag_id, position) VALUES (3, 3, 9)`); err != nil {
			t.Fatal(err)
		}
		docs := allDocs(t, db)
		h.After = nil

		rel := tagging.Tag
		var keys []int64
		orig := rel.ParentKey
		rel.ParentKey = func(p *models.Tagging) ([]any, bool) {
			keys = append(keys, p.TagID)
			return orig(p)
		}
		err := momo.LoadAll(ctx, db, docs, document.Taggings.Scope(
			momo.LoadWhere[models.Tagging](tagging.Position.Le(2)),
			momo.LoadOrderBy[models.Tagging](tagging.Position.Desc()),
		).Then(rel))
		if err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 2 {
			t.Fatalf("want 1 top-level + 1 nested statement, got %v", h.After)
		}
		if slices.Contains(keys, 3) {
			t.Fatalf("an excluded child was used as a nested parent: %v", keys)
		}
		var names []string
		for _, tg := range docs[0].Taggings {
			if tg.Tag == nil {
				t.Fatalf("nested value missing on %+v", tg)
			}
			names = append(names, tg.Tag.Name)
		}
		if !slices.Equal(names, []string{"db", "go"}) {
			t.Fatalf("doc 1: filtered, position-descending taggings with tags, got %v", names)
		}
		if len(docs[2].Taggings) != 1 || docs[2].Taggings[0].TagID != 2 || docs[2].Taggings[0].Tag == nil {
			t.Fatalf("doc 3 must keep only its position<=2 tagging: %+v", docs[2].Taggings)
		}
	})
}

// Combining works at a nested level: the scope applies to the deepest relation
// only.
func TestScopeThenAtNestedLevelIntegration(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		docs := allDocs(t, db)
		err := momo.LoadAll(ctx, db, docs, document.Author.Then(
			author.FirstDocument.Scope(momo.LoadWhere[models.Document](document.Title.Eq("A1"))).
				Then(document.Comments.Scope(momo.LoadWhere[models.Comment](comment.Approved.Eq(true))))))
		if err != nil {
			t.Fatal(err)
		}
		if docs[0].Author == nil || docs[2].Author == nil {
			t.Fatal("the author level must not be filtered")
		}
		fd := docs[0].Author.FirstDocument
		if fd == nil || fd.ID != 1 || len(fd.Comments) != 2 {
			t.Fatalf("Ada's first document with approved comments only: %+v", fd)
		}
		if docs[2].Author.FirstDocument != nil {
			t.Fatalf("Grace's first document is excluded by the scope: %+v", docs[2].Author.FirstDocument)
		}
	})
}
