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
)

// BenchmarkSelectNoHooks measures a primary-key lookup on a DB that has no
// hooks registered. Its allocs/op is the budget enforced by
// TestSelectNoHooksAllocBudget.
func BenchmarkSelectNoHooks(b *testing.B) {
	db := momo.New(relfixture.PostgresDriver(b))
	relfixture.CreateSchema(b, db)
	relfixture.SeedBasic(b, db)
	ctx := context.Background()
	q := momo.Select[models.Author, *models.Author](db).Where(author.ID.Eq(1))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := q.One(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
