// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/comment"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/employee"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/region"
	"github.com/azrtydxb/momo/internal/relfixture/store"
	"github.com/azrtydxb/momo/internal/relfixture/tagging"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
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

func TestEagerLoadComposite(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		stores, err := momo.Select[models.Store, *models.Store](db).OrderBy(store.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := momo.Load(ctx, db, stores, store.Region); err != nil {
			t.Fatal(err)
		}
		// store 1 -> (1,EU) Europe; store 2 -> (1,US) America; store 3 NULL region_code -> nil; store 4 -> (2,EU) Europe2.
		want := map[int64]string{1: "Europe", 2: "America", 4: "Europe2"}
		for _, s := range stores {
			if name, ok := want[s.ID]; ok {
				if s.Region == nil || s.Region.Name != name {
					t.Fatalf("store %d: region %+v want %s", s.ID, s.Region, name)
				}
			} else if s.Region != nil {
				t.Fatalf("store %d with NULL composite component must have nil Region", s.ID)
			}
		}

		regions, err := momo.Select[models.Region, *models.Region](db).OrderBy(region.TenantID.Asc(), region.Code.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := momo.Load(ctx, db, regions, region.Stores); err != nil {
			t.Fatal(err)
		}
		if len(regions[0].Stores) != 1 || regions[0].Stores[0].Name != "Paris" {
			t.Fatalf("region (1,EU) stores: %+v", regions[0].Stores)
		}
		if len(regions[1].Stores) != 1 || regions[1].Stores[0].Name != "NYC" {
			t.Fatalf("region (1,US) stores: %+v", regions[1].Stores)
		}
		if len(regions[2].Stores) != 1 || regions[2].Stores[0].Name != "Other" {
			t.Fatalf("region (2,EU) stores: %+v", regions[2].Stores)
		}
	})
}

func relationStatements(h *relfixture.Hook) int {
	n := 0
	for _, r := range h.After {
		if r != "" {
			n++
		}
	}
	return n
}

// Composite keys must keep the tuple semantics of the single-column paths:
// one statement for many parents, deduplicated parent keys, primary-key child
// order independent of insertion order, tuple (not per-column) matching, and a
// partly NULL key loads nothing.
func TestEagerLoadCompositeSemantics(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		// Two more stores in region (1,EU), inserted out of id order.
		for _, r := range []struct {
			id   int64
			name string
		}{{6, "Nice"}, {5, "Lyon"}} {
			if _, err := momo.Exec(ctx, db, "INSERT INTO stores (id, tenant_id, region_code, name) VALUES (?, 1, 'EU', ?)", r.id, r.name); err != nil {
				t.Fatal(err)
			}
		}

		// Belongs-to: stores 1, 5, 6 share region (1,EU); one statement, all assigned.
		stores, err := momo.Select[models.Store, *models.Store](db).OrderBy(store.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		h.After = nil
		if err := momo.Load(ctx, db, stores, store.Region); err != nil {
			t.Fatal(err)
		}
		if n := relationStatements(h); n != 1 {
			t.Fatalf("belongs-to issued %d relation statements, want 1", n)
		}
		for _, s := range stores {
			switch s.ID {
			case 1, 5, 6:
				if s.Region == nil || s.Region.TenantID != 1 || s.Region.Code != "EU" {
					t.Fatalf("store %d: region %+v want (1,EU)", s.ID, s.Region)
				}
			case 4:
				if s.Region == nil || s.Region.TenantID != 2 || s.Region.Code != "EU" {
					t.Fatalf("store %d: region %+v want (2,EU), tuple matching crossed columns", s.ID, s.Region)
				}
			}
		}
		if stores[0].Region == stores[3].Region {
			t.Fatal("distinct composite keys must not share a child")
		}

		// Has-many: children in primary-key order, one statement for all regions.
		regions, err := momo.Select[models.Region, *models.Region](db).OrderBy(region.TenantID.Asc(), region.Code.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		// A duplicated parent row must not duplicate children or the query keys.
		regions = append(regions, regions[0])
		h.After = nil
		if err := momo.Load(ctx, db, regions, region.Stores); err != nil {
			t.Fatal(err)
		}
		if n := relationStatements(h); n != 1 {
			t.Fatalf("has-many issued %d relation statements, want 1", n)
		}
		ids := func(cs []models.Store) string {
			out := []int64{}
			for _, c := range cs {
				out = append(out, c.ID)
			}
			return fmt.Sprint(out)
		}
		if got := ids(regions[0].Stores); got != "[1 5 6]" {
			t.Fatalf("region (1,EU) stores %s, want [1 5 6] in primary-key order", got)
		}
		if got := ids(regions[3].Stores); got != "[1 5 6]" {
			t.Fatalf("duplicate parent row stores %s, want [1 5 6]", got)
		}
		if got := ids(regions[2].Stores); got != "[4]" {
			t.Fatalf("region (2,EU) stores %s, want [4]", got)
		}
	})
}

// A region with no stores gets a nil slice, and reloading with a scope that
// matches nothing clears previously loaded composite associations.
func TestEagerLoadCompositeEmptyAndStale(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		if _, err := momo.Exec(ctx, db, "INSERT INTO regions (tenant_id, code, name) VALUES (3, 'ZZ', 'Empty')"); err != nil {
			t.Fatal(err)
		}
		regions, err := momo.Select[models.Region, *models.Region](db).OrderBy(region.TenantID.Asc(), region.Code.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := momo.Load(ctx, db, regions, region.Stores); err != nil {
			t.Fatal(err)
		}
		if last := regions[len(regions)-1]; last.Code != "ZZ" || last.Stores != nil {
			t.Fatalf("region without stores: %+v, want nil Stores", last)
		}
		if regions[0].Stores == nil {
			t.Fatal("seed region lost its stores")
		}
		if err := momo.LoadAll(ctx, db, regions, region.Stores.Scope(momo.LoadWhere[models.Store](store.ID.Eq(999)))); err != nil {
			t.Fatal(err)
		}
		for _, r := range regions {
			if r.Stores != nil {
				t.Fatalf("region (%d,%s) kept stale stores after reload", r.TenantID, r.Code)
			}
		}

		stores, err := momo.Select[models.Store, *models.Store](db).OrderBy(store.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := momo.Load(ctx, db, stores, store.Region); err != nil {
			t.Fatal(err)
		}
		if err := momo.LoadAll(ctx, db, stores, store.Region.Scope(momo.LoadWhere[models.Region](region.Code.Eq("nope")))); err != nil {
			t.Fatal(err)
		}
		for _, s := range stores {
			if s.Region != nil {
				t.Fatalf("store %d kept a stale region after reload", s.ID)
			}
		}
	})
}

// NULL in any key component means no key: a descriptor whose extractor reports
// a key with a nil element (ok = true) must still not query or assign.
func TestEagerLoadCompositeNullComponent(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		rel := store.Region
		rel.ParentKey = func(p *models.Store) ([]any, bool) {
			var code any
			if p.RegionCode != nil {
				code = *p.RegionCode
			}
			if p.ID == 3 {
				return []any{nil, code}, true // NULL first component, non-NULL second
			}
			return []any{p.TenantID, code}, true
		}
		stores, err := momo.Select[models.Store, *models.Store](db).Where(store.ID.In(3)).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		h.After = nil
		if err := momo.Load(ctx, db, stores, rel); err != nil {
			t.Fatal(err)
		}
		if n := relationStatements(h); n != 0 {
			t.Fatalf("a key with a NULL component issued %d statements, want 0", n)
		}
		if stores[0].Region != nil {
			t.Fatalf("NULL component matched a region: %+v", stores[0].Region)
		}
	})
}

func TestEagerLoadChunking(t *testing.T) {
	h := &relfixture.Hook{}
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		h.After, h.Events = nil, nil
		relfixture.CreateSchema(t, db)
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		docs := allDocs(t, db) // 4 documents; keys 1,2,3,4 -> with budget 4-16 < 1, chunk size is 1
		h.After = nil
		if err := momo.Load(ctx, db, docs, document.Comments); err != nil {
			t.Fatal(err)
		}
		// keysPerStatement reserves 16 parameters, so with MaxParams 4 each chunk holds one key.
		// Four distinct keys -> four child statements, all tagged with the relation.
		if len(h.After) != 4 {
			t.Fatalf("want 4 chunked child statements, got %d: %v", len(h.After), h.After)
		}
		total := 0
		for _, d := range docs {
			total += len(d.Comments)
		}
		if total != 4 {
			t.Fatalf("chunked load lost or duplicated comments: total %d", total)
		}
	}, momo.WithHook(h), momo.WithMaxParams(4))
}

func TestEagerLoadManyToMany(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		docs := allDocs(t, db)
		h.After = nil
		if err := momo.Load(ctx, db, docs, document.Tags); err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 {
			t.Fatalf("many-to-many must be one statement, got %d", len(h.After))
		}
		names := map[int64][]string{}
		for _, d := range docs {
			for _, tg := range d.Tags {
				names[d.ID] = append(names[d.ID], tg.Name)
			}
		}
		if fmt.Sprint(names[1]) != "[go db]" || len(names[2]) != 0 || len(names[3]) != 1 || names[3][0] != "db" {
			t.Fatalf("tags by doc: %v", names)
		}
	})
}

// LoadOrderBy orders each parent's tags; without the order clause the join
// would return them in key order (go, db).
func TestEagerLoadManyToManyOrder(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		docs := allDocs(t, db)
		name := momo.NewColumn[string]("tags", "name", 0)
		if err := momo.Load(context.Background(), db, docs, document.Tags, momo.LoadOrderBy[models.Tag](name.Desc())); err != nil {
			t.Fatal(err)
		}
		if docs[0].Tags[0].Name != "go" || docs[0].Tags[1].Name != "db" {
			t.Fatalf("desc order: %+v", docs[0].Tags)
		}
		if err := momo.Load(context.Background(), db, docs, document.Tags, momo.LoadOrderBy[models.Tag](name.Asc())); err != nil {
			t.Fatal(err)
		}
		if docs[0].Tags[0].Name != "db" || docs[0].Tags[1].Name != "go" {
			t.Fatalf("asc order: %+v", docs[0].Tags)
		}
	})
}

// A tag shared by two parents is delivered to both as independent copies, in
// per-parent order, and stale tags are cleared on reload. A parent whose key
// extractor reports no key stays empty.
func TestEagerLoadManyToManySharedCopiesStaleAndNull(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		docs := allDocs(t, db)
		docs[1].Tags = []models.Tag{{ID: 99, Name: "stale"}} // doc 2 has no tags
		docs[3].Tags = []models.Tag{{ID: 98, Name: "stale"}} // doc 4: NULL key below
		rel := document.Tags
		rel.ParentKey = func(p *models.Document) ([]any, bool) { return []any{p.ID}, p.ID != 4 }
		h.After = nil
		if err := momo.Load(ctx, db, docs, rel); err != nil {
			t.Fatal(err)
		}
		if len(docs[1].Tags) != 0 || len(docs[3].Tags) != 0 {
			t.Fatalf("stale tags survived: %+v %+v", docs[1].Tags, docs[3].Tags)
		}
		if len(docs[0].Tags) != 2 || len(docs[2].Tags) != 1 {
			t.Fatalf("tags: doc1=%+v doc3=%+v", docs[0].Tags, docs[2].Tags)
		}
		if docs[0].Tags[0].Name != "go" || docs[0].Tags[1].Name != "db" || docs[2].Tags[0].Name != "db" {
			t.Fatalf("order or shared tag wrong: %+v %+v", docs[0].Tags, docs[2].Tags)
		}
		docs[0].Tags[1].Name = "mutated"
		if docs[2].Tags[0].Name != "db" {
			t.Fatal("a tag shared by two parents aliases the same memory")
		}
		if n := relationStatements(h); n != 1 {
			t.Fatalf("want 1 statement, got %d", n)
		}
	})
}

func TestPolymorphicAndThrough(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		docs := allDocs(t, db)
		if err := momo.Load(ctx, db, docs, document.Images); err != nil {
			t.Fatal(err)
		}
		if len(docs[0].Images) != 1 || docs[0].Images[0].URL != "u1" {
			t.Fatalf("document images: %+v", docs[0].Images)
		}

		authors, err := momo.Select[models.Author, *models.Author](db).OrderBy(author.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := momo.Load(ctx, db, authors, author.Images); err != nil {
			t.Fatal(err)
		}
		if len(authors[0].Images) != 1 || authors[0].Images[0].URL != "u2" {
			t.Fatalf("author images must not mix with document images: %+v", authors[0].Images)
		}

		// Join model: document -> taggings (with Position) -> tag.
		if err := momo.LoadAll(ctx, db, docs, document.Taggings.Then(tagging.Tag)); err != nil {
			t.Fatal(err)
		}
		if len(docs[0].Taggings) != 2 || docs[0].Taggings[0].Position != 1 || docs[0].Taggings[0].Tag == nil || docs[0].Taggings[0].Tag.Name != "go" {
			t.Fatalf("join rows with tags: %+v", docs[0].Taggings)
		}
		if docs[0].Taggings[1].Position != 2 || docs[0].Taggings[1].Tag == nil || docs[0].Taggings[1].Tag.Name != "db" {
			t.Fatalf("second join row: %+v", docs[0].Taggings[1])
		}
		if len(docs[1].Taggings) != 0 {
			t.Fatal("document 2 has no taggings")
		}

		emps, err := momo.Select[models.Employee, *models.Employee](db).OrderBy(employee.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := momo.Load(ctx, db, emps, employee.Reports); err != nil {
			t.Fatal(err)
		}
		if len(emps[0].Reports) != 2 || len(emps[1].Reports) != 0 {
			t.Fatalf("self-reference back-reference: %+v", emps[0].Reports)
		}
		if err := momo.Load(ctx, db, emps, employee.Manager); err != nil {
			t.Fatal(err)
		}
		if emps[0].Manager != nil || emps[1].Manager == nil || emps[1].Manager.Name != "boss" || emps[2].Manager.Name != "boss" {
			t.Fatalf("self-reference belongs-to: %+v %+v", emps[0].Manager, emps[1].Manager)
		}
	})
}

// A polymorphic relation chunked under a tiny bind limit keeps the
// discriminator on every statement: the discriminator bind is budgeted, so
// no chunk mixes in another type's rows and none exceeds the limit.
func TestPolymorphicChunkingKeepsDiscriminator(t *testing.T) {
	h := &relfixture.Hook{}
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		h.After, h.Events = nil, nil
		relfixture.CreateSchema(t, db)
		relfixture.SeedBasic(t, db)
		docs := allDocs(t, db)
		h.After = nil
		if err := momo.Load(context.Background(), db, docs, document.Images); err != nil {
			t.Fatal(err)
		}
		if len(h.After) != len(docs) {
			t.Fatalf("want one statement per key under the limit, got %d", len(h.After))
		}
		if len(docs[0].Images) != 1 || docs[0].Images[0].URL != "u1" {
			t.Fatalf("document images: %+v", docs[0].Images)
		}
		for _, d := range docs[1:] {
			if len(d.Images) != 0 {
				t.Fatalf("document %d got images %+v", d.ID, d.Images)
			}
		}
	}, momo.WithHook(h), momo.WithMaxParams(16+1+1))
}

func TestNestedFilteredEagerLoad(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		docs := allDocs(t, db)
		h.After = nil

		loaders := []momo.Loader[models.Document]{
			document.Comments.Scope(
				momo.LoadWhere[models.Comment](comment.Approved.Eq(true)),
				momo.LoadOrderBy[models.Comment](comment.ID.Desc()),
			),
			document.Taggings.Then(tagging.Tag),
		}
		if err := momo.LoadAll(ctx, db, docs, loaders...); err != nil {
			t.Fatal(err)
		}
		// Two top-level statements (comments, taggings) and one nested statement (tags).
		if len(h.After) != 3 {
			t.Fatalf("want 3 statements (comments, taggings, nested tags), got %d: %v", len(h.After), h.After)
		}
		var ids []int64
		for _, c := range docs[0].Comments {
			ids = append(ids, c.ID)
		}
		if fmt.Sprint(ids) != "[3 1]" {
			t.Fatalf("approved comments newest-first: %v", ids)
		}
		if docs[0].Taggings[1].Tag == nil || docs[0].Taggings[1].Tag.Name != "db" {
			t.Fatal("nested tag not loaded")
		}
	})
}
