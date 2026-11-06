// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/employee"
	"github.com/azrtydxb/momo/internal/relfixture/models"
)

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
