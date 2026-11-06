// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/comment"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/employee"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/store"
)

func selectDocs(db *momo.DB) momo.SelectQuery[models.Document] {
	return momo.Select[models.Document, *models.Document](db)
}

func TestJoinLoadToOne(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		h.After = nil
		docs, err := selectDocs(db).OrderBy(document.ID.Asc()).Preload(document.Author.Joined()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 {
			t.Fatalf("join loading must be one statement, got %d", len(h.After))
		}
		if len(docs) != 4 {
			t.Fatalf("join must not duplicate or drop parents: %d", len(docs))
		}
		if docs[0].Author == nil || docs[0].Author.Name != "Ada" {
			t.Fatalf("doc 1 author: %+v", docs[0].Author)
		}
		if docs[2].Author == nil || docs[2].Author.ID != 2 || docs[2].Author.Name != "Grace" {
			t.Fatalf("doc 3 author: %+v", docs[2].Author)
		}
		if docs[3].Author != nil {
			t.Fatal("NULL foreign key through a LEFT JOIN must leave Author nil, not a zero struct")
		}
		if docs[0].Title != "A1" || docs[3].Title != "Orphan" {
			t.Fatalf("parent columns must still scan in place: %q %q", docs[0].Title, docs[3].Title)
		}
		// Parents sharing a target get their own copy.
		if docs[0].Author == docs[1].Author {
			t.Fatal("two parents must not share one child pointer")
		}
		if len(h.Events) != 0 {
			t.Fatalf("a joined load runs no statement of its own and emits no relation event: %+v", h.Events)
		}
	})
}

// A joined load must follow the parent query's Where, Order, Limit and Offset
// exactly: a to-one join cannot multiply parent rows.
func TestJoinLoadParentQueryClauses(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		h.After = nil
		docs, err := selectDocs(db).Where(document.ID.Gt(1)).OrderBy(document.ID.Desc()).Limit(2).
			Preload(document.Author.Joined()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 || len(docs) != 2 || docs[0].ID != 4 || docs[1].ID != 3 {
			t.Fatalf("limit/order/where: statements=%d docs=%+v", len(h.After), docs)
		}
		if docs[0].Author != nil || docs[1].Author == nil || docs[1].Author.Name != "Grace" {
			t.Fatalf("authors: %+v %+v", docs[0].Author, docs[1].Author)
		}
		page, err := selectDocs(db).OrderBy(document.ID.Asc()).Limit(2).Offset(1).Preload(document.Author.Joined()).All(ctx)
		if err != nil || len(page) != 2 || page[0].ID != 2 || page[1].ID != 3 || page[0].Author == nil {
			t.Fatalf("offset page: %v %+v", err, page)
		}
		doc, err := selectDocs(db).Where(document.ID.Eq(3)).Preload(document.Author.Joined()).One(ctx)
		if err != nil || doc.Author == nil || doc.Author.Name != "Grace" {
			t.Fatalf("One: %v %+v", err, doc)
		}
		orphan, err := selectDocs(db).Where(document.ID.Eq(4)).Preload(document.Author.Joined()).First(ctx)
		if err != nil || orphan.Author != nil {
			t.Fatalf("First on NULL FK: %v %+v", err, orphan)
		}
		if _, err := selectDocs(db).Where(document.ID.Eq(99)).Preload(document.Author.Joined()).One(ctx); !errors.Is(err, momo.ErrNotFound) {
			t.Fatalf("One with no row: %v", err)
		}
		if _, err := selectDocs(db).Preload(document.Author.Joined()).One(ctx); !errors.Is(err, momo.ErrTooManyRows) {
			t.Fatalf("One with several rows: %v", err)
		}
	})
}

// Every column of a self-referencing join collides by name with the parent's;
// the alias keeps the two sides apart.
func TestJoinLoadSelfReference(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		h.After = nil
		emps, err := momo.Select[models.Employee, *models.Employee](db).OrderBy(employee.ID.Asc()).
			Preload(employee.Manager.Joined()).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 || len(emps) != 3 {
			t.Fatalf("statements=%d employees=%d", len(h.After), len(emps))
		}
		if emps[0].Manager != nil {
			t.Fatalf("boss has no manager: %+v", emps[0].Manager)
		}
		for _, e := range emps[1:] {
			if e.Manager == nil || e.Manager.ID != 1 || e.Manager.Name != "boss" || e.Name == "boss" {
				t.Fatalf("%s: manager %+v", e.Name, e.Manager)
			}
		}
	})
}

// A composite key joins on every column, and a NULL component matches nothing.
func TestJoinLoadComposite(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		h.After = nil
		stores, err := momo.Select[models.Store, *models.Store](db).OrderBy(store.ID.Asc()).
			Preload(store.Region.Joined()).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 || len(stores) != 4 {
			t.Fatalf("statements=%d stores=%d", len(h.After), len(stores))
		}
		want := []string{"Europe", "America", "", "Europe2"}
		for i, s := range stores {
			switch {
			case want[i] == "" && s.Region != nil:
				t.Fatalf("store %d has a NULL key component: %+v", s.ID, s.Region)
			case want[i] != "" && (s.Region == nil || s.Region.Name != want[i]):
				t.Fatalf("store %d region: %+v want %s", s.ID, s.Region, want[i])
			}
		}
	})
}

// A soft-deleted target is excluded in the ON clause, so the parent row
// survives with a nil relation; a WHERE would drop the parent.
func TestJoinLoadSoftDeletedTargetKeepsParent(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		mustExec(t, db, "INSERT INTO notes (id, document_id, body, deleted) VALUES (1, 1, 'live', FALSE), (3, 3, 'gone', TRUE)")
		notes := map[int64]*models.Note{}
		rel := momo.Relation[models.Comment, models.Note]{
			Name: "Note", Kind: momo.BelongsTo,
			Parent: momo.TableRef{Name: "comments"}, Child: momo.TableRef{Name: "notes"},
			ParentMeta: (*models.Comment)(nil).MomoMeta(), ChildMeta: (*models.Note)(nil).MomoMeta(),
			ParentCols: []string{"document_id"}, ChildCols: []string{"id"}, ChildPK: []string{"id"},
			ParentKey: func(p *models.Comment) ([]any, bool) { return []any{p.DocumentID}, true },
			ChildKey:  func(c *models.Note) ([]any, bool) { return []any{c.ID}, true },
			GetOne:    func(p *models.Comment) *models.Note { return notes[p.ID] },
			SetOne:    func(p *models.Comment, c *models.Note) { notes[p.ID] = c },
		}
		// Comments 1-3 point at note 1 (live); comment 4 points at note 3 (soft-deleted).
		h.After = nil
		cs, err := momo.Select[models.Comment, *models.Comment](db).OrderBy(comment.ID.Asc()).
			Preload(rel.Joined()).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 || len(cs) != 4 {
			t.Fatalf("a soft-deleted target must not drop the parent: statements=%d parents=%d", len(h.After), len(cs))
		}
		for _, id := range []int64{1, 2, 3} {
			if n := notes[id]; n == nil || n.ID != 1 || n.Body != "live" {
				t.Fatalf("comment %d note: %+v", id, n)
			}
		}
		if notes[4] != nil {
			t.Fatalf("comment 4 targets the soft-deleted note 3: %+v", notes[4])
		}
	})
}

// Joined loaders compose with ordinary preloads, which still run as separate
// statements after the parent query, and several joins use distinct aliases.
func TestJoinLoadWithOtherPreloads(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		h.After = nil
		docs, err := selectDocs(db).OrderBy(document.ID.Asc()).
			Preload(document.Author.Joined(), document.Comments, document.Author.Joined()).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 2 {
			t.Fatalf("one joined statement plus one comments statement, got %d", len(h.After))
		}
		if docs[0].Author == nil || len(docs[0].Comments) != 3 || docs[3].Author != nil {
			t.Fatalf("doc 1: %+v", docs[0])
		}
		q := selectDocs(db).Preload(document.Author.Joined(), document.Author.Joined())
		c, err := q.SQL()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(c.SQL, `AS "momo_j0"`) || !strings.Contains(c.SQL, `AS "momo_j1"`) {
			t.Fatalf("each joined loader needs its own alias: %s", c.SQL)
		}
	})
}

// The SQL puts the soft-delete filter and the key match in ON, never in WHERE.
func TestJoinLoadSQLShape(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, _ *relfixture.Hook) {
		rel := momo.Relation[models.Comment, models.Note]{
			Name: "Note", Kind: momo.BelongsTo,
			Parent: momo.TableRef{Name: "comments"}, Child: momo.TableRef{Name: "notes"},
			ParentMeta: (*models.Comment)(nil).MomoMeta(), ChildMeta: (*models.Note)(nil).MomoMeta(),
			ParentCols: []string{"document_id"}, ChildCols: []string{"id"}, ChildPK: []string{"id"},
			ParentKey: func(p *models.Comment) ([]any, bool) { return []any{p.DocumentID}, true },
			ChildKey:  func(c *models.Note) ([]any, bool) { return []any{c.ID}, true },
			GetOne:    func(*models.Comment) *models.Note { return nil },
			SetOne:    func(*models.Comment, *models.Note) {},
		}
		c, err := momo.Select[models.Comment, *models.Comment](db).Where(comment.Approved.Eq(true)).Preload(rel.Joined()).SQL()
		if err != nil {
			t.Fatal(err)
		}
		on := strings.Index(c.SQL, " ON ")
		where := strings.Index(c.SQL, " WHERE ")
		if on < 0 || where < on {
			t.Fatalf("missing ON before WHERE: %s", c.SQL)
		}
		onClause, whereClause := c.SQL[on:where], c.SQL[where:]
		if !strings.Contains(onClause, `"momo_j0"."deleted" = FALSE`) || !strings.Contains(onClause, `"momo_j0"."id" = "comments"."document_id"`) {
			t.Fatalf("ON clause: %s", onClause)
		}
		if strings.Contains(whereClause, "momo_j0") {
			t.Fatalf("the target's soft-delete filter must not be in WHERE: %s", whereClause)
		}
		if !strings.Contains(c.SQL, `LEFT JOIN "notes" AS "momo_j0"`) {
			t.Fatalf("LEFT JOIN: %s", c.SQL)
		}
	})
}

func TestJoinLoadUnsupported(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, _ *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		for name, q := range map[string]momo.SelectQuery[models.Document]{
			"has-many":     selectDocs(db).Preload(document.Comments.Joined()),
			"many-to-many": selectDocs(db).Preload(document.Tags.Joined()),
			"group by":     selectDocs(db).Preload(document.Author.Joined()).GroupBy(document.ID),
			"for update":   selectDocs(db).Preload(document.Author.Joined()).ForUpdate(),
		} {
			if _, err := q.All(ctx); !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("%s: All error %v, want ErrUnsupported", name, err)
			}
		}
		q := selectDocs(db).Preload(document.Author.Joined())
		if err := q.Each(ctx, func(*models.Document) error { return nil }); !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("Each: %v", err)
		}
		for _, err := range q.Iter(ctx) {
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("Iter: %v", err)
			}
		}
		// Load and LoadAll do not take joined loaders.
		docs := allDocs(t, db)
		if err := momo.LoadAll(ctx, db, docs, document.Author.Joined()); !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("LoadAll with a joined loader: %v", err)
		}
		// A user-supplied select list of the wrong width is still a validation error.
		if _, err := selectDocs(db).Columns(document.ID).Preload(document.Author.Joined()).All(ctx); !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("short select list: %v", err)
		}
	})
}

// The base query is not changed by Preload.
func TestJoinLoadPreloadIsImmutable(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, _ *relfixture.Hook) {
		base := selectDocs(db).OrderBy(document.ID.Asc())
		_ = base.Preload(document.Author.Joined())
		c, err := base.SQL()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(c.SQL, "JOIN") {
			t.Fatalf("Preload leaked into its receiver: %s", c.SQL)
		}
	})
}
