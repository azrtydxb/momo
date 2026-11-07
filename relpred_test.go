// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/comment"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/employee"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/region"
	"github.com/azrtydxb/momo/internal/relfixture/store"
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

		// HasFrom with the default qualifier is Has.
		for name, p := range map[string]momo.Predicate{
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

func TestRelationFromManyToManyIsValidation(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		q := document.Tags.From(momo.Select[models.Document, *models.Document](db))
		if _, err := q.SQL(); !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("SQL error = %v, want ErrValidation", err)
		}
		if _, err := q.All(context.Background()); !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("All error = %v, want ErrValidation", err)
		}
	})
}

func TestRelationPredicatesSkipSoftDeletedChildren(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		// document 1: one live and one deleted note; document 2: only a deleted
		// note; document 3: two live notes; document 4: none.
		for _, n := range []struct {
			id, doc int64
			del     bool
		}{{1, 1, false}, {2, 1, true}, {3, 2, true}, {4, 3, false}, {5, 3, false}} {
			if _, err := momo.Exec(ctx, db, `INSERT INTO notes (id, document_id, body, deleted) VALUES (?, ?, ?, ?)`, n.id, n.doc, "n", n.del); err != nil {
				t.Fatal(err)
			}
		}
		sel := func() momo.SelectQuery[models.Document] {
			return momo.Select[models.Document, *models.Document](db)
		}
		has, err := sel().Where(document.Notes.Has()).OrderBy(document.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(docIDs(has)) != "[1 3]" {
			t.Fatalf("Has must ignore soft-deleted notes: %v", docIDs(has))
		}
		hasNo, err := sel().Where(document.Notes.HasNo()).OrderBy(document.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(docIDs(hasNo)) != "[2 4]" {
			t.Fatalf("HasNo must treat only-deleted notes as none: %v", docIDs(hasNo))
		}
		ordered, err := sel().OrderBy(document.Notes.CountOrder(momo.DirDesc), document.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		// live counts: doc3=2, doc1=1, doc2=0, doc4=0 (deleted rows would make doc1 tie/lead).
		if fmt.Sprint(docIDs(ordered)) != "[3 1 2 4]" {
			t.Fatalf("CountOrder must not count soft-deleted notes: %v", docIDs(ordered))
		}
	})
}

func TestRelationFromPolymorphicDiscriminator(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		// Seed has image 1 (document 1) and image 2 (author 1, owner_id 1 collides
		// with document 1's id).
		parents := momo.Select[models.Document, *models.Document](db).Where(document.ID.Eq(1))
		imgs, err := document.Images.From(parents).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(imgs) != 1 || imgs[0].ID != 1 || imgs[0].OwnerType != "document" {
			t.Fatalf("From must keep only document-owned images: %+v", imgs)
		}
	})
}

func TestRelationFromSelfReferencing(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		ids := func(es []models.Employee) []int64 {
			out := []int64{}
			for _, e := range es {
				out = append(out, e.ID)
			}
			return out
		}
		boss := momo.Select[models.Employee, *models.Employee](db).Where(employee.ID.Eq(1))
		reports, err := employee.Reports.From(boss).OrderBy(employee.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(ids(reports)) != "[2 3]" {
			t.Fatalf("reports of the boss: %v", ids(reports))
		}
		dev := momo.Select[models.Employee, *models.Employee](db).Where(employee.ID.Eq(2))
		none, err := employee.Reports.From(dev).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(none) != 0 {
			t.Fatalf("dev1 has no reports: %v", ids(none))
		}
		// Reverse direction: Manager.From(reports) yields the boss.
		mgr, err := employee.Manager.From(momo.Select[models.Employee, *models.Employee](db).Where(employee.ID.Eq(2))).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(ids(mgr)) != "[1]" {
			t.Fatalf("manager of dev1: %v", ids(mgr))
		}
	})
}

// TestRelationPredicateShapes covers self-referencing, composite-key and
// polymorphic descriptors, which the basic tests do not reach.
func TestRelationPredicateShapes(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		// Self-reference: the outer table is aliased and passed to HasFrom.
		bosses := momo.Select[models.Employee, *models.Employee](db).
			From(momo.TableRef{Name: "employees", Alias: "e"}).
			Where(employee.Reports.HasFrom("e")).OrderBy(employee.ID.As("e").Asc())
		c, err := bosses.SQL()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(c.SQL, `"momo_r"."manager_id" = "e"."id"`) {
			t.Fatalf("self-reference must correlate on the outer alias: %s", c.SQL)
		}
		emps, err := bosses.All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(emps) != 1 || emps[0].ID != 1 {
			t.Fatalf("employees with reports: %+v", emps)
		}

		// Composite key: both columns correlate, so region (1,EU) matches only
		// its own store rows, not every store of tenant 1.
		regs, err := momo.Select[models.Region, *models.Region](db).
			Where(region.Stores.Has(store.ID.As("momo_r").Eq(2))).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(regs) != 1 || regs[0].Code != "US" {
			t.Fatalf("regions with store 2: %+v", regs)
		}
		regs, err = momo.Select[models.Region, *models.Region](db).
			Where(region.Stores.Has(store.ID.As("momo_r").Eq(3))).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(regs) != 0 {
			t.Fatalf("store 3 has a NULL region and belongs to no region: %+v", regs)
		}

		// Polymorphic: an author-owned image with owner_id 2 must not make
		// document 2 look like it has images.
		if _, err := momo.Exec(ctx, db, `INSERT INTO images (id, owner_type, owner_id, url) VALUES (?, ?, ?, ?)`, int64(3), "author", int64(2), "u3"); err != nil {
			t.Fatal(err)
		}
		withImages, err := momo.Select[models.Document, *models.Document](db).
			Where(document.Images.Has()).OrderBy(document.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(docIDs(withImages)) != "[1]" {
			t.Fatalf("documents with images: %v", docIDs(withImages))
		}

		// Traversal over a composite key.
		stores, err := region.Stores.From(momo.Select[models.Region, *models.Region](db).
			Where(region.Code.Eq("EU"))).OrderBy(store.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		ids := []int64{}
		for _, s := range stores {
			ids = append(ids, s.ID)
		}
		if fmt.Sprint(ids) != "[1 4]" {
			t.Fatalf("stores of EU regions: %v", ids)
		}
	})
}
