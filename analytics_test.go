// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// anMeta builds the Meta of a projection with the named columns.
func anMeta[T any](names []string, ptrs func(*T) []any) *momo.Meta[T] {
	cols := make([]momo.ColumnMeta, len(names))
	for i, n := range names {
		cols[i] = momo.ColumnMeta{Name: n}
	}
	return &momo.Meta[T]{
		Columns:  cols,
		Ptr:      func(m *T, i int) any { return ptrs(m)[i] },
		Value:    func(m *T, i int) any { return nil },
		Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	}
}

// anFx is a currency and its rate.
type anFx struct {
	Currency string
	Rate     float64
}

var anFxMeta = anMeta([]string{"currency", "rate"}, func(m *anFx) []any { return []any{&m.Currency, &m.Rate} })

func (*anFx) MomoMeta() *momo.Meta[anFx] { return anFxMeta }

// anWin is a title with window results.
type anWin struct {
	Title   string
	Rn      int64
	Prev    *int64
	Running *int64
}

var anWinMeta = anMeta([]string{"title", "rn", "prev", "running"}, func(m *anWin) []any {
	return []any{&m.Title, &m.Rn, &m.Prev, &m.Running}
})

func (*anWin) MomoMeta() *momo.Meta[anWin] { return anWinMeta }

// anRank is a title with its rank and dense rank.
type anRank struct {
	Title string
	Rank  int64
	Dense int64
}

var anRankMeta = anMeta([]string{"title", "rank", "dense"}, func(m *anRank) []any {
	return []any{&m.Title, &m.Rank, &m.Dense}
})

func (*anRank) MomoMeta() *momo.Meta[anRank] { return anRankMeta }

// anCounts is a key with two counts.
type anCounts struct {
	K     int64
	All   int64
	Match int64
}

var anCountsMeta = anMeta([]string{"k", "all", "match"}, func(m *anCounts) []any {
	return []any{&m.K, &m.All, &m.Match}
})

func (*anCounts) MomoMeta() *momo.Meta[anCounts] { return anCountsMeta }

// anKT is a key and a title.
type anKT struct {
	K int64
	T string
}

var anKTMeta = anMeta([]string{"k", "t"}, func(m *anKT) []any { return []any{&m.K, &m.T} })

func (*anKT) MomoMeta() *momo.Meta[anKT] { return anKTMeta }

// anKNT is a key and a nullable title.
type anKNT struct {
	K int64
	T *string
}

var anKNTMeta = anMeta([]string{"k", "t"}, func(m *anKNT) []any { return []any{&m.K, &m.T} })

func (*anKNT) MomoMeta() *momo.Meta[anKNT] { return anKNTMeta }

// anN is one integer.
type anN struct{ N int64 }

var anNMeta = anMeta([]string{"n"}, func(m *anN) []any { return []any{&m.N} })

func (*anN) MomoMeta() *momo.Meta[anN] { return anNMeta }

// anOrd is a value and its ordinality.
type anOrd struct {
	V   string
	Ord int64
}

var anOrdMeta = anMeta([]string{"v", "ord"}, func(m *anOrd) []any { return []any{&m.V, &m.Ord} })

func (*anOrd) MomoMeta() *momo.Meta[anOrd] { return anOrdMeta }

// anT is one timestamp.
type anT struct{ T time.Time }

var anTMeta = anMeta([]string{"t"}, func(m *anT) []any { return []any{&m.T} })

func (*anT) MomoMeta() *momo.Meta[anT] { return anTMeta }

var (
	anVersion = momo.ColumnOf[int64]("documents", "version")
	anDocs    = momo.TableRef{Name: "documents"}
)

func TestAnalyticsGolden(t *testing.T) {
	pg := typedDB(dialect.Postgres)
	sq := typedDB(dialect.SQLite)

	cur := momo.UnnestValues("currency", []string{"EUR", "USD"})
	rate := momo.UnnestValues("rate", []float64{1.1, 1})
	tid := momo.UnnestValues[int64]("tid", []int64{1, 2})
	series := momo.GenerateSeries("g", "n", int64(1), int64(9), int64(2))
	series32 := momo.GenerateSeries("g", "n", int32(1), int32(3))
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tseries := momo.GenerateTimeSeries("d", "t", t0, t0.Add(48*time.Hour), 24*time.Hour)

	byTenant := momo.Over().PartitionBy(DocTenantID).OrderBy(DocID.Desc())
	running := momo.Over().PartitionBy(DocTenantID).OrderBy(DocID.Asc()).Rows(momo.UnboundedPreceding(), momo.CurrentRow())

	latest := func(d *momo.DB) momo.Subquery {
		return momo.Select[anKT](d).
			From(momo.TableRef{Name: "documents", Alias: "d"}).
			Columns(momo.ColumnOf[int64]("d", "id"), momo.ColumnOf[string]("d", "title")).
			Where(momo.ColumnOf[int64]("d", "tenant_id").EqExpr(momo.ColumnOf[int64]("tenants", "id"))).
			OrderBy(momo.ColumnOf[int64]("d", "id").Desc()).
			Limit(1)
	}

	cases := []struct {
		name string
		q    func() (momo.Compiled, error)
		want string
		args []any
	}{
		{"unnest two arrays", func() (momo.Compiled, error) {
			return momo.Select[anFx](pg).From(momo.Unnest("fx", cur, rate)).Columns(cur.Ref("fx"), rate.Ref("fx")).SQL()
		}, `SELECT "fx"."currency", "fx"."rate" FROM unnest($1::text[], $2::double precision[]) AS "fx" ("currency", "rate")`,
			[]any{[]string{"EUR", "USD"}, []float64{1.1, 1}}},
		{"unnest with ordinality", func() (momo.Compiled, error) {
			return momo.Select[anOrd](pg).From(momo.Unnest("u", cur).WithOrdinality("ord")).
				Columns(cur.Ref("u"), momo.ColumnOf[int64]("u", "ord")).SQL()
		}, `SELECT "u"."currency", "u"."ord" FROM unnest($1::text[]) WITH ORDINALITY AS "u" ("currency", "ord")`,
			[]any{[]string{"EUR", "USD"}}},
		{"unnest joined to a table", func() (momo.Compiled, error) {
			return momo.Select[unitDocument](pg).Join(momo.Unnest("t", tid), DocTenantID.EqExpr(tid.Ref("t"))).SQL()
		}, `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "documents" INNER JOIN unnest($1::bigint[]) AS "t" ("tid") ON "documents"."tenant_id" = "t"."tid" WHERE "documents"."deleted_at" IS NULL`,
			[]any{[]int64{1, 2}}},
		{"generate_series with step", func() (momo.Compiled, error) {
			return momo.Select[anN](pg).From(series).Columns(series.Column()).SQL()
		}, `SELECT "g"."n" FROM generate_series($1::bigint, $2::bigint, $3::bigint) AS "g" ("n")`,
			[]any{int64(1), int64(9), int64(2)}},
		{"generate_series int32 without step", func() (momo.Compiled, error) {
			return momo.Select[anN](pg).From(series32).Columns(momo.Cast[int64](series32.Column(), "bigint")).SQL()
		}, `SELECT CAST("g"."n" AS bigint) FROM generate_series($1::integer, $2::integer) AS "g" ("n")`,
			[]any{int32(1), int32(3)}},
		{"generate_series over time", func() (momo.Compiled, error) {
			return momo.Select[anT](pg).From(tseries).Columns(tseries.Column()).SQL()
		}, `SELECT "d"."t" FROM generate_series($1::timestamptz, $2::timestamptz, $3::bigint * interval '1 microsecond') AS "d" ("t")`,
			[]any{t0, t0.Add(48 * time.Hour), int64(24 * time.Hour / time.Microsecond)}},

		{"window functions", func() (momo.Compiled, error) {
			return momo.Select[anWin](pg).From(anDocs).Columns(
				DocTitle,
				momo.RowNumber().Over(byTenant),
				momo.Lag(anVersion, 1).Over(byTenant),
				momo.Sum(anVersion).Over(running),
			).SQL()
		}, `SELECT "documents"."title", row_number() OVER (PARTITION BY "documents"."tenant_id" ORDER BY "documents"."id" DESC), lag("documents"."version", 1) OVER (PARTITION BY "documents"."tenant_id" ORDER BY "documents"."id" DESC), SUM("documents"."version") OVER (PARTITION BY "documents"."tenant_id" ORDER BY "documents"."id" ASC ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) FROM "documents"`, nil},
		{"rank dense_rank and an empty window", func() (momo.Compiled, error) {
			o := momo.Over().OrderBy(anVersion.Desc())
			return momo.Select[anRank](pg).From(anDocs).Columns(DocTitle, momo.Rank().Over(o), momo.DenseRank().Over(o)).SQL()
		}, `SELECT "documents"."title", rank() OVER (ORDER BY "documents"."version" DESC), dense_rank() OVER (ORDER BY "documents"."version" DESC) FROM "documents"`, nil},
		{"lead with default and offset frames", func() (momo.Compiled, error) {
			w := momo.Over().OrderBy(DocID.Asc())
			return momo.Select[anWin](pg).From(anDocs).Columns(
				DocTitle,
				momo.CountAll().Over(momo.Over()),
				momo.LeadDefault(anVersion, 2, int64(0)).Over(w),
				momo.Avg(momo.ColumnOf[float64]("documents", "score")).Over(w.Rows(momo.Preceding(2), momo.Following(1))),
			).SQL()
		}, `SELECT "documents"."title", COUNT(*) OVER (), lead("documents"."version", 2, $1) OVER (ORDER BY "documents"."id" ASC), AVG("documents"."score") OVER (ORDER BY "documents"."id" ASC ROWS BETWEEN 2 PRECEDING AND 1 FOLLOWING) FROM "documents"`,
			[]any{int64(0)}},
		{"first_value and groups frame", func() (momo.Compiled, error) {
			w := momo.Over().PartitionBy(DocTenantID).OrderBy(anVersion.Asc())
			return momo.Select[anKT](pg).From(anDocs).Columns(
				momo.FirstValue(DocTitle).Over(w),
				momo.LastValue(DocTitle).Over(w.Groups(momo.UnboundedPreceding(), momo.UnboundedFollowing())),
			).SQL()
		}, `SELECT first_value("documents"."title") OVER (PARTITION BY "documents"."tenant_id" ORDER BY "documents"."version" ASC), last_value("documents"."title") OVER (PARTITION BY "documents"."tenant_id" ORDER BY "documents"."version" ASC GROUPS BETWEEN UNBOUNDED PRECEDING AND UNBOUNDED FOLLOWING) FROM "documents"`, nil},
		{"aggregate filter with group by and having", func() (momo.Compiled, error) {
			match := momo.CountAll().Filter(DocTitle.Eq("x"))
			return momo.Select[anCounts](pg).From(anDocs).
				Columns(DocTenantID, momo.CountAll(), match).
				GroupBy(DocTenantID).Having(match.Gt(0)).SQL()
		}, `SELECT "documents"."tenant_id", COUNT(*), COUNT(*) FILTER (WHERE "documents"."title" = $1) FROM "documents" GROUP BY "documents"."tenant_id" HAVING COUNT(*) FILTER (WHERE "documents"."title" = $2) > $3`,
			[]any{"x", "x", int64(0)}},
		{"filtered aggregate as a window", func() (momo.Compiled, error) {
			return momo.Select[anN](pg).From(anDocs).Columns(
				momo.Sum(anVersion).Filter(DocTitle.Ne("x")).Over(momo.Over().PartitionBy(DocTenantID)),
			).SQL()
		}, `SELECT SUM("documents"."version") FILTER (WHERE "documents"."title" <> $1) OVER (PARTITION BY "documents"."tenant_id") FROM "documents"`,
			[]any{"x"}},
		{"filter on sqlite", func() (momo.Compiled, error) {
			return momo.Select[anN](sq).From(anDocs).Columns(momo.CountAll().Filter(DocTitle.Eq("x"))).SQL()
		}, `SELECT COUNT(*) FILTER (WHERE "documents"."title" = ?) FROM "documents"`, []any{"x"}},
		{"window on sqlite", func() (momo.Compiled, error) {
			return momo.Select[anN](sq).From(anDocs).Columns(momo.RowNumber().Over(momo.Over().OrderBy(DocID.Asc()))).SQL()
		}, `SELECT row_number() OVER (ORDER BY "documents"."id" ASC) FROM "documents"`, nil},

		{"distinct on", func() (momo.Compiled, error) {
			return momo.Select[unitDocument](pg).DistinctOn(DocTenantID).OrderBy(DocTenantID.Asc(), DocID.Desc()).SQL()
		}, `SELECT DISTINCT ON ("documents"."tenant_id") "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "documents" WHERE "documents"."deleted_at" IS NULL ORDER BY "documents"."tenant_id" ASC, "documents"."id" DESC`, nil},
		{"distinct on in any order of the prefix", func() (momo.Compiled, error) {
			return momo.Select[anKT](pg).From(anDocs).Columns(DocTenantID, DocTitle).
				DistinctOn(DocTenantID, DocTitle).OrderBy(DocTitle.Asc(), DocTenantID.Desc(), DocID.Desc()).SQL()
		}, `SELECT DISTINCT ON ("documents"."tenant_id", "documents"."title") "documents"."tenant_id", "documents"."title" FROM "documents" ORDER BY "documents"."title" ASC, "documents"."tenant_id" DESC, "documents"."id" DESC`, nil},

		{"cross join lateral", func() (momo.Compiled, error) {
			return momo.Select[anKT](pg).From(momo.TableRef{Name: "tenants"}).
				Columns(momo.ColumnOf[int64]("tenants", "id"), momo.ColumnOf[string]("lo", "title")).
				CrossJoin(momo.Lateral(latest(pg), "lo")).SQL()
		}, `SELECT "tenants"."id", "lo"."title" FROM "tenants" CROSS JOIN LATERAL (SELECT "d"."id", "d"."title" FROM "documents" AS "d" WHERE "d"."tenant_id" = "tenants"."id" ORDER BY "d"."id" DESC LIMIT $1) AS "lo"`,
			[]any{int64(1)}},
		{"left join lateral on true", func() (momo.Compiled, error) {
			return momo.Select[anKNT](pg).From(momo.TableRef{Name: "tenants"}).
				Columns(momo.ColumnOf[int64]("tenants", "id"), momo.ColumnOf[string]("lo", "title")).
				LeftJoin(momo.Lateral(latest(pg), "lo"), momo.SQL("true")).SQL()
		}, `SELECT "tenants"."id", "lo"."title" FROM "tenants" LEFT JOIN LATERAL (SELECT "d"."id", "d"."title" FROM "documents" AS "d" WHERE "d"."tenant_id" = "tenants"."id" ORDER BY "d"."id" DESC LIMIT $1) AS "lo" ON true`,
			[]any{int64(1)}},
	}
	for _, c := range cases {
		got, err := c.q()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got.SQL != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got.SQL, c.want)
		}
		if len(got.Args) != len(c.args) || (len(c.args) > 0 && !reflect.DeepEqual(got.Args, c.args)) {
			t.Errorf("%s: args = %#v, want %#v", c.name, got.Args, c.args)
		}
	}
}

func TestAnalyticsErrors(t *testing.T) {
	pg := typedDB(dialect.Postgres)
	sq := typedDB(dialect.SQLite)
	my := typedDB(dialect.MySQL)
	cur := momo.UnnestValues("currency", []string{"EUR"})
	ord := momo.Over().OrderBy(DocID.Asc())

	win := func(d *momo.DB, e momo.Expr) func() error {
		return func() error { _, err := momo.Select[anN](d).From(anDocs).Columns(e).SQL(); return err }
	}
	cases := []struct {
		name string
		run  func() error
		want error
	}{
		{"unnest on sqlite", func() error {
			_, err := momo.Select[anN](sq).From(momo.Unnest("t", cur)).Columns(cur.Ref("t")).SQL()
			return err
		}, momo.ErrUnsupported},
		{"generate_series on mysql", func() error {
			s := momo.GenerateSeries("g", "n", 1, 3)
			_, err := momo.Select[anN](my).From(s).Columns(s.Column()).SQL()
			return err
		}, momo.ErrUnsupported},
		{"unnest without arrays", func() error {
			_, err := momo.Select[anN](pg).From(momo.Unnest("t")).Columns(cur.Ref("t")).SQL()
			return err
		}, momo.ErrValidation},
		{"unnest duplicate column", func() error {
			c2 := momo.UnnestValues("currency", []string{"x"})
			_, err := momo.Select[anN](pg).From(momo.Unnest("t", cur, c2)).Columns(cur.Ref("t")).SQL()
			return err
		}, momo.ErrValidation},
		{"unnest bad alias", func() error {
			_, err := momo.Select[anN](pg).From(momo.Unnest(`t"; DROP`, cur)).Columns(cur.Ref("t")).SQL()
			return err
		}, momo.ErrValidation},
		{"unnest element type unknown", func() error {
			c := momo.UnnestValues("x", []struct{ A int }{{1}})
			_, err := momo.Select[anN](pg).From(momo.Unnest("t", c)).Columns(c.Ref("t")).SQL()
			return err
		}, momo.ErrValidation},
		{"unnest explicit type invalid", func() error {
			c := momo.UnnestValuesAs("x", "numeric; DROP", []string{"1"})
			_, err := momo.Select[anN](pg).From(momo.Unnest("t", c)).Columns(c.Ref("t")).SQL()
			return err
		}, momo.ErrValidation},
		{"series zero step", func() error {
			s := momo.GenerateSeries("g", "n", 1, 3, 0)
			_, err := momo.Select[anN](pg).From(s).Columns(s.Column()).SQL()
			return err
		}, momo.ErrValidation},
		{"series two steps", func() error {
			s := momo.GenerateSeries("g", "n", 1, 3, 1, 2)
			_, err := momo.Select[anN](pg).From(s).Columns(s.Column()).SQL()
			return err
		}, momo.ErrValidation},
		{"time series sub-microsecond step", func() error {
			s := momo.GenerateTimeSeries("g", "t", time.Time{}, time.Time{}, time.Nanosecond)
			_, err := momo.Select[anT](pg).From(s).Columns(s.Column()).SQL()
			return err
		}, momo.ErrValidation},

		{"rank without order by", win(pg, momo.Rank().Over(momo.Over())), momo.ErrValidation},
		{"lag without order by", win(pg, momo.Lag(anVersion, 1).Over(momo.Over())), momo.ErrValidation},
		{"lag negative offset", win(pg, momo.Lag(anVersion, -1).Over(ord)), momo.ErrValidation},
		{"frame on row_number", win(pg, momo.RowNumber().Over(ord.Rows(momo.UnboundedPreceding(), momo.CurrentRow()))), momo.ErrValidation},
		{"frame starts at unbounded following", win(pg, momo.CountAll().Over(ord.Rows(momo.UnboundedFollowing(), momo.UnboundedFollowing()))), momo.ErrValidation},
		{"frame ends at unbounded preceding", win(pg, momo.CountAll().Over(ord.Rows(momo.UnboundedPreceding(), momo.UnboundedPreceding()))), momo.ErrValidation},
		{"frame start after end", win(pg, momo.CountAll().Over(ord.Rows(momo.CurrentRow(), momo.Preceding(1)))), momo.ErrValidation},
		{"negative frame offset", win(pg, momo.CountAll().Over(ord.Rows(momo.Preceding(-1), momo.CurrentRow()))), momo.ErrValidation},
		{"groups frame without order by", win(pg, momo.CountAll().Over(momo.Over().Groups(momo.UnboundedPreceding(), momo.CurrentRow()))), momo.ErrValidation},
		{"range offset frame with two order items", win(pg, momo.CountAll().Over(momo.Over().OrderBy(DocID.Asc(), DocTitle.Asc()).Range(momo.Preceding(1), momo.CurrentRow()))), momo.ErrValidation},
		{"aliased partition expression", win(pg, momo.CountAll().Over(momo.Over().PartitionBy(momo.As[int64](DocTenantID, "x")))), momo.ErrValidation},
		{"groups frame on mysql", win(my, momo.CountAll().Over(ord.Groups(momo.UnboundedPreceding(), momo.CurrentRow()))), momo.ErrUnsupported},
		{"filter without predicate", win(pg, momo.CountAll().Filter(nil)), momo.ErrValidation},
		{"filter on mysql", win(my, momo.CountAll().Filter(DocTitle.Eq("x"))), momo.ErrUnsupported},

		{"distinct on without order by", func() error {
			_, err := momo.Select[unitDocument](pg).DistinctOn(DocTenantID).SQL()
			return err
		}, momo.ErrValidation},
		{"distinct on with other leading order", func() error {
			_, err := momo.Select[unitDocument](pg).DistinctOn(DocTenantID).OrderBy(DocID.Asc(), DocTenantID.Asc()).SQL()
			return err
		}, momo.ErrValidation},
		{"distinct on with too short order", func() error {
			_, err := momo.Select[unitDocument](pg).DistinctOn(DocTenantID, DocTitle).OrderBy(DocTenantID.Asc()).SQL()
			return err
		}, momo.ErrValidation},
		{"distinct on without expressions", func() error {
			_, err := momo.Select[unitDocument](pg).DistinctOn().SQL()
			return err
		}, momo.ErrValidation},
		{"distinct on an alias", func() error {
			_, err := momo.Select[unitDocument](pg).DistinctOn(momo.As[int64](DocTenantID, "x")).OrderBy(DocTenantID.Asc()).SQL()
			return err
		}, momo.ErrValidation},
		{"distinct on sqlite", func() error {
			_, err := momo.Select[unitDocument](sq).DistinctOn(DocTenantID).OrderBy(DocTenantID.Asc()).SQL()
			return err
		}, momo.ErrUnsupported},
		{"distinct on mysql", func() error {
			_, err := momo.Select[unitDocument](my).DistinctOn(DocTenantID).OrderBy(DocTenantID.Asc()).SQL()
			return err
		}, momo.ErrUnsupported},

		{"lateral on sqlite", func() error {
			sub := momo.Select[anKT](sq).From(anDocs).Columns(DocID, DocTitle)
			_, err := momo.Select[anKT](sq).From(momo.TableRef{Name: "tenants"}).Columns(momo.ColumnOf[int64]("tenants", "id"), momo.ColumnOf[string]("lo", "title")).CrossJoin(momo.Lateral(sub, "lo")).SQL()
			return err
		}, momo.ErrUnsupported},
		{"lateral bad alias", func() error {
			sub := momo.Select[anKT](pg).From(anDocs).Columns(DocID, DocTitle)
			_, err := momo.Select[anKT](pg).From(momo.TableRef{Name: "tenants"}).Columns(momo.ColumnOf[int64]("tenants", "id"), momo.ColumnOf[string]("lo", "title")).CrossJoin(momo.Lateral(sub, "")).SQL()
			return err
		}, momo.ErrValidation},
	}
	for _, c := range cases {
		if err := c.run(); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
}

func TestAnalyticsMySQLLateralCompiles(t *testing.T) {
	my := typedDB(dialect.MySQL)
	sub := momo.Select[anKT](my).From(anDocs).Columns(DocID, DocTitle).Limit(1)
	c, err := momo.Select[anKT](my).From(momo.TableRef{Name: "tenants"}).
		Columns(momo.ColumnOf[int64]("tenants", "id"), momo.ColumnOf[string]("lo", "title")).
		CrossJoin(momo.Lateral(sub, "lo")).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT `tenants`.`id`, `lo`.`title` FROM `tenants` CROSS JOIN LATERAL (SELECT `documents`.`id`, `documents`.`title` FROM `documents` LIMIT ?) AS `lo`"
	if c.SQL != want {
		t.Errorf("got  %s\nwant %s", c.SQL, want)
	}
}
