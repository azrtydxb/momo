// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/document"
)

func TestRelationEventsReported(t *testing.T) {
	ctx := context.Background()
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		docs := obsDocs(t, db)
		h.After = nil
		h.Events = nil

		if err := momo.Load(ctx, db, docs, document.Author); err != nil {
			t.Fatal(err)
		}
		if err := momo.Load(ctx, db, docs, document.Comments); err != nil {
			t.Fatal(err)
		}
		if len(h.Events) != 2 {
			t.Fatalf("want 2 relation events, got %d", len(h.Events))
		}
		a, c := h.Events[0], h.Events[1]
		if a.Parent != "documents" || a.Name != "Author" || a.Kind != momo.BelongsTo || a.Parents != 4 || a.Children != 2 || a.Queries != 1 {
			t.Fatalf("Author event %+v", a)
		}
		if c.Parent != "documents" || c.Name != "Comments" || c.Kind != momo.HasMany || c.Parents != 4 || c.Children != 4 || c.Queries != 1 {
			t.Fatalf("Comments event %+v", c)
		}
		if a.Duration <= 0 {
			t.Fatalf("Duration = %v, want > 0", a.Duration)
		}
		if len(h.After) != 2 {
			t.Fatalf("want 2 child statements, got %d: %q", len(h.After), h.After)
		}
	})
}
