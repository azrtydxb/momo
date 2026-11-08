// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
)

// Catches: a write path reporting a different interceptor kind than the one
// documented on Operation.Kind, which would let a tenant rule miss it.
func TestInterceptorKindOfEveryWritePath(t *testing.T) {
	ctx := t.Context()
	doc := func() *suite.Document { return &suite.Document{TenantID: 1, Title: "t"} }
	src := momo.RawSubquery("SELECT 3 AS id, 1 AS tenant_id, 'merged' AS title")
	on := suite.DocID.EqExpr(momo.TypedSQL[int64]("src.id"))
	cols := []momo.ColumnRef{suite.DocID, suite.DocTenantID, suite.DocTitle}
	vals := []momo.Expr{momo.TypedSQL[int64]("src.id"), momo.TypedSQL[int64]("src.tenant_id"), momo.TypedSQL[string]("src.title")}
	cases := []struct {
		name, want string
		run        func(db *momo.DB) error
	}{
		{"insert", "insert", func(db *momo.DB) error {
			_, err := momo.Insert[suite.Document](db).Model(doc()).Exec(ctx)
			return err
		}},
		{"upsert", "upsert", func(db *momo.DB) error {
			_, err := momo.Upsert[suite.Document](db).Model(doc()).Exec(ctx)
			return err
		}},
		{"update", "update", func(db *momo.DB) error {
			_, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("x")).Where(suite.DocID.Eq(1)).Exec(ctx)
			return err
		}},
		{"bulk update", "update", func(db *momo.DB) error {
			_, err := bulkQ(db, []any{int64(2), "new"}).Exec(ctx)
			return err
		}},
		{"soft delete", "delete", func(db *momo.DB) error {
			_, err := momo.SoftDelete[suite.Document](db).Where(suite.DocID.Eq(1)).Exec(ctx)
			return err
		}},
		{"delete", "delete", func(db *momo.DB) error {
			_, err := momo.ForceDelete[suite.Document](db).Where(suite.DocID.Eq(1)).Exec(ctx)
			return err
		}},
		{"merge insert", "insert", func(db *momo.DB) error {
			_, err := momo.Merge[suite.Document](db, src).On(on).WhenNotMatchedInsert(cols, vals...).Exec(ctx)
			return err
		}},
		{"merge upsert", "upsert", func(db *momo.DB) error {
			_, err := momo.Merge[suite.Document](db, src).On(on).WhenMatchedUpdate(suite.DocTitle.Set("x")).
				WhenNotMatchedInsert(cols, vals...).Exec(ctx)
			return err
		}},
		{"truncate", "truncate", func(db *momo.DB) error { return momo.Truncate[suite.Document](ctx, db) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var kinds []string
			db := momo.New(newVersioned(170000), momo.WithInterceptor(kindRecorder(&kinds)))
			if err := c.run(db); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(kinds, []string{c.want}) {
				t.Fatalf("interceptor saw %v, want [%s]", kinds, c.want)
			}
		})
	}
	t.Run("copy", func(t *testing.T) {
		var kinds []string
		db := momo.New(newCopyDriver(dialect.Postgres), momo.WithInterceptor(func(_ context.Context, op *momo.Operation) error {
			kinds = append(kinds, op.Kind)
			return nil
		}))
		if _, err := copyQuery(db).Copy(ctx, copyDocs()); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(kinds, []string{"insert"}) {
			t.Fatalf("interceptor saw %v, want [insert]", kinds)
		}
	})
}
