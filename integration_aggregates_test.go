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

type tenantCount struct {
	momo.Projection
	TenantID int64
	N        int64
}

func (*tenantCount) MomoMeta() *momo.Meta[tenantCount] {
	return &momo.Meta[tenantCount]{
		Columns: []momo.ColumnMeta{{Name: "tenant_id"}, {Name: "n"}},
		Ptr: func(m *tenantCount, i int) any {
			if i == 0 {
				return &m.TenantID
			}
			return &m.N
		},
		Value: func(m *tenantCount, i int) any { return nil },
	}
}

func TestAggregatesGroupHaving(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		for _, tenant := range []int64{1, 1, 1, 2} {
			if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (?, 'x')", tenant); err != nil {
				t.Fatal(err)
			}
		}
		rows, err := momo.Select[tenantCount](db).
			From(momo.TableRef{Name: "documents"}).
			Columns(suite.DocTenantID, momo.CountAll()).
			GroupBy(suite.DocTenantID).
			Having(momo.CountAll().Gt(1)).
			OrderBy(suite.DocTenantID.Asc()).
			All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].TenantID != 1 || rows[0].N != 3 {
			t.Fatalf("rows = %+v, want tenant 1 with 3 documents", rows)
		}
	})
}

type tenantSum struct {
	momo.Projection
	TenantID int64
	Total    int64
	Lo       int64
	Hi       int64
}

func (*tenantSum) MomoMeta() *momo.Meta[tenantSum] {
	return &momo.Meta[tenantSum]{
		Columns: []momo.ColumnMeta{{Name: "tenant_id"}, {Name: "total"}, {Name: "lo"}, {Name: "hi"}},
		Ptr: func(m *tenantSum, i int) any {
			return []any{&m.TenantID, &m.Total, &m.Lo, &m.Hi}[i]
		},
		Value: func(m *tenantSum, i int) any { return nil },
	}
}

// TestAggregatesSumMinMax runs the value aggregates and a HAVING over every
// comparison method against real databases.
func TestAggregatesSumMinMax(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		for _, r := range [][2]int64{{1, 10}, {1, 20}, {2, 5}} {
			if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title, version) VALUES (?, 'x', ?)", r[0], r[1]); err != nil {
				t.Fatal(err)
			}
		}
		rows, err := momo.Select[tenantSum](db).
			From(momo.TableRef{Name: "documents"}).
			Columns(suite.DocTenantID, momo.Sum(suite.DocVersion), momo.Min(suite.DocVersion), momo.Max(suite.DocVersion)).
			GroupBy(suite.DocTenantID).
			Having(momo.Sum(suite.DocVersion).Ge(30), momo.Min(suite.DocVersion).Le(10), momo.Max(suite.DocVersion).Lt(21), momo.CountAll().Eq(2)).
			All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].TenantID != 1 || rows[0].Total != 30 || rows[0].Lo != 10 || rows[0].Hi != 20 {
			t.Fatalf("rows = %+v", rows)
		}
	})
}

func TestAggregatesSQL(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		c, err := momo.Select[tenantCount](db).
			From(momo.TableRef{Name: "documents"}).
			Columns(momo.Avg(momo.ColumnOf[float64]("documents", "score")), momo.CountAll()).
			Having(momo.CountAll().Ne(0)).SQL()
		if err != nil {
			t.Fatal(err)
		}
		want := map[dialect.Name]string{
			dialect.Postgres: `SELECT AVG("documents"."score"), COUNT(*) FROM "documents" HAVING COUNT(*) <> $1`,
			dialect.SQLite:   `SELECT AVG("documents"."score"), COUNT(*) FROM "documents" HAVING COUNT(*) <> ?`,
		}[d]
		if c.SQL != want {
			t.Fatalf("sql = %s\nwant  %s", c.SQL, want)
		}
	})
}
