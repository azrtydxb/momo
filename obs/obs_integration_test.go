// SPDX-License-Identifier: Apache-2.0

//go:build integration

package obs_test

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/obs"
)

func seededDocs(t *testing.T, db *momo.DB, ctx context.Context) []models.Document {
	t.Helper()
	relfixture.CreateSchema(t, db)
	relfixture.SeedBasic(t, db)
	docs, err := momo.Select[models.Document](db).OrderBy(document.ID.Asc()).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) < 2 {
		t.Fatalf("fixture has %d documents, need at least 2", len(docs))
	}
	return docs
}

func TestNPlusOneAgainstDatabase(t *testing.T) {
	var mu sync.Mutex
	var found []obs.Finding
	rec := &obs.Recorder{}
	det := obs.NewNPlusOne(1, func(f obs.Finding) {
		mu.Lock()
		found = append(found, f)
		mu.Unlock()
	})
	reset := func() {
		mu.Lock()
		found = nil
		mu.Unlock()
		rec.Reset()
	}
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		ctx := obs.WithScope(context.Background())
		docs := seededDocs(t, db, ctx)

		reset()
		if err := momo.Load(ctx, db, docs, document.Author); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		n := len(found)
		mu.Unlock()
		if n != 0 {
			t.Fatalf("one batched Load reported %+v", found)
		}
		if got := len(rec.Relations()); got != 1 {
			t.Fatalf("batched Load produced %d relation events, want 1", got)
		}

		reset()
		ctx = obs.WithScope(context.Background())
		for i := range docs {
			if err := momo.Load(ctx, db, docs[i:i+1], document.Author); err != nil {
				t.Fatal(err)
			}
		}
		mu.Lock()
		defer mu.Unlock()
		if len(found) != 1 || found[0].Relation != "documents.Author" || found[0].Parents != 1 {
			t.Fatalf("a Load per document must report documents.Author once, got %+v", found)
		}
		if !slices.ContainsFunc(rec.Events(), func(e obs.Event) bool { return e.Relation == "documents.Author" }) {
			t.Fatal("no recorded statement carries the relation name")
		}
	}, momo.WithHook(rec), momo.WithHook(det))
}
