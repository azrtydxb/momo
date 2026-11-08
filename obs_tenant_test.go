// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/store"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestTenantFilter(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		obsSeed(t, db)
		stores := func(ctx context.Context) momo.SelectQuery[models.Store] {
			return momo.Select[models.Store](db).Where(momo.TenantFilter(ctx, store.TenantID))
		}

		t.Run("scoped to the tenant in the context", func(t *testing.T) {
			ctx := momo.WithTenant(context.Background(), int64(1))
			c, err := stores(ctx).SQL()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(c.SQL, `"stores"."tenant_id" =`) || len(c.Args) != 1 || c.Args[0] != int64(1) {
				t.Fatalf("SQL %q args %v", c.SQL, c.Args)
			}
			rows, err := stores(ctx).All(ctx)
			if err != nil || len(rows) != 3 {
				t.Fatalf("tenant 1 stores = %d err %v, want 3", len(rows), err)
			}
		})

		t.Run("fails the build without a tenant or with the wrong type", func(t *testing.T) {
			for name, ctx := range map[string]context.Context{
				"absent":     context.Background(),
				"wrong type": momo.WithTenant(context.Background(), 1), // int, column is int64
				"nil":        momo.WithTenant(context.Background(), nil),
			} {
				wrap := map[string]func(momo.Predicate) momo.Predicate{
					"plain": func(p momo.Predicate) momo.Predicate { return p },
					"not":   momo.Not,
					"or": func(p momo.Predicate) momo.Predicate {
						return momo.Or(p, momo.SQL("1 = 1"))
					},
				}
				for wname, w := range wrap {
					q := momo.Select[models.Store](db).Where(w(momo.TenantFilter(ctx, store.TenantID)))
					if _, err := q.SQL(); !errors.Is(err, momo.ErrValidation) {
						t.Fatalf("%s/%s: SQL err = %v, want ErrValidation", name, wname, err)
					}
					if rows, err := q.All(ctx); !errors.Is(err, momo.ErrValidation) || len(rows) != 0 {
						t.Fatalf("%s/%s: %d rows err %v, want ErrValidation and none", name, wname, len(rows), err)
					}
				}
			}
		})

		t.Run("the zero tenant is a real tenant", func(t *testing.T) {
			ctx := momo.WithTenant(context.Background(), int64(0))
			rows, err := stores(ctx).All(ctx)
			if err != nil || len(rows) != 0 {
				t.Fatalf("tenant 0: %d rows err %v, want none and no error", len(rows), err)
			}
		})

		t.Run("is not a security boundary", func(t *testing.T) {
			ctx := momo.WithTenant(context.Background(), int64(1))
			all, err := momo.Select[models.Store](db).All(ctx)
			if err != nil || len(all) != 4 {
				t.Fatalf("unfiltered query returned %d rows err %v, want all 4 tenants' stores", len(all), err)
			}
			n, err := momo.Select[models.Store](db).Where(momo.SQL("1 = 1")).Count(ctx)
			if err != nil || n != 4 {
				t.Fatalf("raw predicate count %d err %v, want 4", n, err)
			}
		})
	})
}
