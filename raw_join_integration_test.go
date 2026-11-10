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

// seedJoinDocs inserts one document for tenant 1 and one for tenant 2.
func seedJoinDocs(t *testing.T, db *momo.DB) {
	t.Helper()
	suite.CreateDocuments(t, db, suite.CapsOf(db))
	stamp := "'2024-03-15 13:45:30+00'"
	if db.Dialect().Name == dialect.SQLite {
		stamp = "'2024-03-15 13:45:30.000'"
	}
	for _, s := range []string{
		"INSERT INTO documents (tenant_id, title, created_at) VALUES (1, 'old', " + stamp + ")",
		"INSERT INTO documents (tenant_id, title, created_at) VALUES (2, 'new', " + stamp + ")",
	} {
		if _, err := momo.Exec(context.Background(), db, s); err != nil {
			t.Fatal(err)
		}
	}
}

// Catches: JoinSQL fragments losing their binds or placeholder numbering when
// executed, and a typed join not usable as a value.
func TestJoinSQLIntegration(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		seedJoinDocs(t, db)
		ctx := context.Background()
		var conds []momo.Expr
		for _, tenant := range []int64{1, 99} {
			conds = append(conds, momo.SQL("tenant_id = ?", tenant))
		}
		r, err := momo.Select[unitTally](db).From(momo.TableRef{Name: "documents"}).
			Columns(momo.CountAll()).
			Where(suite.DocTitle.Ne("zzz"), momo.JoinSQL(" OR ", conds...)).One(ctx)
		if err != nil || r.N != 1 {
			t.Fatalf("OR join count = %+v, %v; want 1", r, err)
		}
		sum := momo.JoinTypedSQL[int64]("+", momo.SQL("tenant_id"), momo.SQL("tenant_id"))
		r, err = momo.Select[unitTally](db).From(momo.TableRef{Name: "documents"}).
			Columns(sum).Where(suite.DocTenantID.Eq(2)).One(ctx)
		if err != nil || r.N != 4 {
			t.Fatalf("typed + join = %+v, %v; want 4", r, err)
		}
	})
}
