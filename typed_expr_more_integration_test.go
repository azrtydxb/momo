// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// Catches: any of lower, upper, length, concat, CAST and COALESCE over
// expressions rendering SQL the databases reject or computing the wrong value.
func TestStringCastCoalesceHelpers(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		for _, s := range []string{
			"INSERT INTO documents (tenant_id, title, body) VALUES (2, 'ABC', NULL)",
			"INSERT INTO documents (tenant_id, title, body) VALUES (3, 'ABC', 'bb')",
		} {
			if _, err := momo.Exec(ctx, db, s); err != nil {
				t.Fatal(err)
			}
		}
		docs := momo.TableRef{Name: "documents"}

		// lower, cast and sum grouped: both ABC rows fold into one.
		g, err := momo.Select[strCount](db).From(docs).
			Columns(momo.Lower(suite.DocTitle), momo.Cast[int64](momo.Sum(suite.DocTenantID), "bigint")).
			GroupBy(momo.Lower(suite.DocTitle)).All(ctx)
		if err != nil || len(g) != 1 || g[0].S != "abc" || g[0].N != 5 {
			t.Fatalf("lower + cast(sum) = %+v, %v", g, err)
		}

		// upper, concat and length.
		c, err := momo.Select[strCount](db).From(docs).
			Columns(momo.Concat(momo.Upper(suite.DocTitle), momo.Value("-x")), momo.Length(suite.DocTitle)).
			Where(suite.DocTenantID.Eq(2)).All(ctx)
		if err != nil || len(c) != 1 || c[0].S != "ABC-x" || c[0].N != 3 {
			t.Fatalf("concat + length = %+v, %v", c, err)
		}

		// coalesce over two columns: NULL body falls back to the title.
		co, err := momo.Select[strCount](db).From(docs).
			Columns(momo.CoalesceExpr(suite.DocBody, suite.DocTitle), suite.DocTenantID).
			OrderBy(suite.DocTenantID.Asc()).All(ctx)
		if err != nil || len(co) != 2 || co[0].S != "ABC" || co[1].S != "bb" {
			t.Fatalf("coalesce = %+v, %v", co, err)
		}
	})
}
