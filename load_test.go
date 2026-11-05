// SPDX-License-Identifier: Apache-2.0

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
	"github.com/azrtydxb/momo/internal/relfixture/employee"
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

func TestEagerLoadHasMany(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		docs := allDocs(t, db)
		if err := momo.Load(context.Background(), db, docs, document.Comments); err != nil {
			t.Fatal(err)
		}
		got := map[int64][]int64{}
		for _, d := range docs {
			for _, c := range d.Comments {
				got[d.ID] = append(got[d.ID], c.ID)
			}
		}
		if fmt.Sprint(got[1]) != "[1 2 3]" || fmt.Sprint(got[3]) != "[4]" || len(got[2]) != 0 || len(got[4]) != 0 {
			t.Fatalf("comments by doc: %v", got)
		}
	})
}

func TestEagerLoadParentOrderAndPagination(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		// A page of parents, reversed and with a duplicate key on purpose.
		page, err := momo.Select[models.Document, *models.Document](db).
			Where(document.ID.Le(3)).OrderBy(document.ID.Desc()).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		page = append(page, page[0]) // duplicate parent row
		if err := momo.Load(context.Background(), db, page, document.Comments); err != nil {
			t.Fatal(err)
		}
		ids := []int64{}
		for _, d := range page {
			ids = append(ids, d.ID)
		}
		if fmt.Sprint(ids) != "[3 2 1 3]" {
			t.Fatalf("parent order changed: %v", ids)
		}
		if len(page[0].Comments) != 1 || len(page[3].Comments) != 1 || page[3].Comments[0].ID != 4 {
			t.Fatalf("duplicate parent must receive its own copy of the children: %+v", page[3].Comments)
		}
		if len(page[2].Comments) != 3 {
			t.Fatalf("parent 1 must have 3 comments, got %d", len(page[2].Comments))
		}
	})
}

func TestEagerLoadNullFKEmpty(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		emps, err := momo.Select[models.Employee, *models.Employee](db).OrderBy(employee.ID.Asc()).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		// Employee 1 has a NULL manager_id: Manager must stay nil, and Reports of boss must be 2 employees.
		if err := momo.Load(context.Background(), db, emps, employee.Manager); err != nil {
			t.Fatal(err)
		}
		if emps[0].Manager != nil || emps[1].Manager == nil || emps[1].Manager.Name != "boss" {
			t.Fatalf("manager loading wrong: %+v", emps)
		}
		if err := momo.Load(context.Background(), db, emps, employee.Reports); err != nil {
			t.Fatal(err)
		}
		if len(emps[0].Reports) != 2 || emps[0].Reports[0].Name != "dev1" || emps[0].Reports[1].Name != "dev2" {
			t.Fatalf("reports of boss: %+v", emps[0].Reports)
		}
		if emps[1].Reports != nil {
			t.Fatal("a leaf must have a nil slice, not an empty one")
		}
	})
}

func TestEagerLoadReloadClearsStaleChildren(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		docs := allDocs(t, db)
		if err := momo.Load(ctx, db, docs, document.Comments); err != nil {
			t.Fatal(err)
		}
		// Reload with a scope that matches nothing: every association must become empty.
		if err := momo.LoadAll(ctx, db, docs, document.Comments.Scope(momo.LoadWhere[models.Comment](comment.ID.Eq(999)))); err != nil {
			t.Fatal(err)
		}
		for _, d := range docs {
			if d.Comments != nil {
				t.Fatalf("doc %d kept stale comments after reload: %d", d.ID, len(d.Comments))
			}
		}
	})
}

// Children of one parent come back in primary-key order even when the physical
// row order differs (an UPDATE moves a row version in PostgreSQL heaps).
func TestEagerLoadHasManyOrderIndependentOfStorage(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		for _, id := range []int64{1, 2} {
			if _, err := momo.Exec(ctx, db, "UPDATE comments SET body = body || 'x' WHERE id = ?", id); err != nil {
				t.Fatal(err)
			}
		}
		docs := allDocs(t, db)
		if err := momo.Load(ctx, db, docs, document.Comments); err != nil {
			t.Fatal(err)
		}
		ids := []int64{}
		for _, c := range docs[0].Comments {
			ids = append(ids, c.ID)
		}
		if fmt.Sprint(ids) != "[1 2 3]" {
			t.Fatalf("children not in primary-key order: %v", ids)
		}
	})
}
