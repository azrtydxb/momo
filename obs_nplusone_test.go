// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// nPlusOne runs fn on every dialect with a DB carrying the detector hook.
func nPlusOne(t *testing.T, threshold int, fn func(t *testing.T, db *momo.DB, found *[]momo.NPlusOneFinding)) {
	t.Helper()
	var found []momo.NPlusOneFinding
	hook := momo.NewNPlusOneHook(threshold, func(f momo.NPlusOneFinding) { found = append(found, f) })
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		obsSeed(t, db)
		found = nil
		fn(t, db, &found)
	}, momo.WithHook(hook))
}

func nPlusOneLoop(t *testing.T, ctx context.Context, db *momo.DB, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := momo.Select[models.Author](db).Where(author.ID.Eq(int64(1))).One(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNPlusOneHook(t *testing.T) {
	t.Run("a loop of single-row selects is reported once", func(t *testing.T) {
		nPlusOne(t, 3, func(t *testing.T, db *momo.DB, found *[]momo.NPlusOneFinding) {
			nPlusOneLoop(t, momo.WithQueryScope(context.Background()), db, 5)
			if len(*found) != 1 {
				t.Fatalf("want exactly one finding for 5 repeats at threshold 3, got %d", len(*found))
			}
			f := (*found)[0]
			if f.Count != 4 || f.Shape == "" || f.Example == "" {
				t.Fatalf("finding %+v, want Count 4 (reported when the threshold is first exceeded)", f)
			}
		})
	})

	t.Run("without a scope nothing is tracked", func(t *testing.T) {
		nPlusOne(t, 3, func(t *testing.T, db *momo.DB, found *[]momo.NPlusOneFinding) {
			nPlusOneLoop(t, context.Background(), db, 5)
			if len(*found) != 0 {
				t.Fatalf("unscoped context reported %d findings", len(*found))
			}
		})
	})

	t.Run("scopes are independent", func(t *testing.T) {
		nPlusOne(t, 3, func(t *testing.T, db *momo.DB, found *[]momo.NPlusOneFinding) {
			nPlusOneLoop(t, momo.WithQueryScope(context.Background()), db, 3)
			nPlusOneLoop(t, momo.WithQueryScope(context.Background()), db, 3)
			if len(*found) != 0 {
				t.Fatalf("3 repeats per scope at threshold 3 reported %d findings", len(*found))
			}
		})
	})

	t.Run("one batched relation load stays silent", func(t *testing.T) {
		nPlusOne(t, 1, func(t *testing.T, db *momo.DB, found *[]momo.NPlusOneFinding) {
			ctx := momo.WithQueryScope(context.Background())
			docs, err := momo.Select[models.Document](db).OrderBy(document.ID.Asc()).All(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := momo.Load(ctx, db, docs, document.Author); err != nil {
				t.Fatal(err)
			}
			if len(*found) != 0 {
				t.Fatalf("batched load reported %+v", *found)
			}
		})
	})
}
