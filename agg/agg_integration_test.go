// SPDX-License-Identifier: Apache-2.0

//go:build integration

package agg_test

import (
	"context"
	"math"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/agg"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/types"
)

type aggStrRow struct {
	momo.Projection
	S string
}

func (*aggStrRow) MomoMeta() *momo.Meta[aggStrRow] {
	return &momo.Meta[aggStrRow]{
		Columns:  []momo.ColumnMeta{{Name: "s"}},
		Ptr:      func(m *aggStrRow, _ int) any { return &m.S },
		Value:    func(m *aggStrRow, _ int) any { return m.S },
		Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	}
}

type aggFloatRow struct {
	momo.Projection
	F float64
	D int64
}

func (*aggFloatRow) MomoMeta() *momo.Meta[aggFloatRow] {
	return &momo.Meta[aggFloatRow]{
		Columns:  []momo.ColumnMeta{{Name: "f"}, {Name: "d"}},
		Ptr:      func(m *aggFloatRow, i int) any { return [...]any{&m.F, &m.D}[i] },
		Value:    func(m *aggFloatRow, i int) any { return [...]any{m.F, m.D}[i] },
		Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	}
}

type aggArrayRow struct {
	momo.Projection
	A []string
}

func (*aggArrayRow) MomoMeta() *momo.Meta[aggArrayRow] {
	return &momo.Meta[aggArrayRow]{
		Columns:  []momo.ColumnMeta{{Name: "a"}},
		Ptr:      func(m *aggArrayRow, _ int) any { return &m.A },
		Value:    func(m *aggArrayRow, _ int) any { return m.A },
		Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	}
}

type aggItem struct {
	Label  string `json:"label"`
	Tenant int64  `json:"tenant"`
}

type aggJSONRow struct {
	momo.Projection
	J types.JSON[[]aggItem]
}

func (*aggJSONRow) MomoMeta() *momo.Meta[aggJSONRow] {
	return &momo.Meta[aggJSONRow]{
		Columns:  []momo.ColumnMeta{{Name: "j"}},
		Ptr:      func(m *aggJSONRow, _ int) any { return &m.J },
		Value:    func(m *aggJSONRow, _ int) any { return m.J },
		Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	}
}

func seedAggDocs(t *testing.T, db *momo.DB) {
	t.Helper()
	for _, r := range []struct {
		tenant int64
		title  string
	}{{1, "delta"}, {1, "alpha"}, {2, "charlie"}, {2, "bravo"}} {
		if _, err := momo.Exec(context.Background(), db, "INSERT INTO documents (tenant_id, title) VALUES (?, ?)", r.tenant, r.title); err != nil {
			t.Fatal(err)
		}
	}
}

// Catches: String ignoring its ORDER BY or separator on a real server.
func TestStringAggOrderExecutes(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d == dialect.SQLite {
			t.Skip("aggregate ORDER BY needs SQLite 3.44; rendering is covered by goldens")
		}
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		seedAggDocs(t, db)
		call := agg.String(suite.DocTitle, ", ").OrderBy(suite.DocTitle.Desc())
		rows, err := momo.Select[aggStrRow](db).From(momo.TableRef{Name: "documents"}).Columns(call).All(context.Background())
		if err != nil || len(rows) != 1 || rows[0].S != "delta, charlie, bravo, alpha" {
			t.Fatalf("string_agg = %+v, %v; want delta, charlie, bravo, alpha", rows, err)
		}
	})
}

// Catches: percentile arguments or WITHIN GROUP rendered wrongly, checked on
// a known distribution (1..4: the interpolated median is 2.5, the discrete 2).
func TestPercentileOnKnownDistribution(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("ordered-set aggregates are PostgreSQL only")
		}
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		for i := int64(1); i <= 4; i++ {
			if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (?, 'x')", i); err != nil {
				t.Fatal(err)
			}
		}
		cont := agg.PercentileCont(0.5).WithinGroup(suite.DocTenantID.Asc())
		disc := agg.PercentileDisc[int64](0.5).WithinGroup(suite.DocTenantID.Asc())
		rows, err := momo.Select[aggFloatRow](db).From(momo.TableRef{Name: "documents"}).Columns(cont, disc).All(ctx)
		if err != nil || len(rows) != 1 || math.Abs(rows[0].F-2.5) > 1e-9 || rows[0].D != 2 {
			t.Fatalf("percentiles = %+v, %v; want cont 2.5 disc 2", rows, err)
		}
	})
}

// Catches: Array losing its order or failing to scan into a slice.
func TestArrayAggExecutes(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("array_agg is PostgreSQL only")
		}
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		seedAggDocs(t, db)
		call := agg.Array[string](suite.DocTitle).OrderBy(suite.DocTenantID.Asc(), suite.DocTitle.Asc())
		rows, err := momo.Select[aggArrayRow](db).From(momo.TableRef{Name: "documents"}).Columns(call).All(context.Background())
		if err != nil || len(rows) != 1 || !slices.Equal(rows[0].A, []string{"alpha", "delta", "bravo", "charlie"}) {
			t.Fatalf("array_agg = %+v, %v", rows, err)
		}
	})
}

// Catches: the jsonb_agg shape (array of objects, in order) and the empty
// group falling out as NULL instead of an empty array.
func TestJSONBAggShapeAndEmptyGroup(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("ordered jsonb_agg is PostgreSQL only")
		}
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		seedAggDocs(t, db)
		ctx := context.Background()
		obj := agg.BuildObject[aggItem](agg.Field("label", suite.DocTitle), agg.Field("tenant", suite.DocTenantID))
		call := func() agg.Call[types.JSON[[]aggItem]] {
			return agg.JSONB[aggItem](obj).OrderBy(suite.DocTitle.Asc())
		}
		run := func(tenant int64) []aggItem {
			t.Helper()
			col := momo.CoalesceExpr(call(), agg.EmptyJSONArray[aggItem]())
			rows, err := momo.Select[aggJSONRow](db).From(momo.TableRef{Name: "documents"}).
				Columns(col).Where(suite.DocTenantID.Eq(tenant)).All(ctx)
			if err != nil || len(rows) != 1 {
				t.Fatalf("jsonb_agg tenant %d = %+v, %v", tenant, rows, err)
			}
			return rows[0].J.V
		}
		got := run(1)
		if len(got) != 2 || got[0] != (aggItem{"alpha", 1}) || got[1] != (aggItem{"delta", 1}) {
			t.Fatalf("tenant 1 = %+v; want alpha, delta in order", got)
		}
		if got := run(99); got == nil || len(got) != 0 {
			t.Fatalf("empty group = %#v; want an empty array, not null", got)
		}
	})
}
