// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/comment"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
)

func TestRelationPredicates(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		withApproved := momo.Select[models.Document, *models.Document](db).
			Where(document.Comments.Has(comment.Approved.As("momo_r").Eq(true))).OrderBy(document.ID.Asc())
		c, err := withApproved.SQL()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(c.SQL, "EXISTS (SELECT 1 FROM") || strings.Contains(c.SQL, "JOIN") {
			t.Fatalf("HasX must render as EXISTS without JOIN: %s", c.SQL)
		}
		docs, err := withApproved.All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(docIDs(docs)) != "[1 3]" {
			t.Fatalf("documents with an approved comment: %v", docIDs(docs))
		}
		noComments := momo.Select[models.Document, *models.Document](db).
			Where(document.Comments.HasNo()).OrderBy(document.ID.Asc())
		c, err = noComments.SQL()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(c.SQL, "NOT EXISTS (SELECT 1 FROM") || strings.Contains(c.SQL, "JOIN") {
			t.Fatalf("HasNo must render as NOT EXISTS without JOIN: %s", c.SQL)
		}
		got, err := noComments.All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(docIDs(got)) != "[2 4]" {
			t.Fatalf("documents without comments: %v", docIDs(got))
		}

		// HasWith is Has; HasFrom with the default qualifier is Has.
		for name, p := range map[string]momo.Predicate{
			"HasWith": document.Comments.HasWith(),
			"HasFrom": document.Comments.HasFrom("documents"),
		} {
			got, err := momo.Select[models.Document, *models.Document](db).Where(p).OrderBy(document.ID.Asc()).All(ctx)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if fmt.Sprint(docIDs(got)) != "[1 3]" {
				t.Fatalf("%s: %v", name, docIDs(got))
			}
		}

		// Many-to-many: EXISTS goes through the join table, never an outer JOIN.
		tagged := momo.Select[models.Document, *models.Document](db).
			Where(document.Tags.Has()).OrderBy(document.ID.Asc())
		c, err = tagged.SQL()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(c.SQL, "EXISTS (SELECT 1 FROM") || !strings.Contains(c.SQL, `"momo_j"`) {
			t.Fatalf("many-to-many Has must go through the join table: %s", c.SQL)
		}
		got, err = tagged.All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(docIDs(got)) != "[1 3]" {
			t.Fatalf("documents with a tag: %v", docIDs(got))
		}
	})
}

func docIDs(ds []models.Document) []int64 {
	out := []int64{}
	for _, d := range ds {
		out = append(out, d.ID)
	}
	return out
}

func TestRelationTraverseAndOrder(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		parents := momo.Select[models.Document, *models.Document](db).Where(document.ID.Eq(1))
		comments, err := document.Comments.From(parents).OrderBy(comment.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(commentIDs(comments)) != "[1 2 3]" {
			t.Fatalf("traversal to typed child query: %v", commentIDs(comments))
		}
		ordered, err := momo.Select[models.Document, *models.Document](db).
			OrderBy(document.Comments.CountOrder(momo.DirDesc), document.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(docIDs(ordered)) != "[1 3 2 4]" {
			t.Fatalf("order by relation count desc: %v", docIDs(ordered))
		}
		asc, err := momo.Select[models.Document, *models.Document](db).
			OrderBy(document.Comments.CountOrder(momo.DirAsc), document.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(docIDs(asc)) != "[2 4 3 1]" {
			t.Fatalf("order by relation count asc: %v", docIDs(asc))
		}
	})
}

func commentIDs(cs []models.Comment) []int64 {
	out := []int64{}
	for _, c := range cs {
		out = append(out, c.ID)
	}
	return out
}
