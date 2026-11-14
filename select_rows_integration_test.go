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

func seedTupleDocs(t *testing.T, db *momo.DB) {
	t.Helper()
	ctx := context.Background()
	for _, row := range []struct {
		tenant int64
		title  string
	}{{1, "a"}, {1, "b"}, {2, "c"}} {
		if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (?, ?)", row.tenant, row.title); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTupleRowAggregates(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		seedTupleDocs(t, db)
		ctx := context.Background()
		docs := momo.TableRef{Name: "documents"}

		rows, err := momo.Select[momo.Row3[int64, int64, string]](db).From(docs).
			Columns(suite.DocTenantID, momo.CountAll(), momo.Max(suite.DocTitle)).
			GroupBy(suite.DocTenantID).
			OrderBy(suite.DocTenantID.Asc()).
			All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 ||
			rows[0].C1 != 1 || rows[0].C2 != 2 || rows[0].C3 != "b" ||
			rows[1].C1 != 2 || rows[1].C2 != 1 || rows[1].C3 != "c" {
			t.Fatalf("grouped rows = %+v", rows)
		}

		total, err := momo.Select[momo.Row1[int64]](db).From(docs).Columns(momo.CountAll()).One(ctx)
		if err != nil || total.C1 != 3 {
			t.Fatalf("COUNT(*) = %+v, %v; want 3", total, err)
		}
	})
}

func TestTupleRowLeftJoinNullablePointers(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		seedTupleDocs(t, db)
		left := momo.TableRef{Name: "documents", Alias: "l"}
		right := momo.TableRef{Name: "documents", Alias: "r"}

		// Every document pairs with the tenant's document titled "b"; tenant 2
		// has none, so its right side is NULL.
		rows, err := momo.Select[momo.Row3[string, *int64, *string]](db).From(left).
			Columns(suite.DocTitle.As("l"), suite.DocID.As("r"), suite.DocTitle.As("r")).
			LeftJoin(right, suite.DocTenantID.As("l").EqExpr(suite.DocTenantID.As("r")), suite.DocTitle.As("r").Eq("b")).
			OrderBy(suite.DocTitle.As("l").Asc()).
			All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 3 {
			t.Fatalf("got %d rows: %+v", len(rows), rows)
		}
		for _, r := range rows[:2] {
			if r.C2 == nil || r.C3 == nil || *r.C3 != "b" {
				t.Errorf("matched row %+v: want a non-nil id and title b", r)
			}
		}
		if rows[2].C1 != "c" || rows[2].C2 != nil || rows[2].C3 != nil {
			t.Errorf("unmatched row = %+v, want c with nil pointers", rows[2])
		}
	})
}

func TestTupleRowSubqueryColumnAndCTE(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		seedTupleDocs(t, db)
		ctx := context.Background()
		docs := momo.TableRef{Name: "documents"}

		maxTenant := momo.Select[momo.Row1[int64]](db).From(docs).Columns(momo.Max(suite.DocTenantID))
		rows, err := momo.Select[momo.Row2[string, int64]](db).From(docs).
			Columns(suite.DocTitle, maxTenant).
			OrderBy(suite.DocTitle.Asc()).
			All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 3 || rows[0].C1 != "a" || rows[0].C2 != 2 || rows[2].C1 != "c" || rows[2].C2 != 2 {
			t.Fatalf("subquery column rows = %+v", rows)
		}

		big := momo.Select[momo.Row2[int64, string]](db).From(docs).
			Columns(suite.DocTenantID, suite.DocTitle).Where(suite.DocTenantID.Ge(2))
		cte, err := momo.Select[momo.Row1[string]](db).
			With("big", big, "tenant_id", "title").
			From(momo.CTE("big")).
			Columns(momo.ColumnOf[string]("big", "title")).
			All(ctx)
		if err != nil || len(cte) != 1 || cte[0].C1 != "c" {
			t.Fatalf("CTE rows = %+v, %v", cte, err)
		}

		u := momo.Union(true,
			momo.Select[momo.Row1[string]](db).From(docs).Columns(suite.DocTitle).Where(suite.DocTenantID.Eq(1)),
			momo.Select[momo.Row1[string]](db).From(docs).Columns(suite.DocTitle).Where(suite.DocTenantID.Eq(2)),
		)
		ut := momo.ColumnOf[string]("u", "title")
		un, err := momo.Select[momo.Row1[string]](db).
			From(momo.FromSubquery(u, "u")).Columns(ut).OrderBy(ut.Asc()).All(ctx)
		if err != nil || len(un) != 3 || un[0].C1 != "a" || un[2].C1 != "c" {
			t.Fatalf("union rows = %+v, %v", un, err)
		}
	})
}

// SUM over no rows is NULL; a pointer type parameter reads it as nil, and a
// non-empty set as the value.
func TestTupleRowNullableAggregate(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		seedTupleDocs(t, db)
		ctx := context.Background()
		docs := momo.TableRef{Name: "documents"}

		none, err := momo.Select[momo.Row1[*int64]](db).From(docs).
			Columns(momo.Sum(suite.DocTenantID)).
			Where(suite.DocTenantID.Gt(100)).
			One(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if none.C1 != nil {
			t.Errorf("SUM over no rows = %d, want nil", *none.C1)
		}

		some, err := momo.Select[momo.Row1[*int64]](db).From(docs).
			Columns(momo.Sum(suite.DocTenantID)).
			One(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if some.C1 == nil || *some.C1 != 4 {
			t.Errorf("SUM over three rows = %v, want 4", some.C1)
		}
	})
}

// A tuple row reads a recursive CTE that references itself.
func TestTupleRowWithRecursive(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seqN := momo.ColumnOf[int64]("seq", "n")
		body := momo.Union(true,
			momo.RawSubquery(`SELECT 1 AS n`),
			momo.Select[momo.Row1[int64]](db).From(momo.CTE("seq")).
				Columns(momo.TypedSQL[int64](`"seq"."n" + 1`)).Where(seqN.Lt(5)),
		)
		rows, err := momo.Select[momo.Row1[int64]](db).
			WithRecursive("seq", body, "n").
			From(momo.CTE("seq")).
			Columns(seqN).
			OrderBy(seqN.Asc()).
			All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 5 || rows[0].C1 != 1 || rows[4].C1 != 5 {
			t.Fatalf("recursive CTE rows = %+v, want 1..5", rows)
		}
	})
}
