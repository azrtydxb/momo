// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/employee"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/tagging"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// TestNestedLoadErrorPropagates checks that a failure at the nested level
// (here an unsupported option, rejected when the nested loader is prepared)
// is returned from LoadAll rather than swallowed.
func TestNestedLoadErrorPropagates(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		docs := allDocs(t, db)

		err := momo.LoadAll(context.Background(), db, docs,
			document.Author.Then(
				author.FirstDocument.Scope(momo.LoadPerParentLimit[models.Document](1)),
			))
		var me *momo.Error
		if !errors.As(err, &me) || me.Kind != momo.ErrUnsupported {
			t.Fatalf("nested-level error must propagate as ErrUnsupported, got %v", err)
		}
		// The failure aborts the load: the level above must not be assigned.
		if docs[0].Author != nil {
			t.Fatalf("a failed nested load must not assign the parent level: %+v", docs[0].Author)
		}
	})
}

// TestNestedLoadHonoursContext checks that the caller's context reaches the
// nested level: cancelling it as the nested relation starts surfaces
// context.Canceled instead of running the nested statement.
func TestNestedLoadHonoursContext(t *testing.T) {
	h := &cancelHook{Hook: &relfixture.Hook{}, name: "FirstDocument"}
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		relfixture.CreateSchema(t, db)
		relfixture.SeedBasic(t, db)
		docs := allDocs(t, db)
		h.After = nil

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		h.cancel = cancel
		err := momo.LoadAll(ctx, db, docs, document.Author.Then(author.FirstDocument))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled from the nested level, got %v", err)
		}
		for _, r := range h.After {
			if r == "authors.FirstDocument" {
				t.Fatalf("nested statement must not run after cancellation: %v", h.After)
			}
		}
	}, momo.WithHook(h))
}

// TestNestedLoadSkipsEmptyIntermediate checks that when the level above loaded
// no children the nested level issues no statement at all.
func TestNestedLoadSkipsEmptyIntermediate(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		all := allDocs(t, db)
		// Documents 2 and 4 have no taggings.
		docs := []models.Document{all[1], all[3]}
		h.After = nil

		if err := momo.LoadAll(context.Background(), db, docs, document.Taggings.Then(tagging.Tag)); err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 {
			t.Fatalf("want only the top-level statement, got %d: %v", len(h.After), h.After)
		}
		if len(docs[0].Taggings) != 0 || len(docs[1].Taggings) != 0 {
			t.Fatalf("taggings must be empty: %+v", docs)
		}
	})
}

// TestNestedLoadSiblings loads two nested relations under one parent: each
// runs once over the whole pool of children.
func TestNestedLoadSiblings(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		docs := allDocs(t, db)
		h.After = nil

		err := momo.LoadAll(context.Background(), db, docs,
			document.Author.Then(author.Images, author.FirstDocument))
		if err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 3 {
			t.Fatalf("want authors + images + first document = 3 statements, got %d: %v", len(h.After), h.After)
		}
		a := docs[0].Author
		if a == nil || len(a.Images) != 1 || a.Images[0].ID != 2 {
			t.Fatalf("sibling one (images) missing: %+v", a)
		}
		if a.FirstDocument == nil || a.FirstDocument.ID != 1 {
			t.Fatalf("sibling two (first document) missing: %+v", a)
		}
		// Author 2 has no images but still gets its first document.
		if b := docs[2].Author; b == nil || len(b.Images) != 0 || b.FirstDocument == nil {
			t.Fatalf("author 2 siblings wrong: %+v", b)
		}
	})
}

// TestNestedLoadManyToMany loads a many-to-many relation at a nested level.
func TestNestedLoadManyToMany(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		docs := allDocs(t, db)
		h.After = nil

		err := momo.LoadAll(context.Background(), db, docs,
			document.Author.Then(author.FirstDocument.Then(document.Tags)))
		if err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 3 {
			t.Fatalf("want one statement per level (3), got %d: %v", len(h.After), h.After)
		}
		var names []string
		for _, tg := range docs[0].Author.FirstDocument.Tags {
			names = append(names, tg.Name)
		}
		sort.Strings(names)
		if fmt.Sprint(names) != "[db go]" {
			t.Fatalf("nested many-to-many tags: got %v want [db go]", names)
		}
		// Author 2's first document has no tags.
		if fd := docs[2].Author.FirstDocument; fd == nil || len(fd.Tags) != 0 {
			t.Fatalf("author 2 first document tags wrong: %+v", fd)
		}
	})
}

// TestNestedLoadNullKeyAtIntermediate checks a NULL foreign key on a child of
// the level above: it gets an empty nested association, with no error and no
// nested statement (the boss, with a NULL manager, is the only child).
func TestNestedLoadNullKeyAtIntermediate(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		emps, err := momo.Select[models.Employee, *models.Employee](db).OrderBy(employee.ID.Asc()).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		h.After = nil

		if err := momo.LoadAll(context.Background(), db, emps, employee.Manager.Then(employee.Manager)); err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 {
			t.Fatalf("a NULL key at the intermediate level must not query: got %d: %v", len(h.After), h.After)
		}
		if emps[0].Manager != nil {
			t.Fatalf("the boss has no manager: %+v", emps[0].Manager)
		}
		for _, i := range []int{1, 2} {
			m := emps[i].Manager
			if m == nil || m.ID != 1 || m.Manager != nil {
				t.Fatalf("emps[%d] manager chain wrong: %+v", i, m)
			}
		}
	})
}

// noteDocument is a test-local belongs-to from the soft-deletable Note back to
// its Document. Parent keys read are recorded so a test can see which
// intermediates the nested level used as parents.
func noteDocument(seen *[]int64) momo.Relation[models.Note, models.Document] {
	return momo.Relation[models.Note, models.Document]{
		Name: "Document", Kind: momo.BelongsTo,
		Parent: momo.TableRef{Name: "notes"}, Child: momo.TableRef{Name: "documents"},
		ParentMeta: (*models.Note)(nil).MomoMeta(), ChildMeta: (*models.Document)(nil).MomoMeta(),
		ParentCols: []string{"document_id"}, ChildCols: []string{"id"}, ChildPK: []string{"id"},
		ParentKey: func(p *models.Note) ([]any, bool) {
			*seen = append(*seen, p.ID)
			return []any{p.DocumentID}, true
		},
		ChildKey: func(c *models.Document) ([]any, bool) { return []any{c.ID}, true },
		SetParentKey: func(p *models.Note, k []any) {
			p.DocumentID = k[0].(int64)
		},
		GetOne: func(*models.Note) *models.Document { return nil },
		SetOne: func(*models.Note, *models.Document) {},
	}
}

// TestNestedLoadSoftDeletedIntermediate checks that soft-deleted children at an
// intermediate level are not loaded and so never become parents of the nested
// level.
func TestNestedLoadSoftDeletedIntermediate(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		for _, s := range []struct {
			id, doc int64
			del     bool
		}{{1, 1, false}, {2, 3, true}} {
			if _, err := momo.Exec(ctx, db, `INSERT INTO notes (id, document_id, body, deleted) VALUES (?, ?, ?, ?)`,
				s.id, s.doc, "n", s.del); err != nil {
				t.Fatal(err)
			}
		}

		var seen []int64
		docs := allDocs(t, db)
		h.After = nil
		if err := momo.LoadAll(ctx, db, docs, document.Notes.Then(noteDocument(&seen))); err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(seen) != "[1]" {
			t.Fatalf("only the live note may be a nested parent, got notes %v", seen)
		}
		if len(docs[0].Notes) != 1 || len(docs[2].Notes) != 0 {
			t.Fatalf("soft-deleted note must not load: %+v / %+v", docs[0].Notes, docs[2].Notes)
		}

		// Document 3 has only a deleted note: no intermediates, so no nested statement.
		only3 := []models.Document{docs[2]}
		seen, h.After = nil, nil
		if err := momo.LoadAll(ctx, db, only3, document.Notes.Then(noteDocument(&seen))); err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 || len(seen) != 0 {
			t.Fatalf("want 1 statement and no nested parents, got %v / %v", h.After, seen)
		}
	})
}
