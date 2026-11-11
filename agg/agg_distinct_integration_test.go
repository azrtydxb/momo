// SPDX-License-Identifier: Apache-2.0

//go:build integration

package agg_test

import (
	"context"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/agg"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

type aggDistinctRow struct {
	momo.Projection
	A []int64
	S string
	N int64
}

func (*aggDistinctRow) MomoMeta() *momo.Meta[aggDistinctRow] {
	return &momo.Meta[aggDistinctRow]{
		Columns:  []momo.ColumnMeta{{Name: "a"}, {Name: "s"}, {Name: "n"}},
		Ptr:      func(m *aggDistinctRow, i int) any { return [...]any{&m.A, &m.S, &m.N}[i] },
		Value:    func(m *aggDistinctRow, i int) any { return [...]any{m.A, m.S, m.N}[i] },
		Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	}
}

// Catches: Distinct not reaching the database, so duplicates stay in the
// array, the joined string and the count (tenants are 1, 1, 2, 2).
func TestDistinctAggregatesExecute(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d != dialect.Postgres {
			t.Skip("rendering on other dialects is covered by goldens")
		}
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		seedAggDocs(t, db)
		tenant := suite.DocTenantID
		arr := agg.Array[int64](tenant).Distinct().OrderBy(tenant.Asc())
		str := agg.String(suite.DocTitle, ",").Distinct().OrderBy(suite.DocTitle.Asc())
		cnt := agg.Func[int64]("count", tenant).Distinct()
		rows, err := momo.Select[aggDistinctRow](db).From(momo.TableRef{Name: "documents"}).Columns(arr, str, cnt).All(context.Background())
		if err != nil || len(rows) != 1 {
			t.Fatalf("distinct aggregates = %+v, %v", rows, err)
		}
		if !slices.Equal(rows[0].A, []int64{1, 2}) || rows[0].S != "alpha,bravo,charlie,delta" || rows[0].N != 2 {
			t.Fatalf("distinct aggregates = %+v; want [1 2], alpha..delta, 2", rows[0])
		}
	})
}
