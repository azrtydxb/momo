// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
)

func TestPreloadOnQuery(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		base := momo.Select[models.Document, *models.Document](db).OrderBy(document.ID.Asc())
		derived := base.Preload(document.Author, document.Comments)
		// The base query is immutable: it must not gain preloads.
		h.After = nil
		docs, err := base.All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if docs[0].Author != nil || len(h.After) != 1 {
			t.Fatalf("base query was mutated or preloaded: author=%v statements=%d", docs[0].Author, len(h.After))
		}
		h.After = nil
		docs, err = derived.All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if docs[0].Author == nil || docs[0].Author.Name != "Ada" || len(docs[0].Comments) != 3 {
			t.Fatalf("preloads not applied: %+v", docs[0])
		}
		if len(h.After) != 3 {
			t.Fatalf("want parent + 2 relation statements, got %d: %v", len(h.After), h.After)
		}
	})
}

// Two queries derived from one base must not share the preload list: the
// second Preload must not overwrite or leak into the first.
func TestPreloadDerivedQueriesAreIndependent(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		base := momo.Select[models.Document, *models.Document](db).
			OrderBy(document.ID.Asc()).Preload(document.Author).Preload(document.Comments).Preload(document.Notes)
		// len 3 with spare capacity: an unclipped append would alias.
		withTags := base.Preload(document.Taggings)
		withImages := base.Preload(document.Images)
		a, err := withTags.All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		b, err := withImages.All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(a[0].Taggings) == 0 || len(a[0].Images) != 0 {
			t.Fatalf("withTags: taggings=%d images=%d", len(a[0].Taggings), len(a[0].Images))
		}
		if len(b[0].Images) == 0 || len(b[0].Taggings) != 0 {
			t.Fatalf("withImages: taggings=%d images=%d", len(b[0].Taggings), len(b[0].Images))
		}
	})
}

func TestPreloadFirstOneAndEmpty(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		q := momo.Select[models.Document, *models.Document](db).
			OrderBy(document.ID.Asc()).Preload(document.Author, document.Comments)
		h.After = nil
		d, err := q.First(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if d.Author == nil || d.Author.Name != "Ada" || len(d.Comments) != 3 {
			t.Fatalf("First did not preload: %+v", d)
		}
		if len(h.After) != 3 {
			t.Fatalf("First: want 3 statements, got %v", h.After)
		}
		d, err = q.Where(document.ID.Eq(d.ID)).One(ctx)
		if err != nil || len(d.Comments) != 3 {
			t.Fatalf("One: %v %+v", err, d)
		}

		// No parents: no relation statements.
		h.After = nil
		none, err := q.Where(document.ID.Eq(int64(-1))).All(ctx)
		if err != nil || len(none) != 0 {
			t.Fatalf("empty: %v %v", none, err)
		}
		if len(h.After) != 1 {
			t.Fatalf("empty result must issue only the parent statement, got %v", h.After)
		}
		if _, err := q.Where(document.ID.Eq(int64(-1))).One(ctx); !errors.Is(err, momo.ErrNotFound) {
			t.Fatalf("One on empty: %v", err)
		}
		if len(h.After) != 2 {
			t.Fatalf("One on empty must not preload, got %v", h.After)
		}
	})
}

func TestPreloadInTransactionAndEvents(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			h.After, h.Events = nil, nil
			docs, err := momo.Select[models.Document, *models.Document](tx).
				OrderBy(document.ID.Asc()).Preload(document.Comments).All(ctx)
			if err != nil {
				return err
			}
			if len(docs[0].Comments) != 3 {
				t.Errorf("tx preload: %d comments", len(docs[0].Comments))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(h.Events) != 1 || h.Events[0].Name != "Comments" || h.Events[0].Err != nil {
			t.Fatalf("relation events: %+v", h.Events)
		}
	})
}

func TestPreloadErrorPropagates(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		// A scope naming another table fails the relation statement's build.
		bad := document.Comments.Scope(momo.LoadWhere[models.Comment](document.Title.Eq("x")))
		q := momo.Select[models.Document, *models.Document](db).Preload(bad)
		if docs, err := q.All(ctx); err == nil || docs != nil {
			t.Fatalf("All: want error and nil rows, got %v, %v", docs, err)
		}
		if _, err := q.First(ctx); err == nil {
			t.Fatal("First: want error")
		}
	})
}

func TestPreloadStreamingUnsupported(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		q := momo.Select[models.Document, *models.Document](db).Preload(document.Author)
		h.After = nil
		err := q.Each(ctx, func(*models.Document) error { return nil })
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("Each: want ErrUnsupported, got %v", err)
		}
		seen := 0
		for _, err := range q.Iter(ctx) {
			seen++
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("Iter: want ErrUnsupported, got %v", err)
			}
		}
		if seen != 1 {
			t.Fatalf("Iter must yield one error, got %d elements", seen)
		}
		if len(h.After) != 0 {
			t.Fatalf("streaming with preloads must not run statements, got %v", h.After)
		}
		// Without preloads streaming still works.
		n := 0
		plain := momo.Select[models.Document, *models.Document](db)
		if err := plain.Each(ctx, func(*models.Document) error { n++; return nil }); err != nil || n == 0 {
			t.Fatalf("plain Each: %d %v", n, err)
		}
	})
}
