// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/store"
)

// TestEagerLoadLargeCompositeKeySet loads a belongs-to over a composite key
// with far more distinct keys than PostgreSQL's parser accepts in one row-value
// IN list (about 8000 two-column tuples fail with SQLSTATE 54001), yet well
// inside the 65535 bind limit. The load must chunk at the composite cap and
// still assign every match, in parent order.
func TestEagerLoadLargeCompositeKeySet(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		const n = 20000
		stores := make([]models.Store, n)
		for i := range stores {
			code := fmt.Sprintf("X%d", i)
			stores[i] = models.Store{ID: int64(i + 1), TenantID: 1, RegionCode: &code}
		}
		eu, us := "EU", "US"
		stores[0].RegionCode = &eu
		stores[n-1].RegionCode = &us
		stores[7].RegionCode = nil // NULL component: never queried

		h.After, h.Events = nil, nil
		if err := momo.Load(context.Background(), db, stores, store.Region); err != nil {
			t.Fatalf("load: %v", err)
		}
		if stores[0].Region == nil || stores[0].Region.Name != "Europe" {
			t.Fatalf("first store: %+v", stores[0].Region)
		}
		if stores[n-1].Region == nil || stores[n-1].Region.Name != "America" {
			t.Fatalf("last store: %+v", stores[n-1].Region)
		}
		for i := 1; i < n-1; i++ {
			if stores[i].Region != nil {
				t.Fatalf("store %d has no region, got %+v", i, stores[i].Region)
			}
		}
		// n-1 distinct non-nil keys, 2000 per statement.
		want := (n - 1 + 1999) / 2000
		if len(h.After) != want {
			t.Fatalf("want %d statements, got %d", want, len(h.After))
		}
		if len(h.Events) != 1 || h.Events[0].Queries != want || h.Events[0].Children != 2 {
			t.Fatalf("event: %+v", h.Events)
		}
	})
}
