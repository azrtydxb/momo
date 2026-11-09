// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/sqlfn"
)

// seedAnalytics inserts documents: tenant 1 has versions 10, 20, 30 (titles
// a, b, c), tenant 2 has 5 (d) and tenant 3 has 7, 7, 9 (e, f, g).
func seedAnalytics(t *testing.T, db *momo.DB) {
	t.Helper()
	suite.CreateDocuments(t, db, suite.CapsOf(db))
	rows := []struct {
		tenant  int64
		title   string
		version int64
	}{{1, "a", 10}, {1, "b", 20}, {1, "c", 30}, {2, "d", 5}, {3, "e", 7}, {3, "f", 7}, {3, "g", 9}}
	for _, r := range rows {
		if _, err := momo.Exec(context.Background(), db, "INSERT INTO documents (tenant_id, title, version) VALUES (?, ?, ?)", r.tenant, r.title, r.version); err != nil {
			t.Fatal(err)
		}
	}
}

func wantUnsupported(t *testing.T, d dialect.Name, err error) bool {
	t.Helper()
	if d == dialect.Postgres {
		return false
	}
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("%s: err = %v, want ErrUnsupported", d, err)
	}
	return true
}

func TestAnalyticsUnnestSources(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		ctx := context.Background()
		cur := momo.UnnestValues("currency", []string{"EUR", "USD"})
		rate := momo.UnnestValues("rate", []float64{1.1, 1})
		fx := momo.Select[anFx](db).From(momo.Unnest("fx", cur, rate)).
			Columns(cur.Ref("fx"), rate.Ref("fx")).OrderBy(cur.Ref("fx").Asc())
		rows, err := fx.All(ctx)
		if wantUnsupported(t, d, err) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 || rows[0] != (anFx{"EUR", 1.1}) || rows[1] != (anFx{"USD", 1}) {
			t.Fatalf("rows = %+v", rows)
		}

		// The NovaCRM shape: the unnest as a CTE joined to a table.
		seedAnalytics(t, db)
		if _, err := momo.Exec(ctx, db, "UPDATE documents SET title = 'EUR' WHERE title = 'a'"); err != nil {
			t.Fatal(err)
		}
		joined, err := momo.Select[anFx](db).
			With("fx", fx, "currency", "rate").
			From(anDocs).
			Join(momo.CTE("fx"), suite.DocTitle.EqExpr(momo.ColumnOf[string]("fx", "currency"))).
			Columns(suite.DocTitle, momo.ColumnOf[float64]("fx", "rate")).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(joined) != 1 || joined[0] != (anFx{"EUR", 1.1}) {
			t.Fatalf("joined = %+v", joined)
		}

		// WITH ORDINALITY numbers the rows from 1.
		ord, err := momo.Select[anOrd](db).From(momo.Unnest("u", cur).WithOrdinality("ord")).
			Columns(cur.Ref("u"), momo.ColumnOf[int64]("u", "ord")).OrderBy(momo.ColumnOf[int64]("u", "ord").Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(ord) != 2 || ord[0] != (anOrd{"EUR", 1}) || ord[1] != (anOrd{"USD", 2}) {
			t.Fatalf("ord = %+v", ord)
		}

		// Arrays of different lengths are padded with NULL; an empty array
		// yields no rows.
		none := momo.UnnestValues("v", []string{})
		n, err := momo.Select[anN](db).From(momo.Unnest("e", none)).Columns(momo.CountAll()).One(ctx)
		if err != nil || n.N != 0 {
			t.Fatalf("empty unnest = %+v, %v", n, err)
		}
	})
}

func TestAnalyticsGenerateSeries(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		ctx := context.Background()
		s := momo.GenerateSeries("g", "n", int64(1), int64(6), int64(2))
		rows, err := momo.Select[anN](db).From(s).Columns(s.Column()).OrderBy(s.Column().Asc()).All(ctx)
		if wantUnsupported(t, d, err) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 3 || rows[0].N != 1 || rows[1].N != 3 || rows[2].N != 5 {
			t.Fatalf("series = %+v, want 1 3 5", rows)
		}

		down := momo.GenerateSeries("g", "n", 3, 1, -1)
		rows, err = momo.Select[anN](db).From(down).Columns(sqlfn.Cast[int64](down.Column(), "bigint")).All(ctx)
		if err != nil || len(rows) != 3 || rows[0].N != 3 || rows[2].N != 1 {
			t.Fatalf("down series = %+v, %v", rows, err)
		}

		t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		ts := momo.GenerateTimeSeries("d", "t", t0, t0.Add(48*time.Hour), 24*time.Hour)
		days, err := momo.Select[anT](db).From(ts).Columns(ts.Column()).OrderBy(ts.Column().Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(days) != 3 || !days[0].T.Equal(t0) || !days[2].T.Equal(t0.Add(48*time.Hour)) {
			t.Fatalf("days = %+v", days)
		}
	})
}

func TestAnalyticsWindowFunctions(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedAnalytics(t, db)
		ctx := context.Background()
		byTenant := momo.Over().PartitionBy(suite.DocTenantID).OrderBy(suite.DocVersion.Desc())
		asc := momo.Over().PartitionBy(suite.DocTenantID).OrderBy(suite.DocVersion.Asc(), suite.DocID.Asc())
		running := asc.Rows(momo.UnboundedPreceding(), momo.CurrentRow())
		rows, err := momo.Select[anWin](db).From(anDocs).
			Columns(
				suite.DocTitle,
				momo.RowNumber().Over(byTenant),
				momo.Lag(suite.DocVersion, 1).Over(asc),
				momo.Sum(suite.DocVersion).Over(running),
			).
			Where(suite.DocTenantID.Le(2)).
			OrderBy(suite.DocTitle.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		type want struct {
			title   string
			rn      int64
			prev    *int64
			running int64
		}
		i := func(v int64) *int64 { return &v }
		wants := []want{{"a", 3, nil, 10}, {"b", 2, i(10), 30}, {"c", 1, i(20), 60}, {"d", 1, nil, 5}}
		if len(rows) != len(wants) {
			t.Fatalf("rows = %+v", rows)
		}
		for k, w := range wants {
			r := rows[k]
			if r.Title != w.title || r.Rn != w.rn || (r.Prev == nil) != (w.prev == nil) || (w.prev != nil && *r.Prev != *w.prev) || r.Running == nil || *r.Running != w.running {
				t.Errorf("row %d = %+v (prev %v, running %v), want %+v", k, r, r.Prev, r.Running, w)
			}
		}

		// Ties: rank leaves a gap, dense_rank does not.
		o := momo.Over().PartitionBy(suite.DocTenantID).OrderBy(suite.DocVersion.Asc())
		ranks, err := momo.Select[anRank](db).From(anDocs).
			Columns(suite.DocTitle, momo.Rank().Over(o), momo.DenseRank().Over(o)).
			Where(suite.DocTenantID.Eq(3)).OrderBy(suite.DocTitle.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		wantRanks := []anRank{{"e", 1, 1}, {"f", 1, 1}, {"g", 3, 2}}
		if len(ranks) != 3 || ranks[0] != wantRanks[0] || ranks[1] != wantRanks[1] || ranks[2] != wantRanks[2] {
			t.Fatalf("ranks = %+v, want %+v", ranks, wantRanks)
		}
	})
}

func TestAnalyticsAggregateFilter(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedAnalytics(t, db)
		ctx := context.Background()
		over10 := momo.CountAll().Filter(suite.DocVersion.Gt(10))
		rows, err := momo.Select[anCounts](db).From(anDocs).
			Columns(suite.DocTenantID, momo.CountAll(), over10).
			GroupBy(suite.DocTenantID).OrderBy(suite.DocTenantID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := []anCounts{{1, 3, 2}, {2, 1, 0}, {3, 3, 0}}
		if len(rows) != 3 || rows[0] != want[0] || rows[1] != want[1] || rows[2] != want[2] {
			t.Fatalf("rows = %+v, want %+v", rows, want)
		}

		// HAVING over the filtered aggregate keeps only tenant 1.
		kept, err := momo.Select[anCounts](db).From(anDocs).
			Columns(suite.DocTenantID, momo.CountAll(), over10).
			GroupBy(suite.DocTenantID).Having(over10.Gt(0)).All(ctx)
		if err != nil || len(kept) != 1 || kept[0].K != 1 {
			t.Fatalf("having = %+v, %v", kept, err)
		}

		// A filtered aggregate used as a window function.
		sum := momo.Sum(suite.DocVersion).Filter(suite.DocVersion.Gt(10)).Over(momo.Over().PartitionBy(suite.DocTenantID))
		w, err := momo.Select[anWin](db).From(anDocs).
			Columns(suite.DocTitle, momo.RowNumber().Over(momo.Over().OrderBy(suite.DocID.Asc())), momo.Lag(suite.DocVersion, 1).Over(momo.Over().OrderBy(suite.DocID.Asc())), sum).
			Where(suite.DocTenantID.Eq(1)).OrderBy(suite.DocTitle.Asc()).All(ctx)
		if err != nil || len(w) != 3 || w[0].Running == nil || *w[0].Running != 50 {
			t.Fatalf("filtered window = %+v, %v", w, err)
		}
	})
}

func TestAnalyticsDistinctOn(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedAnalytics(t, db)
		ctx := context.Background()
		q := momo.Select[anKT](db).From(anDocs).Columns(suite.DocTenantID, suite.DocTitle).
			DistinctOn(suite.DocTenantID).OrderBy(suite.DocTenantID.Asc(), suite.DocVersion.Desc())
		rows, err := q.All(ctx)
		if wantUnsupported(t, d, err) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 3 || rows[0] != (anKT{1, "c"}) || rows[1] != (anKT{2, "d"}) || rows[2] != (anKT{3, "g"}) {
			t.Fatalf("rows = %+v, want the highest version per tenant", rows)
		}
		n, err := q.Count(ctx)
		if err != nil || n != 3 {
			t.Fatalf("Count = %d, %v, want 3", n, err)
		}
	})
}

func TestAnalyticsLateral(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedAnalytics(t, db)
		ctx := context.Background()
		ids := momo.UnnestValues("id", []int64{1, 2, 4})
		tenants := momo.Unnest("t", ids)
		latest := momo.Select[anKT](db).
			From(momo.TableRef{Name: "documents", Alias: "d"}).
			Columns(momo.ColumnOf[int64]("d", "id"), momo.ColumnOf[string]("d", "title")).
			Where(momo.ColumnOf[int64]("d", "tenant_id").EqExpr(ids.Ref("t"))).
			OrderBy(momo.ColumnOf[int64]("d", "version").Desc()).
			Limit(1)
		title := momo.ColumnOf[string]("lo", "title")

		inner, err := momo.Select[anKT](db).From(tenants).Columns(ids.Ref("t"), title).
			CrossJoin(momo.Lateral(latest, "lo")).OrderBy(ids.Ref("t").Asc()).All(ctx)
		if wantUnsupported(t, d, err) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(inner) != 2 || inner[0] != (anKT{1, "c"}) || inner[1] != (anKT{2, "d"}) {
			t.Fatalf("cross join lateral = %+v", inner)
		}

		outer, err := momo.Select[anKNT](db).From(tenants).Columns(ids.Ref("t"), title).
			LeftJoin(momo.Lateral(latest, "lo"), momo.SQL("true")).OrderBy(ids.Ref("t").Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(outer) != 3 || outer[2].K != 4 || outer[2].T != nil || outer[0].T == nil || *outer[0].T != "c" {
			t.Fatalf("left join lateral = %+v", outer)
		}
	})
}
