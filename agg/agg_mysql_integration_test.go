// SPDX-License-Identifier: Apache-2.0

//go:build integration

package agg_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/agg"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/sqlfn"
)

type aggTextCountRow struct {
	momo.Projection
	S string
	N int64
}

func (*aggTextCountRow) MomoMeta() *momo.Meta[aggTextCountRow] {
	return &momo.Meta[aggTextCountRow]{
		Columns:  []momo.ColumnMeta{{Name: "s"}, {Name: "n"}},
		Ptr:      func(m *aggTextCountRow, i int) any { return [...]any{&m.S, &m.N}[i] },
		Value:    func(m *aggTextCountRow, i int) any { return [...]any{m.S, m.N}[i] },
		Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	}
}

// Catches: the MySQL and MariaDB forms of the aggregates rendering SQL the
// servers reject or computing the wrong value: GROUP_CONCAT with ORDER BY,
// DISTINCT and SEPARATOR, COUNT(DISTINCT), JSON_ARRAYAGG over JSON_OBJECT
// (member order is not defined there, so the test compares sorted), the empty
// group falling out as NULL instead of an empty array, and the forms MySQL
// cannot express (array_agg, an ordered or distinct JSON aggregate, to_jsonb,
// FILTER, WITHIN GROUP) rendering anyway instead of ErrUnsupported.
func TestAggregatesOnMySQLFamily(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		seedAggDocs(t, db)
		ctx := context.Background()
		if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, 'alpha')"); err != nil {
			t.Fatal(err)
		}
		docs := momo.TableRef{Name: "documents"}

		ordered := agg.String(suite.DocTitle, ", ").OrderBy(suite.DocTitle.Desc())
		rows, err := momo.Select[aggStrRow](db).From(docs).Where(suite.DocTenantID.Eq(2)).Columns(ordered).All(ctx)
		if err != nil || len(rows) != 1 || rows[0].S != "charlie, bravo" {
			t.Fatalf("GROUP_CONCAT ordered = %+v, %v; want charlie, bravo", rows, err)
		}

		distinct := agg.String(suite.DocTitle, ",").Distinct().OrderBy(suite.DocTitle.Asc())
		count := agg.Func[int64]("count", suite.DocTenantID).Distinct()
		both, err := momo.Select[aggTextCountRow](db).From(docs).Columns(distinct, count).All(ctx)
		if err != nil || len(both) != 1 || both[0].S != "alpha,bravo,charlie,delta" || both[0].N != 2 {
			t.Fatalf("distinct aggregates = %+v, %v; want alpha..delta and 2 tenants", both, err)
		}

		obj := agg.BuildObject[aggItem](agg.Field("label", suite.DocTitle), agg.Field("tenant", suite.DocTenantID))
		run := func(tenant int64) []aggItem {
			t.Helper()
			col := sqlfn.CoalesceExpr(agg.JSONB[aggItem](obj), agg.EmptyJSONArray[aggItem]())
			got, err := momo.Select[aggJSONRow](db).From(docs).Columns(col).Where(suite.DocTenantID.Eq(tenant)).All(ctx)
			if err != nil || len(got) != 1 {
				t.Fatalf("JSON_ARRAYAGG tenant %d = %+v, %v", tenant, got, err)
			}
			items := slices.Clone(got[0].J.V)
			slices.SortFunc(items, func(a, b aggItem) int {
				if a.Label < b.Label {
					return -1
				}
				if a.Label > b.Label {
					return 1
				}
				return 0
			})
			return items
		}
		if got := run(2); len(got) != 2 || got[0] != (aggItem{"bravo", 2}) || got[1] != (aggItem{"charlie", 2}) {
			t.Fatalf("tenant 2 = %+v; want bravo, charlie", got)
		}
		if got := run(99); got == nil || len(got) != 0 {
			t.Fatalf("empty group = %#v; want an empty array, not null", got)
		}

		unsupported := func(name string, e momo.Expr) {
			t.Helper()
			_, err := momo.Select[aggStrRow](db).From(docs).Columns(e).All(ctx)
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("%s = %v, want ErrUnsupported", name, err)
			}
		}
		unsupported("array_agg", agg.Array[string](suite.DocTitle))
		unsupported("ordered JSON_ARRAYAGG", agg.JSONB[aggItem](obj).OrderBy(suite.DocTitle.Asc()))
		unsupported("distinct JSON_ARRAYAGG", agg.JSONB[aggItem](obj).Distinct())
		unsupported("to_jsonb", agg.ToJSONB[string](suite.DocTitle))
		unsupported("FILTER", agg.String(suite.DocTitle, ",").Filter(suite.DocTenantID.Eq(1)))
		unsupported("WITHIN GROUP", agg.PercentileCont(0.5).WithinGroup(suite.DocTenantID.Asc()))
	})
}
