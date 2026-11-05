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

type intRow struct {
	momo.Projection
	N int64
}

func (*intRow) MomoMeta() *momo.Meta[intRow] {
	return &momo.Meta[intRow]{
		Columns: []momo.ColumnMeta{{Name: "n"}},
		Ptr:     func(m *intRow, _ int) any { return &m.N },
		Value:   func(m *intRow, _ int) any { return nil },
	}
}

func seedTenants(t *testing.T, db *momo.DB, tenants ...int64) {
	t.Helper()
	for _, tenant := range tenants {
		if _, err := momo.Exec(context.Background(), db, "INSERT INTO documents (tenant_id, title) VALUES (?, 'x')", tenant); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSubqueriesCTEsRecursive(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		seedTenants(t, db, 1, 2, 3)

		docs := momo.TableRef{Name: "documents"}
		sub := momo.Select[intRow](db).From(docs).Columns(suite.DocTenantID).Where(suite.DocTenantID.Ge(2))
		n, err := momo.Select[suite.Document](db).Where(suite.DocTenantID.InQuery(sub)).Count(ctx)
		if err != nil || n != 2 {
			t.Fatalf("IN subquery count = %d, %v; want 2", n, err)
		}

		// A recursive CTE that references itself, selected from the main query.
		seqN := momo.ColumnOf[int64]("seq", "n")
		body := momo.Union(true,
			momo.RawSubquery(`SELECT 1 AS n`),
			momo.Select[intRow](db).From(momo.CTE("seq")).
				Columns(momo.TypedSQL[int64](`"seq"."n" + 1`)).Where(seqN.Lt(5)),
		)
		rows, err := momo.Select[intRow](db).
			WithRecursive("seq", body, "n").
			From(momo.CTE("seq")).
			Columns(seqN).
			OrderBy(seqN.Asc()).
			All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 5 || rows[0].N != 1 || rows[4].N != 5 {
			t.Fatalf("recursive CTE returned %+v", rows)
		}

		// The raw-SQL path scans the same recursive CTE through Scalars.
		ints, err := momo.Scalars[int64](ctx, db, `WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < 5) SELECT n FROM seq ORDER BY n`)
		if err != nil || len(ints) != 5 || ints[4] != 5 {
			t.Fatalf("Scalars over recursive CTE = %v, %v", ints, err)
		}
	})
}

// Every valid subquery and CTE shape must run: a scope check that over-rejects
// would fail here.
func TestSubqueryShapesExecute(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		seedTenants(t, db, 1, 2, 2, 3)

		// A CTE defined once and referenced from the main query.
		big := momo.ColumnOf[int64]("big", "tenant_id")
		docs := momo.TableRef{Name: "documents"}
		cteBody := momo.Select[intRow](db).From(docs).Columns(suite.DocTenantID).Where(suite.DocTenantID.Ge(2))
		rows, err := momo.Select[intRow](db).With("big", cteBody, "tenant_id").
			From(momo.CTE("big")).Columns(big).OrderBy(big.Asc()).All(ctx)
		if err != nil || len(rows) != 3 || rows[0].N != 2 {
			t.Fatalf("CTE referenced from main = %+v, %v", rows, err)
		}

		// The CTE is referenced from an IN subquery of a model query.
		inCTE := momo.Select[suite.Document](db).
			With("big", cteBody, "tenant_id").
			Where(suite.DocTenantID.InQuery(momo.Select[intRow](db).From(momo.CTE("big")).Columns(big)))
		if n, err := inCTE.Count(ctx); err != nil || n != 3 {
			t.Fatalf("IN over CTE count = %d, %v; want 3", n, err)
		}

		// A scalar subquery in the select list.
		maxTenant := momo.Select[intRow](db).Columns(momo.Max(suite.DocTenantID))
		scalar, err := momo.Select[intRow](db).From(momo.TableRef{Name: "documents"}).
			Columns(maxTenant).Limit(1).All(ctx)
		if err != nil || len(scalar) != 1 || scalar[0].N != 3 {
			t.Fatalf("scalar subquery in SELECT = %+v, %v", scalar, err)
		}

		// A correlated EXISTS referencing the enclosing SELECT.
		inner := momo.Select[intRow](db).
			From(momo.TableRef{Name: "documents", Alias: "i"}).
			Columns(suite.DocTenantID.As("i")).
			Where(suite.DocTenantID.As("i").EqExpr(suite.DocTenantID.As("o")), suite.DocID.As("i").NeExpr(suite.DocID.As("o")))
		n, err := momo.Select[suite.Document](db).
			From(momo.TableRef{Name: "documents", Alias: "o"}).
			Where(momo.Exists(inner)).Count(ctx)
		if err != nil || n != 2 {
			t.Fatalf("correlated EXISTS count = %d, %v; want 2 (the two tenant-2 rows)", n, err)
		}

		// FromSubquery over a UNION of two queries.
		u := momo.Union(false,
			momo.Select[intRow](db).From(docs).Columns(suite.DocTenantID).Where(suite.DocTenantID.Eq(1)),
			momo.Select[intRow](db).From(docs).Columns(suite.DocTenantID).Where(suite.DocTenantID.Eq(3)),
		)
		un := momo.ColumnOf[int64]("u", "tenant_id")
		urows, err := momo.Select[intRow](db).From(momo.FromSubquery(u, "u")).Columns(un).OrderBy(un.Asc()).All(ctx)
		if err != nil || len(urows) != 2 || urows[1].N != 3 {
			t.Fatalf("union FromSubquery = %+v, %v", urows, err)
		}
	})
}
