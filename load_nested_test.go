// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/comment"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
)

// TestNestedLoadDepthAndSharedChildren pins a three-level nesting
// (documents -> author -> first document -> scoped comments): one statement per
// level however many parents share a child, the scope applied at the deepest
// level only, and the nested result visible on every parent's copy.
func TestNestedLoadDepthAndSharedChildren(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		docs := allDocs(t, db)
		h.After = nil

		err := momo.LoadAll(context.Background(), db, docs,
			document.Author.Then(
				author.FirstDocument.Then(
					document.Comments.Scope(
						momo.LoadWhere[models.Comment](comment.Approved.Eq(true)),
						momo.LoadOrderBy[models.Comment](comment.ID.Desc()),
					),
				),
			))
		if err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 3 {
			t.Fatalf("want one statement per level (3), got %d: %v", len(h.After), h.After)
		}
		// Documents 1 and 2 share author 1 (copied per parent); both copies carry
		// author 1's first document (document 1) with its approved comments, newest first.
		for _, i := range []int{0, 1} {
			a := docs[i].Author
			if a == nil || a.FirstDocument == nil || a.FirstDocument.ID != 1 {
				t.Fatalf("docs[%d] nested first document missing: %+v", i, a)
			}
			if got := fmt.Sprint(commentIDs(a.FirstDocument.Comments)); got != "[3 1]" {
				t.Fatalf("docs[%d] deepest comments: got %s want [3 1]", i, got)
			}
		}
		// Document 3's author (2) has first document 2, which has no comments.
		if a := docs[2].Author; a == nil || a.FirstDocument == nil || a.FirstDocument.ID != 2 || len(a.FirstDocument.Comments) != 0 {
			t.Fatalf("docs[2] nested chain wrong: %+v", a)
		}
		// The orphan document has no author, hence no nested loads.
		if docs[3].Author != nil {
			t.Fatalf("orphan document must have no author: %+v", docs[3].Author)
		}
		// The scope applies to the leaf only: the document's own comments stay unloaded.
		if len(docs[0].Comments) != 0 {
			t.Fatalf("top-level comments must not be loaded: %v", docs[0].Comments)
		}
	})
}

// TestNestedLoadFilterAtLeaf checks that a filter on the nested level does not
// leak to the level above: the unfiltered top-level comments keep the
// unapproved row, while the nested scope drops it.
func TestNestedLoadFilterAtLeaf(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		docs := allDocs(t, db)
		h.After = nil

		err := momo.LoadAll(context.Background(), db, docs,
			document.Comments,
			document.Author.Then(author.FirstDocument.Then(
				document.Comments.Scope(momo.LoadWhere[models.Comment](comment.Approved.Eq(false))),
			)),
		)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprint(commentIDs(docs[0].Comments)); got != "[1 2 3]" {
			t.Fatalf("top-level comments unfiltered: got %s want [1 2 3]", got)
		}
		if got := fmt.Sprint(commentIDs(docs[0].Author.FirstDocument.Comments)); got != "[2]" {
			t.Fatalf("nested unapproved comments: got %s want [2]", got)
		}
	})
}
