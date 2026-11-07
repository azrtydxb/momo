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

func seedTitles(t *testing.T, db *momo.DB, titles ...string) {
	t.Helper()
	for i, title := range titles {
		if _, err := momo.Exec(context.Background(), db, "INSERT INTO documents (tenant_id, title) VALUES (?, ?)", int64(i+1), title); err != nil {
			t.Fatal(err)
		}
	}
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
			Where(momo.Substr(suite.DocTitle, 1, 3).Eq("abc")).OrderBy(suite.DocTenantID.Asc()).All(ctx)
		if err != nil || len(rows) != 2 || rows[0].S != "abcdef" || rows[1].S != "abcxyz" {
			t.Fatalf("where substr = %+v, %v", rows, err)
		}

		rows, err = momo.Select[strCount](db).From(docs).Columns(suite.DocTitle, suite.DocTenantID).
			Where(momo.Length(suite.DocTitle).Le(1)).OrderBy(momo.Lower(suite.DocTitle)).All(ctx)
		if err != nil || len(rows) != 2 || rows[0].S != "a" || rows[1].S != "B" {
			t.Fatalf("order by lower = %+v, %v", rows, err)
		}

		rows, err = momo.Select[strCount](db).From(docs).Columns(suite.DocTitle, suite.DocTenantID).
			OrderBy(momo.Case[int64]().When(suite.DocTenantID.Eq(3), 0).Else(1).Asc(), momo.Add(suite.DocTenantID, 0).Desc()).
			Limit(2).All(ctx)
		if err != nil || len(rows) != 2 || rows[0].N != 3 || rows[1].N != 4 {
			t.Fatalf("order by case then arithmetic = %+v, %v", rows, err)
		}

		n := momo.CountAll().As("n")
		grouped, err := momo.Select[strCount](db).From(docs).
			Columns(momo.Substr(suite.DocTitle, 1, 1), n).
			GroupBy(momo.Substr(suite.DocTitle, 1, 1)).OrderBy(n.Desc(), momo.Substr(suite.DocTitle, 1, 1).Asc()).All(ctx)
		if err != nil || len(grouped) != 2 || grouped[0].S != "a" || grouped[0].N != 3 {
			t.Fatalf("order by alias = %+v, %v", grouped, err)
		}
	})
}

// Catches: Func[T] rendering a call the database rejects or binding its
// arguments wrongly, and CurrentSchema returning nothing on PostgreSQL.
func TestFuncAndCurrentSchemaExecute(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		seedTitles(t, db, "x", "y", "z")
		docs := momo.TableRef{Name: "documents"}

		rows, err := momo.Select[kvRow](db).From(docs).
			Columns(suite.DocTenantID, momo.Func[int64]("abs", momo.Sub(suite.DocTenantID, 10))).
			Where(momo.Func[int64]("abs", momo.Sub(suite.DocTenantID, 10)).Gt(7)).
			OrderBy(suite.DocTenantID.Asc()).All(ctx)
		if err != nil || len(rows) != 2 || rows[0].N != 9 || rows[1].N != 8 {
			t.Fatalf("Func abs = %+v, %v", rows, err)
		}

		if d != dialect.Postgres {
			return
		}
		s, err := momo.Select[strCount](db).From(docs).
			Columns(momo.CurrentSchema(), momo.CountAll()).All(ctx)
		if err != nil || len(s) != 1 || s[0].S == "" || s[0].N != 3 {
			t.Fatalf("current_schema = %+v, %v", s, err)
		}
	})
}
