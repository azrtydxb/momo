// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestSelectNoHooksAllocBudget(t *testing.T) {
	db := testopen.Postgres(t)
	relfixture.CreateSchema(t, db)
	relfixture.SeedBasic(t, db)
	ctx := context.Background()
	q := momo.Select[models.Author, *models.Author](db).Where(author.ID.Eq(1))
	// Warm the statement cache so the first prepare is not counted.
	if _, err := q.One(ctx); err != nil {
		t.Fatal(err)
	}
	allocs := testing.AllocsPerRun(200, func() {
		if _, err := q.One(ctx); err != nil {
			t.Fatal(err)
		}
	})
	t.Logf("measured allocs/op = %v", allocs)
	if allocs > selectNoHooksAllocBudget {
		t.Fatalf("allocs/op %v exceeds budget %d: disabled hooks must add no allocation", allocs, selectNoHooksAllocBudget)
	}
}
