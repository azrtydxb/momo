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
		inner := momo.Select[pairRow](db).From(momo.TableRef{Name: "documents"}).
			Columns(suite.DocTenantID, n).GroupBy(suite.DocTenantID)

		rows, err := momo.Select[pairRow](db).From(momo.FromSubquery(inner, "s")).
			Columns(pairK, n.Ref("s")).Where(n.Ref("s").Gt(1)).All(ctx)
		if err != nil || len(rows) != 1 || rows[0].K != 2 || rows[0].N != 2 {
			t.Fatalf("subquery filtered on alias = %+v, %v; want tenant 2 with 2", rows, err)
		}

		k := momo.ColumnOf[int64]("t", "tenant_id")
		cte := momo.Select[pairRow](db).With("t", inner).From(momo.CTE("t")).
			Columns(k, n.Ref("t")).OrderBy(n.Ref("t").Desc(), k.Asc())
		rows, err = cte.All(ctx)
		if err != nil || len(rows) != 3 || rows[0].K != 2 || rows[0].N != 2 || rows[2].K != 3 {
			t.Fatalf("cte ordered on alias = %+v, %v", rows, err)
		}

		top := momo.Max(suite.DocTenantID).As("top")
		maxRows, err := momo.Select[pairRow](db).
			From(momo.FromSubquery(momo.Select[pairRow](db).From(momo.TableRef{Name: "documents"}).
				Columns(top, n), "m")).
			Columns(top.Ref("m"), n.Ref("m")).All(ctx)
		if err != nil || len(maxRows) != 1 || maxRows[0].K != 3 || maxRows[0].N != 4 {
			t.Fatalf("max and count aliased together = %+v, %v", maxRows, err)
		}
	})
}

// Catches: Substr positions bound as parameters (PostgreSQL rejects the
// ungrouped expression) and a wrong substring.
func TestSubstrGroupBy(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		for _, title := range []string{"abcdef", "abcxyz", "zzz"} {
			if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, ?)", title); err != nil {
				t.Fatal(err)
			}
		}
		prefix := func() momo.FuncExpr[string] { return momo.Substr(suite.DocTitle, 1, 3) }
		rows, err := momo.Select[strCount](db).From(momo.TableRef{Name: "documents"}).
			Columns(prefix(), momo.CountAll()).GroupBy(prefix()).OrderBy(momo.Asc(prefix())).All(ctx)
		if err != nil || len(rows) != 2 || rows[0].S != "abc" || rows[0].N != 2 || rows[1].S != "zzz" || rows[1].N != 1 {
			t.Fatalf("group by substr = %+v, %v", rows, err)
		}
	})
}

func createNodes(t *testing.T, db *momo.DB) {
	t.Helper()
	for _, s := range []string{
		"DROP TABLE IF EXISTS nodes",
		"CREATE TABLE nodes (name text primary key, hostname text not null, hits bigint not null default 0)",
	} {
		if _, err := db.Driver().Exec(context.Background(), s, nil); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func upsertNode(ctx context.Context, db *momo.DB, host string, hits int64) error {
	merge := momo.Case[string]().
		WhenExpr(nodeHostname.Excluded().Ne(""), nodeHostname.Excluded()).
		ElseExpr(nodeHostname)
	_, err := momo.Insert[Node](db).
		Values(nodeName.Set("a"), nodeHostname.Set(host), nodeHits.Set(hits)).
		OnConflict(nodeName).
		DoUpdate(
			nodeHostname.SetExpr(merge),
			nodeHits.SetExpr(momo.AddExpr(nodeHits, nodeHits.Excluded())),
		).Exec(ctx)
	return err
}

// Catches: the CASE choosing the wrong branch (an empty hostname wiping the
// stored one), and arithmetic overwriting instead of accumulating.
func TestUpsertCaseAndArithmetic(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		createNodes(t, db)
		ctx := context.Background()
		check := func(host string, hits int64) {
			t.Helper()
			n, err := momo.Select[Node](db).Where(nodeName.Eq("a")).One(ctx)
			if err != nil || n.Hostname != host || n.Hits != hits {
				t.Fatalf("node = %+v, %v; want host %q hits %d", n, err, host, hits)
			}
		}
		if err := upsertNode(ctx, db, "h1", 1); err != nil {
			t.Fatal(err)
		}
		check("h1", 1)
		if err := upsertNode(ctx, db, "", 5); err != nil { // empty keeps h1
			t.Fatal(err)
		}
		check("h1", 6)
		if err := upsertNode(ctx, db, "h2", 2); err != nil { // non-empty replaces
			t.Fatal(err)
		}
		check("h2", 8)

		// Column arithmetic with a bound value in a plain UPDATE.
		if _, err := momo.Update[Node](db).
			Set(nodeHits.SetExpr(momo.Sub(momo.Mul(nodeHits, 2), 1))).
			Where(nodeName.Eq("a")).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		check("h2", 15)
	})
}
