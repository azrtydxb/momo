// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/comment"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/employee"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/note"
	"github.com/azrtydxb/momo/internal/relfixture/region"
	"github.com/azrtydxb/momo/internal/relfixture/store"
	"github.com/azrtydxb/momo/internal/relfixture/tagging"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// A duplicated join row must not consume the per-parent limit: post 1 has tag
// 1 twice, and the top-2 must still be two distinct tags. Soft-deleted far
// rows are excluded before ranking.
func TestTopNManyToManyDuplicateJoinRows(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		ctx := context.Background()
		for _, s := range []string{
			"DROP TABLE IF EXISTS mm_post_tags",
			"DROP TABLE IF EXISTS mm_tags",
			"DROP TABLE IF EXISTS mm_posts",
			"CREATE TABLE mm_posts (id BIGINT PRIMARY KEY)",
			"CREATE TABLE mm_tags (id BIGINT PRIMARY KEY, name TEXT NOT NULL, deleted_at TIMESTAMP NULL)",
			"CREATE TABLE mm_post_tags (post_id BIGINT NOT NULL, tag_id BIGINT NOT NULL)",
			"INSERT INTO mm_posts (id) VALUES (1), (2)",
			"INSERT INTO mm_tags (id, name, deleted_at) VALUES (1, 'a', NULL), (2, 'b', NULL), (3, 'c', NULL), (4, 'gone', CURRENT_TIMESTAMP)",
			"INSERT INTO mm_post_tags (post_id, tag_id) VALUES (1, 1), (1, 1), (1, 2), (1, 3), (1, 4), (2, 3), (2, 3), (2, 2)",
		} {
			mustExec(t, db, s)
		}
		t.Cleanup(func() {
			for _, s := range []string{"mm_post_tags", "mm_tags", "mm_posts"} {
				_, _ = momo.Exec(context.Background(), db, "DROP TABLE IF EXISTS "+s)
			}
		})
		ps := []mmPost{{ID: 1}, {ID: 2}}
		if err := momo.Load(ctx, db, ps, mmTags, momo.LoadPerParentLimit[mmTag](2)); err != nil {
			t.Fatal(err)
		}
		if got := mmNames(ps[0]) + " " + mmNames(ps[1]); got != "[a b] [b c]" {
			t.Fatalf("top-2 distinct tags per post: %s", got)
		}
	})
}

// With LoadOrderBy on a non-unique column (every tag has the same name) and
// duplicated join rows, each post still gets exactly N distinct tags: ties are
// broken by the child primary key and duplicates share a rank.
func TestTopNManyToManyNonUniqueOrderDuplicateJoinRows(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		ctx := context.Background()
		for _, s := range []string{
			"DROP TABLE IF EXISTS mm_post_tags",
			"DROP TABLE IF EXISTS mm_tags",
			"DROP TABLE IF EXISTS mm_posts",
			"CREATE TABLE mm_posts (id BIGINT PRIMARY KEY)",
			"CREATE TABLE mm_tags (id BIGINT PRIMARY KEY, name TEXT NOT NULL, deleted_at TIMESTAMP NULL)",
			"CREATE TABLE mm_post_tags (post_id BIGINT NOT NULL, tag_id BIGINT NOT NULL)",
			"INSERT INTO mm_posts (id) VALUES (1), (2)",
			"INSERT INTO mm_tags (id, name) VALUES (1, 'same'), (2, 'same'), (3, 'same'), (4, 'same')",
			"INSERT INTO mm_post_tags (post_id, tag_id) VALUES (1, 1), (1, 1), (1, 1), (1, 2), (1, 3), (1, 4), (2, 3), (2, 3), (2, 2), (2, 4)",
		} {
			mustExec(t, db, s)
		}
		t.Cleanup(func() {
			for _, s := range []string{"mm_post_tags", "mm_tags", "mm_posts"} {
				_, _ = momo.Exec(context.Background(), db, "DROP TABLE IF EXISTS "+s)
			}
		})
		ps := []mmPost{{ID: 1}, {ID: 2}}
		err := momo.Load(ctx, db, ps, mmTags,
			momo.LoadOrderBy[mmTag](momo.Order{Expr: momo.ColumnOf[string]("mm_tags", "name")}),
			momo.LoadPerParentLimit[mmTag](3))
		if err != nil {
			t.Fatal(err)
		}
		ids := func(p mmPost) []int64 {
			var out []int64
			for _, k := range p.Tags {
				out = append(out, k.ID)
			}
			return out
		}
		if got := fmt.Sprint(ids(ps[0]), ids(ps[1])); got != "[1 2 3] [2 3 4]" {
			t.Fatalf("exactly 3 distinct tags per post in a stable order: %s", got)
		}
	})
}

func mustExec(t *testing.T, db *momo.DB, sql string, args ...any) {
	t.Helper()
	if _, err := momo.Exec(context.Background(), db, sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func TestEagerLoadTopNPerParent(t *testing.T) {
	relfixture.EachWithMySQL(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		docs := allDocs(t, db)
		h.After = nil
		err := momo.LoadAll(context.Background(), db, docs, document.Comments.Scope(
			momo.LoadPerParentLimit[models.Comment](2),
			momo.LoadOrderBy[models.Comment](comment.ID.Asc()),
		))
		if err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 {
			t.Fatalf("top-N must be one statement per chunk, got %d", len(h.After))
		}
		if fmt.Sprint(ids(docs[0].Comments)) != "[1 2]" {
			t.Fatalf("doc 1 top-2: got %v want [1 2]", ids(docs[0].Comments))
		}
		if len(docs[2].Comments) != 1 || docs[2].Comments[0].ID != 4 {
			t.Fatalf("doc 3 must keep its only comment: %+v", docs[2].Comments)
		}
		if len(docs[1].Comments) != 0 || len(docs[3].Comments) != 0 {
			t.Fatalf("parents without children stay empty: %+v %+v", docs[1].Comments, docs[3].Comments)
		}
	})
}

// TestWindowFunctions pins the window semantics the strategy relies on: ranks
// restart in every partition and follow the ORDER BY.
func TestWindowFunctions(t *testing.T) {
	relfixture.EachWithMySQL(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ranks, err := momo.Scalars[int64](context.Background(), db,
			`SELECT ROW_NUMBER() OVER (PARTITION BY document_id ORDER BY id DESC) FROM comments ORDER BY document_id, id DESC`)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(ranks) != "[1 2 3 1]" {
			t.Fatalf("ranks per partition: %v", ranks)
		}
	})
}

func TestTopNPerParentVariants(t *testing.T) {
	relfixture.EachWithMySQL(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		docs := allDocs(t, db)
		cids := func(i int) string { return fmt.Sprint(ids(docs[i].Comments)) }

		if err := momo.Load(ctx, db, docs, document.Comments,
			momo.LoadPerParentLimit[models.Comment](2), momo.LoadOrderBy[models.Comment](comment.ID.Desc())); err != nil {
			t.Fatal(err)
		}
		if cids(0) != "[3 2]" || cids(2) != "[4]" {
			t.Fatalf("desc top-2: %s %s", cids(0), cids(2))
		}
		if err := momo.Load(ctx, db, docs, document.Comments, momo.LoadPerParentLimit[models.Comment](10)); err != nil {
			t.Fatal(err)
		}
		if cids(0) != "[1 2 3]" {
			t.Fatalf("limit above count keeps all in primary-key order: %s", cids(0))
		}
		if err := momo.Load(ctx, db, docs, document.Comments, momo.LoadPerParentLimit[models.Comment](1)); err != nil {
			t.Fatal(err)
		}
		if cids(0) != "[1]" {
			t.Fatalf("top-1: %s", cids(0))
		}

		// LoadWhere filters before ranking.
		if err := momo.Load(ctx, db, docs, document.Comments,
			momo.LoadPerParentLimit[models.Comment](2), momo.LoadWhere[models.Comment](comment.Approved.Eq(true))); err != nil {
			t.Fatal(err)
		}
		if cids(0) != "[1 3]" {
			t.Fatalf("where applies before the limit: %s", cids(0))
		}
		if err := momo.Load(ctx, db, docs, document.Comments,
			momo.LoadPerParentLimit[models.Comment](1), momo.LoadWhere[models.Comment](comment.ID.Gt(1))); err != nil {
			t.Fatal(err)
		}
		if cids(0) != "[2]" {
			t.Fatalf("where applies before the limit (top-1): %s", cids(0))
		}

		// Many-to-many partitions by the join table's parent key.
		name := momo.NewColumn[string]("tags", "name", 0)
		if err := momo.Load(ctx, db, docs, document.Tags,
			momo.LoadPerParentLimit[models.Tag](1), momo.LoadOrderBy[models.Tag](name.Asc())); err != nil {
			t.Fatal(err)
		}
		if len(docs[0].Tags) != 1 || docs[0].Tags[0].Name != "db" || len(docs[2].Tags) != 1 || docs[2].Tags[0].Name != "db" || len(docs[1].Tags) != 0 {
			t.Fatalf("m2m top-1: %+v %+v %+v", docs[0].Tags, docs[1].Tags, docs[2].Tags)
		}
		if err := momo.Load(ctx, db, docs, document.Tags,
			momo.LoadPerParentLimit[models.Tag](1), momo.LoadOrderBy[models.Tag](name.Desc())); err != nil {
			t.Fatal(err)
		}
		if len(docs[0].Tags) != 1 || docs[0].Tags[0].Name != "go" {
			t.Fatalf("m2m top-1 desc: %+v", docs[0].Tags)
		}

		// Self-reference: has-many Reports.
		emps, err := momo.Select[models.Employee, *models.Employee](db).OrderBy(employee.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := momo.Load(ctx, db, emps, employee.Reports,
			momo.LoadPerParentLimit[models.Employee](1), momo.LoadOrderBy[models.Employee](employee.ID.Desc())); err != nil {
			t.Fatal(err)
		}
		if len(emps[0].Reports) != 1 || emps[0].Reports[0].ID != 3 || len(emps[1].Reports) != 0 {
			t.Fatalf("reports top-1: %+v", emps[0].Reports)
		}
	})
}

func TestTopNPerParentSoftDeleteComposite(t *testing.T) {
	relfixture.EachWithMySQL(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		// Soft-deleted children are excluded before ranking.
		mustExec(t, db, `INSERT INTO notes (id, document_id, body, deleted) VALUES (1, 1, 'a', TRUE), (2, 1, 'b', FALSE), (3, 1, 'c', FALSE)`)
		docs := allDocs(t, db)
		if err := momo.Load(ctx, db, docs, document.Notes,
			momo.LoadPerParentLimit[models.Note](1), momo.LoadOrderBy[models.Note](note.ID.Asc())); err != nil {
			t.Fatal(err)
		}
		if len(docs[0].Notes) != 1 || docs[0].Notes[0].ID != 2 {
			t.Fatalf("soft-deleted note must not take the slot: %+v", docs[0].Notes)
		}
		if err := momo.Load(ctx, db, docs, document.Notes, momo.LoadWithDeleted[models.Note](),
			momo.LoadPerParentLimit[models.Note](1), momo.LoadOrderBy[models.Note](note.ID.Asc())); err != nil {
			t.Fatal(err)
		}
		if len(docs[0].Notes) != 1 || docs[0].Notes[0].ID != 1 {
			t.Fatalf("with deleted: %+v", docs[0].Notes)
		}

		// Composite parent key: partition by (tenant_id, region_code).
		mustExec(t, db, `INSERT INTO stores (id, tenant_id, region_code, name) VALUES (5, 1, 'EU', 'Lyon'), (6, 2, 'EU', 'Bonn')`)
		regions, err := momo.Select[models.Region, *models.Region](db).OrderBy(region.TenantID.Asc(), region.Code.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := momo.Load(ctx, db, regions, region.Stores,
			momo.LoadPerParentLimit[models.Store](1), momo.LoadOrderBy[models.Store](store.ID.Desc())); err != nil {
			t.Fatal(err)
		}
		got := map[string]int64{}
		for _, r := range regions {
			if len(r.Stores) != 1 {
				t.Fatalf("region %d/%s: %+v", r.TenantID, r.Code, r.Stores)
			}
			got[fmt.Sprintf("%d/%s", r.TenantID, r.Code)] = r.Stores[0].ID
		}
		if fmt.Sprint(got) != "map[1/EU:5 1/US:2 2/EU:6]" {
			t.Fatalf("composite top-1: %v", got)
		}
	})
}

func TestTopNPerParentPolymorphic(t *testing.T) {
	relfixture.EachWithMySQL(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		mustExec(t, db, `INSERT INTO images (id, owner_type, owner_id, url) VALUES (3, 'document', 1, 'u3'), (4, 'author', 1, 'u4'), (5, 'document', 1, 'u5')`)
		docs := allDocs(t, db)
		if err := momo.Load(ctx, db, docs, document.Images, momo.LoadPerParentLimit[models.Image](2)); err != nil {
			t.Fatal(err)
		}
		var got []int64
		for _, im := range docs[0].Images {
			got = append(got, im.ID)
		}
		if fmt.Sprint(got) != "[1 3]" {
			t.Fatalf("document images keep the discriminator and the limit: %v", got)
		}
		authors, err := momo.Select[models.Author, *models.Author](db).OrderBy(author.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := momo.Load(ctx, db, authors, author.Images, momo.LoadPerParentLimit[models.Image](1)); err != nil {
			t.Fatal(err)
		}
		if len(authors[0].Images) != 1 || authors[0].Images[0].ID != 2 {
			t.Fatalf("author images: %+v", authors[0].Images)
		}
	})
}

// Nested loads run once, over the kept children only.
func TestTopNPerParentNested(t *testing.T) {
	relfixture.EachWithMySQL(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		docs := allDocs(t, db)
		h.Events = nil
		l := document.Taggings.Scope(
			momo.LoadPerParentLimit[models.Tagging](1),
			momo.LoadOrderBy[models.Tagging](tagging.Position.Asc()),
		).Then(tagging.Tag)
		if err := momo.LoadAll(context.Background(), db, docs, l); err != nil {
			t.Fatal(err)
		}
		if len(docs[0].Taggings) != 1 || docs[0].Taggings[0].Tag == nil || docs[0].Taggings[0].Tag.Name != "go" {
			t.Fatalf("doc 1 keeps its first tagging with its tag: %+v", docs[0].Taggings)
		}
		if len(docs[2].Taggings) != 1 || docs[2].Taggings[0].Tag == nil || docs[2].Taggings[0].Tag.Name != "db" {
			t.Fatalf("doc 3: %+v", docs[2].Taggings)
		}
		var nested *momo.RelationEvent
		for i := range h.Events {
			if h.Events[i].Name == "Tag" {
				nested = &h.Events[i]
			}
		}
		if nested == nil || nested.Parents != 2 {
			t.Fatalf("nested Tag must load over the 2 kept taggings (not 3): %+v", h.Events)
		}
	})
}

func TestTopNPerParentChunking(t *testing.T) {
	h := &relfixture.Hook{}
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		h.After, h.Events = nil, nil
		relfixture.CreateSchema(t, db)
		relfixture.SeedBasic(t, db)
		docs := allDocs(t, db)
		h.After = nil
		if err := momo.Load(context.Background(), db, docs, document.Comments,
			momo.LoadPerParentLimit[models.Comment](2), momo.LoadOrderBy[models.Comment](comment.ID.Asc())); err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 4 {
			t.Fatalf("want one statement per key under MaxParams 4, got %d", len(h.After))
		}
		if fmt.Sprint(ids(docs[0].Comments)) != "[1 2]" || len(docs[2].Comments) != 1 {
			t.Fatalf("chunked top-2: %v %v", ids(docs[0].Comments), ids(docs[2].Comments))
		}
	}, momo.WithHook(h), momo.WithMaxParams(4))
}

func TestTopNPerParentRejections(t *testing.T) {
	relfixture.EachWithMySQL(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		docs := allDocs(t, db)
		authors, _ := momo.Select[models.Author, *models.Author](db).OrderBy(author.ID.Asc()).All(ctx)
		h.After = nil
		err := momo.Load(ctx, db, docs, document.Author, momo.LoadPerParentLimit[models.Author](1))
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("belongs-to: want ErrUnsupported, got %v", err)
		}
		err = momo.Load(ctx, db, docs, document.Comments, momo.LoadPerParentLimit[models.Comment](-1))
		if !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("negative: want ErrValidation, got %v", err)
		}
		if err := momo.Load(ctx, db, authors, author.FirstDocument, momo.LoadPerParentLimit[models.Document](1)); !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("has-one: want ErrUnsupported, got %v", err)
		}
		if len(h.After) != 0 {
			t.Fatalf("rejections must not run statements: %v", h.After)
		}
		if err := momo.Load(ctx, db, docs, document.Comments, momo.LoadPerParentLimit[models.Comment](0)); err != nil {
			t.Fatal(err)
		}
		if len(docs[0].Comments) != 3 {
			t.Fatalf("limit 0 is unlimited: %v", ids(docs[0].Comments))
		}
	})
}
