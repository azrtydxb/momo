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

// Catches: an aliased aggregate that the outer query cannot read by name,
// from a FROM subquery or from a CTE without positional column names.
func TestAliasedAggregateReferencedByName(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		seedTenants(t, db, 1, 2, 2, 3)

		n := momo.CountAll().As("n")
		inner := momo.Select[kvRow](db).From(momo.TableRef{Name: "documents"}).
			Columns(suite.DocTenantID, n).GroupBy(suite.DocTenantID)

		rows, err := momo.Select[kvRow](db).From(momo.FromSubquery(inner, "s")).
			Columns(pairK, n.Ref("s")).Where(n.Ref("s").Gt(1)).All(ctx)
		if err != nil || len(rows) != 1 || rows[0].K != 2 || rows[0].N != 2 {
			t.Fatalf("subquery filtered on alias = %+v, %v; want tenant 2 with 2", rows, err)
		}

		k := momo.ColumnOf[int64]("t", "tenant_id")
		cte := momo.Select[kvRow](db).With("t", inner).From(momo.CTE("t")).
			Columns(k, n.Ref("t")).OrderBy(n.Ref("t").Desc(), k.Asc())
		rows, err = cte.All(ctx)
		if err != nil || len(rows) != 3 || rows[0].K != 2 || rows[0].N != 2 || rows[2].K != 3 {
			t.Fatalf("cte ordered on alias = %+v, %v", rows, err)
		}

		top := momo.Max(suite.DocTenantID).As("top")
		maxRows, err := momo.Select[kvRow](db).
			From(momo.FromSubquery(momo.Select[kvRow](db).From(momo.TableRef{Name: "documents"}).
				Columns(top, n), "m")).
			Columns(top.Ref("m"), n.Ref("m")).All(ctx)
		if err != nil || len(maxRows) != 1 || maxRows[0].K != 3 || maxRows[0].N != 4 {
			t.Fatalf("max and count aliased together = %+v, %v", maxRows, err)
		}
	})
}
