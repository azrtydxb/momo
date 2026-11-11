// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"fmt"
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

// Catches: Func[T] rendering a call the database rejects or binding its
// arguments wrongly.
func TestFuncExecutes(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		seedTitles(t, db, "x", "y", "z")
		docs := momo.TableRef{Name: "documents"}

		rows, err := momo.Select[kvRow](db).From(docs).
			Columns(suite.DocTenantID, momo.Func[int64]("abs", suite.DocTenantID)).
			Where(momo.Func[int64]("abs", suite.DocTenantID).Gt(1)).
			OrderBy(suite.DocTenantID.Asc()).All(ctx)
		if err != nil || len(rows) != 2 || rows[0].N != 2 || rows[1].N != 3 {
			t.Fatalf("Func abs = %+v, %v", rows, err)
		}
	})
}

// Catches: a predicate not usable as an ORDER BY term, or rendered so that a
// database rejects it or sorts NULLs first instead of last.
func TestOrderByPredicateNullsLast(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		for i, body := range []any{nil, "b", nil, "a"} {
			if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title, body) VALUES (?, ?, ?)", int64(i+1), "t", body); err != nil {
				t.Fatal(err)
			}
		}
		ids := func(o ...momo.Orderable) []int64 {
			docs, err := momo.Select[suite.Document](db).OrderBy(o...).All(ctx)
			if err != nil {
				t.Fatal(err)
			}
			out := make([]int64, len(docs))
			for i, d := range docs {
				out[i] = d.TenantID
			}
			return out
		}
		same := func(a, b []int64) bool { return fmt.Sprint(a) == fmt.Sprint(b) }
		nullBody := momo.OrderByPredicate(suite.DocBody.IsNull())
		if got := ids(nullBody.Asc(), suite.DocTenantID.Asc()); !same(got, []int64{2, 4, 1, 3}) {
			t.Errorf("(body IS NULL) ASC = %v, want NULLs last [2 4 1 3]", got)
		}
		if got := ids(nullBody.Desc(), suite.DocTenantID.Asc()); !same(got, []int64{1, 3, 2, 4}) {
			t.Errorf("(body IS NULL) DESC = %v, want NULLs first [1 3 2 4]", got)
		}
		// A predicate with a bound argument, with another bound argument in WHERE.
		got, err := momo.Select[suite.Document](db).Where(suite.DocTitle.Eq("t")).
			OrderBy(momo.OrderByPredicate(suite.DocTenantID.Gt(2)).Desc(), suite.DocTenantID.Asc()).All(ctx)
		if err != nil || len(got) != 4 || got[0].TenantID != 3 || got[1].TenantID != 4 || got[2].TenantID != 1 {
			t.Errorf("(tenant_id > 2) DESC = %+v, %v", got, err)
		}
	})
}
