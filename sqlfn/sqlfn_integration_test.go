// SPDX-License-Identifier: Apache-2.0

//go:build integration

package sqlfn_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/sqlfn"
)

// strCount is a projection of a string key and a count.
type strCount struct {
	S string
	N int64
}

var strCountMeta = momo.Meta[strCount]{
	Columns:  []momo.ColumnMeta{{Name: "s"}, {Name: "n"}},
	Ptr:      func(m *strCount, i int) any { return [...]any{&m.S, &m.N}[i] },
	Value:    func(m *strCount, i int) any { return [...]any{m.S, m.N}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*strCount) MomoMeta() *momo.Meta[strCount] { return &strCountMeta }

func seedTitles(t *testing.T, db *momo.DB, titles ...string) {
	t.Helper()
	for i, title := range titles {
		if _, err := momo.Exec(context.Background(), db, "INSERT INTO documents (tenant_id, title) VALUES (?, ?)", int64(i+1), title); err != nil {
			t.Fatal(err)
		}
	}
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
		prefix := func() momo.FuncExpr[string] { return sqlfn.Substr(suite.DocTitle, 1, 3) }
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
	merge := sqlfn.Case[string]().
		WhenExpr(nodeHostname.Excluded().Ne(""), nodeHostname.Excluded()).
		ElseExpr(nodeHostname)
	_, err := momo.Insert[Node](db).
		Values(nodeName.Set("a"), nodeHostname.Set(host), nodeHits.Set(hits)).
		OnConflict(nodeName).
		DoUpdate(
			nodeHostname.SetExpr(merge),
			nodeHits.SetExpr(sqlfn.AddExpr(nodeHits, nodeHits.Excluded())),
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
			Set(nodeHits.SetExpr(sqlfn.Sub(sqlfn.Mul(nodeHits, 2), 1))).
			Where(nodeName.Eq("a")).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		check("h2", 15)
	})
}

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
			Columns(sqlfn.Lower(suite.DocTitle), sqlfn.Cast[int64](momo.Sum(suite.DocTenantID), "bigint")).
			GroupBy(sqlfn.Lower(suite.DocTitle)).All(ctx)
		if err != nil || len(g) != 1 || g[0].S != "abc" || g[0].N != 5 {
			t.Fatalf("lower + cast(sum) = %+v, %v", g, err)
		}

		// upper, concat and length.
		c, err := momo.Select[strCount](db).From(docs).
			Columns(sqlfn.Concat(sqlfn.Upper(suite.DocTitle), momo.Value("-x")), sqlfn.Length(suite.DocTitle)).
			Where(suite.DocTenantID.Eq(2)).All(ctx)
		if err != nil || len(c) != 1 || c[0].S != "ABC-x" || c[0].N != 3 {
			t.Fatalf("concat + length = %+v, %v", c, err)
		}

		// coalesce over two columns: NULL body falls back to the title.
		co, err := momo.Select[strCount](db).From(docs).
			Columns(sqlfn.CoalesceExpr(suite.DocBody, suite.DocTitle), suite.DocTenantID).
			OrderBy(suite.DocTenantID.Asc()).All(ctx)
		if err != nil || len(co) != 2 || co[0].S != "ABC" || co[1].S != "bb" {
			t.Fatalf("coalesce = %+v, %v", co, err)
		}
	})
}

// Catches: computed expressions that compile but render SQL the databases
// reject in WHERE or ORDER BY, or order/filter on the wrong value.
func TestComputedExpressionsInWhereAndOrderBy(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		seedTitles(t, db, "abcdef", "B", "abcxyz", "a")
		docs := momo.TableRef{Name: "documents"}

		rows, err := momo.Select[strCount](db).From(docs).Columns(suite.DocTitle, suite.DocTenantID).
			Where(sqlfn.Substr(suite.DocTitle, 1, 3).Eq("abc")).OrderBy(suite.DocTenantID.Asc()).All(ctx)
		if err != nil || len(rows) != 2 || rows[0].S != "abcdef" || rows[1].S != "abcxyz" {
			t.Fatalf("where substr = %+v, %v", rows, err)
		}

		rows, err = momo.Select[strCount](db).From(docs).Columns(suite.DocTitle, suite.DocTenantID).
			Where(sqlfn.Length(suite.DocTitle).Le(1)).OrderBy(sqlfn.Lower(suite.DocTitle)).All(ctx)
		if err != nil || len(rows) != 2 || rows[0].S != "a" || rows[1].S != "B" {
			t.Fatalf("order by lower = %+v, %v", rows, err)
		}

		rows, err = momo.Select[strCount](db).From(docs).Columns(suite.DocTitle, suite.DocTenantID).
			OrderBy(sqlfn.Case[int64]().When(suite.DocTenantID.Eq(3), 0).Else(1).Asc(), sqlfn.Add(suite.DocTenantID, 0).Desc()).
			Limit(2).All(ctx)
		if err != nil || len(rows) != 2 || rows[0].N != 3 || rows[1].N != 4 {
			t.Fatalf("order by case then arithmetic = %+v, %v", rows, err)
		}

		n := momo.CountAll().As("n")
		grouped, err := momo.Select[strCount](db).From(docs).
			Columns(sqlfn.Substr(suite.DocTitle, 1, 1), n).
			GroupBy(sqlfn.Substr(suite.DocTitle, 1, 1)).OrderBy(n.Desc(), sqlfn.Substr(suite.DocTitle, 1, 1).Asc()).All(ctx)
		if err != nil || len(grouped) != 2 || grouped[0].S != "a" || grouped[0].N != 3 {
			t.Fatalf("order by alias = %+v, %v", grouped, err)
		}
	})
}

// Catches: CurrentSchema returning nothing on PostgreSQL.
func TestCurrentSchemaExecutes(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("current_schema() is PostgreSQL and MySQL only")
		}
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		seedTitles(t, db, "x", "y", "z")
		s, err := momo.Select[strCount](db).From(momo.TableRef{Name: "documents"}).
			Columns(sqlfn.CurrentSchema(), momo.CountAll()).All(ctx)
		if err != nil || len(s) != 1 || s[0].S == "" || s[0].N != 3 {
			t.Fatalf("current_schema = %+v, %v", s, err)
		}
	})
}
