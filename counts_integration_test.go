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
	"github.com/azrtydxb/momo/internal/relfixture/comment"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/region"
	"github.com/azrtydxb/momo/internal/relfixture/tag"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestLoadCounts(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		for _, s := range []string{
			`INSERT INTO notes (id, document_id, body, deleted) VALUES (1, 1, 'n1', FALSE)`,
			`INSERT INTO notes (id, document_id, body, deleted) VALUES (2, 1, 'n2', TRUE)`,
			`INSERT INTO notes (id, document_id, body, deleted) VALUES (3, 3, 'n3', FALSE)`,
		} {
			if _, err := momo.Exec(ctx, db, s); err != nil {
				t.Fatal(err)
			}
		}
		// Documents 1..4, a repeated document and one with no rows at all.
		docs := []models.Document{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 1}, {ID: 99}}

		check := func(name string, got []int64, err error, want string) {
			t.Helper()
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if fmt.Sprint(got) != want {
				t.Fatalf("%s: counts %v, want %s", name, got, want)
			}
		}

		h.After = nil
		got, err := momo.LoadCounts(ctx, db, docs, document.Comments)
		check("has-many", got, err, "[3 0 1 0 3 0]")
		if len(h.After) != 1 {
			t.Fatalf("one statement for all parents, got %d", len(h.After))
		}

		got, err = momo.LoadCounts(ctx, db, docs, document.Comments,
			momo.LoadWhere[models.Comment](comment.Approved.Eq(true)))
		check("filtered", got, err, "[2 0 1 0 2 0]")

		got, err = momo.LoadCounts(ctx, db, docs, document.Notes)
		check("soft-delete default", got, err, "[1 0 1 0 1 0]")
		got, err = momo.LoadCounts(ctx, db, docs, document.Notes, momo.LoadWithDeleted[models.Note]())
		check("with deleted", got, err, "[2 0 1 0 2 0]")
		got, err = momo.LoadCounts(ctx, db, docs, document.Notes, momo.LoadOnlyDeleted[models.Note]())
		check("only deleted", got, err, "[1 0 0 0 1 0]")

		got, err = momo.LoadCounts(ctx, db, docs, document.Tags)
		check("many-to-many", got, err, "[2 0 1 0 2 0]")
		got, err = momo.LoadCounts(ctx, db, docs, document.Tags,
			momo.LoadWhere[models.Tag](tag.Name.Eq("db")))
		check("many-to-many filtered", got, err, "[1 0 1 0 1 0]")

		regions := []models.Region{{TenantID: 1, Code: "EU"}, {TenantID: 1, Code: "US"}, {TenantID: 2, Code: "EU"}, {TenantID: 2, Code: "US"}}
		got, err = momo.LoadCounts(ctx, db, regions, region.Stores)
		check("composite key", got, err, "[1 1 1 0]")

		// Counts agree with the per-parent Count.
		for i := range docs[:4] {
			n, err := document.Comments.Count(ctx, db, &docs[i])
			if err != nil {
				t.Fatal(err)
			}
			one, err := momo.LoadCounts(ctx, db, docs[i:i+1], document.Comments)
			if err != nil || one[0] != n {
				t.Fatalf("document %d: LoadCounts %v (%v), Count %d", docs[i].ID, one, err, n)
			}
		}

		if _, err := momo.LoadCounts(ctx, db, docs, document.Author); err == nil {
			t.Fatal("belongs-to must be unsupported")
		}
		if _, err := momo.LoadCounts(ctx, db, docs, document.Comments, momo.LoadPerParentLimit[models.Comment](1)); err == nil {
			t.Fatal("a per-parent limit must be rejected")
		}
		if got, err = momo.LoadCounts(ctx, db, []models.Document{}, document.Comments); err != nil || len(got) != 0 {
			t.Fatalf("no parents: %v %v", got, err)
		}
	})
}

func TestLoadCountsChunks(t *testing.T) {
	h := &relfixture.Hook{}
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		relfixture.CreateSchema(t, db)
		relfixture.SeedBasic(t, db)
		var docs []models.Document
		for i := int64(1); i <= 40; i++ {
			docs = append(docs, models.Document{ID: i})
		}
		h.After = nil
		got, err := momo.LoadCounts(context.Background(), db, docs, document.Comments)
		if err != nil {
			t.Fatal(err)
		}
		if got[0] != 3 || got[2] != 1 || got[39] != 0 {
			t.Fatalf("chunked counts wrong: %v", got)
		}
		if len(h.After) < 2 {
			t.Fatalf("40 keys under 20 binds must chunk, got %d statements", len(h.After))
		}
	}, momo.WithHook(h), momo.WithMaxParams(16+4))
}
